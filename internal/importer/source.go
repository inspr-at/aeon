// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Record is an API object. Raw fields are retained so new classic fields are
// reported rather than silently discarded.
type Record map[string]any

type Project struct {
	Record Record
	Issues []Record
}

type Snapshot struct {
	SourceID string
	Users    []Record
	Projects []Project
	Orphans  []Record
	Details  map[int64]Details
	Skipped  []SkippedItem
}

type Details struct{ Relations, Comments, History, Attachments []Record }

// SkippedItem records a source record that disappeared during the snapshot.
type SkippedItem struct {
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Status int    `json:"status"`
}

type sourceHTTPError struct {
	method string
	path   string
	status int
}

func (e *sourceHTTPError) Error() string {
	return fmt.Sprintf("source %s %s returned HTTP %d", e.method, e.path, e.status)
}

func isNotFound(err error) bool {
	var httpErr *sourceHTTPError
	return errors.As(err, &httpErr) && httpErr.status == http.StatusNotFound
}

// Source is deliberately read-only. The client below only sends GET requests.
type Source interface {
	Read(context.Context, string) (Snapshot, error)
}

type HTTPSource struct {
	base        *url.URL
	key         string
	client      *http.Client
	concurrency int
	delay       time.Duration
	limit       chan struct{}
	mu          sync.Mutex
	nextRequest time.Time
}

// NewHTTPSource reads the bearer token once from a path. Errors never contain
// the key, response body, URL userinfo, or the file's contents.
func NewHTTPSource(rawURL, keyFile string, client *http.Client) (*HTTPSource, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("source-url must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read api-key-file: %w", err)
	}
	key := strings.TrimSpace(string(b))
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("api-key-file must contain one nonempty key")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	s := &HTTPSource{base: u, key: key, client: &copyClient}
	_ = s.Configure(4, 0)
	return s, nil
}

// Configure sets the request cap and minimum spacing between request starts.
// Call before Read; a source is used for one import at a time.
func (s *HTTPSource) Configure(concurrency int, delay time.Duration) error {
	if concurrency < 1 || delay < 0 {
		return errors.New("concurrency must be positive and delay nonnegative")
	}
	s.concurrency, s.delay, s.limit = concurrency, delay, make(chan struct{}, concurrency)
	return nil
}

func (s *HTTPSource) get(ctx context.Context, path string, out any) error {
	select {
	case s.limit <- struct{}{}:
		defer func() { <-s.limit }()
	case <-ctx.Done():
		return fmt.Errorf("source GET %s: %w", path, ctx.Err())
	}
	s.mu.Lock()
	wait := time.Until(s.nextRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			s.mu.Unlock()
			return fmt.Errorf("source GET %s: %w", path, ctx.Err())
		}
	}
	s.nextRequest = time.Now().Add(s.delay)
	s.mu.Unlock()
	u := *s.base
	parts := strings.SplitN(path, "?", 2)
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api" + parts[0]
	if len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("source GET %s: invalid request", path)
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("source GET %s failed", path)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &sourceHTTPError{method: http.MethodGet, path: path, status: resp.StatusCode}
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 64<<20))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode source GET %s JSON: %w", path, err)
	}
	return nil
}

func (s *HTTPSource) Read(ctx context.Context, projectKey string) (Snapshot, error) {
	snap := Snapshot{Details: map[int64]Details{}}
	origin := sha256.Sum256([]byte(s.base.String()))
	snap.SourceID = fmt.Sprintf("%x", origin[:12])
	var projects []Record
	if err := s.get(ctx, "/projects?status=all", &projects); err != nil {
		return snap, err
	}
	var deletedProjects []Record
	if err := s.get(ctx, "/projects?status=deleted", &deletedProjects); err != nil {
		return snap, err
	}
	projects = append(projects, deletedProjects...)
	var users []Record
	if err := s.get(ctx, "/users", &users); err != nil {
		return snap, err
	}
	var deletedUsers []Record
	if err := s.get(ctx, "/users?status=deleted", &deletedUsers); err != nil {
		return snap, err
	}
	snap.Users = append(users, deletedUsers...)
	selectedFound := false
	skippedProjects := map[int64]bool{}
	skippedIssues := map[int64]bool{}
	for _, p := range projects {
		if projectKey != "" && stringField(p, "key") != projectKey {
			continue
		}
		selectedFound = true
		id, ok := intField(p, "id")
		if !ok {
			return snap, errors.New("project missing id")
		}
		var issues []Record
		issuesPath := "/projects/" + strconv.FormatInt(id, 10) + "/issues"
		if err := s.get(ctx, issuesPath, &issues); err != nil {
			if isNotFound(err) {
				snap.Skipped = append(snap.Skipped, SkippedItem{Type: "project", ID: id, Path: issuesPath, Status: http.StatusNotFound})
				skippedProjects[id] = true
				continue
			}
			return snap, err
		}
		var knowledge []Record
		knowledgePath := "/projects/" + strconv.FormatInt(id, 10) + "/knowledge"
		if err := s.get(ctx, knowledgePath, &knowledge); err != nil {
			if isNotFound(err) {
				snap.Skipped = append(snap.Skipped, SkippedItem{Type: "project", ID: id, Path: knowledgePath, Status: http.StatusNotFound})
				skippedProjects[id] = true
				continue
			}
			return snap, err
		}
		seen := map[int64]bool{}
		for _, i := range issues {
			if id, ok := intField(i, "id"); ok {
				seen[id] = true
			}
		}
		for _, k := range knowledge {
			kid, ok := intField(k, "id")
			if !ok {
				return snap, errors.New("knowledge missing id")
			}
			if seen[kid] {
				continue
			}
			var full Record
			issuePath := "/issues/" + strconv.FormatInt(kid, 10)
			if err := s.get(ctx, issuePath, &full); err != nil {
				if isNotFound(err) {
					snap.Skipped = append(snap.Skipped, SkippedItem{Type: "issue", ID: kid, Path: issuePath, Status: http.StatusNotFound})
					skippedIssues[kid] = true
					seen[kid] = true
					continue
				}
				return snap, err
			}
			// Knowledge API has slug/body/metadata, issue API has preserved issue_key.
			for name, value := range k {
				if name != "id" {
					full[name] = value
				}
			}
			issues = append(issues, full)
			seen[kid] = true
		}
		snap.Projects = append(snap.Projects, Project{Record: p, Issues: issues})
	}
	if projectKey != "" && !selectedFound {
		return snap, fmt.Errorf("project %q not found", projectKey)
	}
	if projectKey == "" {
		known := map[int64]bool{}
		byProject := map[int64]int{}
		for n, p := range snap.Projects {
			pid, _ := intField(p.Record, "id")
			byProject[pid] = n
			for _, issue := range p.Issues {
				iid, _ := intField(issue, "id")
				known[iid] = true
			}
		}
		// The global list is paginated and includes orphan sprint issues.
		for offset := 0; ; offset += 100 {
			var page struct {
				Issues  []Record `json:"issues"`
				HasMore bool     `json:"has_more"`
			}
			if err := s.get(ctx, "/issues?limit=100&offset="+strconv.Itoa(offset), &page); err != nil {
				return snap, err
			}
			for _, issue := range page.Issues {
				iid, ok := intField(issue, "id")
				if !ok {
					return snap, errors.New("global issue missing id")
				}
				if known[iid] {
					continue
				}
				if skippedIssues[iid] {
					continue
				}
				known[iid] = true
				if pid, ok := intField(issue, "project_id"); ok {
					if skippedProjects[pid] {
						continue
					}
					if n, found := byProject[pid]; found {
						snap.Projects[n].Issues = append(snap.Projects[n].Issues, issue)
						continue
					}
				}
				snap.Orphans = append(snap.Orphans, issue)
			}
			if !page.HasMore {
				break
			}
			if len(page.Issues) == 0 {
				return snap, errors.New("global issues page has_more with no records")
			}
		}
		var trash []Record
		if err := s.get(ctx, "/issues/trash", &trash); err != nil {
			return snap, err
		}
		for _, issue := range trash {
			iid, ok := intField(issue, "id")
			if !ok {
				return snap, errors.New("trash issue missing id")
			}
			if known[iid] {
				continue
			}
			if skippedIssues[iid] {
				continue
			}
			known[iid] = true
			if pid, ok := intField(issue, "project_id"); ok {
				if skippedProjects[pid] {
					continue
				}
				if n, found := byProject[pid]; found {
					snap.Projects[n].Issues = append(snap.Projects[n].Issues, issue)
					continue
				}
			}
			snap.Orphans = append(snap.Orphans, issue)
		}
	} else if len(snap.Projects) > 0 {
		var trash []Record
		if err := s.get(ctx, "/issues/trash", &trash); err != nil {
			return snap, err
		}
		pid, _ := intField(snap.Projects[0].Record, "id")
		known := map[int64]bool{}
		for _, issue := range snap.Projects[0].Issues {
			iid, _ := intField(issue, "id")
			known[iid] = true
		}
		for _, issue := range trash {
			parentID, ok := intField(issue, "project_id")
			if !ok || parentID != pid {
				continue
			}
			iid, ok := intField(issue, "id")
			if !ok {
				return snap, errors.New("trash issue missing id")
			}
			if !known[iid] {
				if skippedIssues[iid] {
					continue
				}
				snap.Projects[0].Issues = append(snap.Projects[0].Issues, issue)
				known[iid] = true
			}
		}
	}
	var allIssues []Record
	for _, p := range snap.Projects {
		allIssues = append(allIssues, p.Issues...)
	}
	allIssues = append(allIssues, snap.Orphans...)
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan Record)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	var detailsMu sync.Mutex
	var skippedMu sync.Mutex
	for n := 0; n < s.concurrency && n < len(allIssues); n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for issue := range jobs {
				id, detail, err := s.readDetails(workCtx, issue)
				if err != nil {
					if isNotFound(err) {
						var httpErr *sourceHTTPError
						_ = errors.As(err, &httpErr)
						iid, _ := intField(issue, "id")
						skippedMu.Lock()
						snap.Skipped = append(snap.Skipped, SkippedItem{Type: "issue", ID: iid, Path: httpErr.path, Status: httpErr.status})
						skippedIssues[iid] = true
						skippedMu.Unlock()
						continue
					}
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
				detailsMu.Lock()
				snap.Details[id] = detail
				detailsMu.Unlock()
			}
		}()
	}
sendLoop:
	for _, issue := range allIssues {
		select {
		case jobs <- issue:
		case <-workCtx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return snap, err
	default:
	}
	if err := ctx.Err(); err != nil {
		return snap, err
	}
	for n := range snap.Projects {
		kept := snap.Projects[n].Issues[:0]
		for _, issue := range snap.Projects[n].Issues {
			id, _ := intField(issue, "id")
			if !skippedIssues[id] {
				kept = append(kept, issue)
			}
		}
		snap.Projects[n].Issues = kept
	}
	keptOrphans := snap.Orphans[:0]
	for _, issue := range snap.Orphans {
		id, _ := intField(issue, "id")
		if !skippedIssues[id] {
			keptOrphans = append(keptOrphans, issue)
		}
	}
	snap.Orphans = keptOrphans
	sort.Slice(snap.Skipped, func(i, j int) bool {
		if snap.Skipped[i].Type != snap.Skipped[j].Type {
			return snap.Skipped[i].Type < snap.Skipped[j].Type
		}
		if snap.Skipped[i].ID != snap.Skipped[j].ID {
			return snap.Skipped[i].ID < snap.Skipped[j].ID
		}
		return snap.Skipped[i].Path < snap.Skipped[j].Path
	})
	return snap, nil
}

func (s *HTTPSource) readDetails(ctx context.Context, issue Record) (int64, Details, error) {
	iid, ok := intField(issue, "id")
	if !ok {
		return 0, Details{}, errors.New("issue missing id")
	}
	base := "/issues/" + strconv.FormatInt(iid, 10)
	d := Details{}
	for _, part := range []struct {
		suffix string
		to     *[]Record
	}{{"/relations", &d.Relations}, {"/comments", &d.Comments}, {"/history", &d.History}, {"/attachments", &d.Attachments}} {
		if err := s.get(ctx, base+part.suffix, part.to); err != nil {
			return 0, Details{}, err
		}
	}
	return iid, d, nil
}

func stringField(r Record, k string) string { s, _ := r[k].(string); return s }
func intField(r Record, k string) (int64, bool) {
	switch v := r[k].(type) {
	case json.Number:
		n, e := v.Int64()
		return n, e == nil
	case float64:
		return int64(v), v == float64(int64(v))
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
}
