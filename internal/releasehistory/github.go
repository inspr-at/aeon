// SPDX-License-Identifier: AGPL-3.0-only

package releasehistory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// GitHub reads release evidence from the GitHub REST API. The token (the
// release workflow's GITHUB_TOKEN) is sent only to API and never logged.
type GitHub struct {
	API    string // default https://api.github.com
	Token  string
	Client *http.Client
}

var (
	containerLine = regexp.MustCompile(`(?m)^Container:\s*(\S+)\s*$`)
	digestLine    = regexp.MustCompile(`(?m)^Digest:\s*(sha256:[0-9a-f]{64})\s*$`)
)

type ghRelease struct {
	TagName     string     `json:"tag_name"`
	HTMLURL     string     `json:"html_url"`
	Body        string     `json:"body"`
	Draft       bool       `json:"draft"`
	PublishedAt *time.Time `json:"published_at"`
}

type ghRun struct {
	Name       string `json:"name"`
	HTMLURL    string `json:"html_url"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Event      string `json:"event"`
	HeadBranch string `json:"head_branch"`
}

func (g *GitHub) get(ctx context.Context, path string, into any) error {
	base := g.API
	if base == "" {
		base = "https://api.github.com"
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(into)
}

// enrich adds publication times, image evidence and CI runs to tagged releases.
func (g *GitHub) enrich(ctx context.Context, repository string, releases []Release) error {
	if repository == "" {
		return fmt.Errorf("no repository named")
	}
	byTag := map[string]ghRelease{}
	for page := 1; page <= 10; page++ {
		var list []ghRelease
		if err := g.get(ctx, fmt.Sprintf("/repos/%s/releases?per_page=100&page=%d", repository, page), &list); err != nil {
			return err
		}
		for _, r := range list {
			if !r.Draft {
				byTag[r.TagName] = r
			}
		}
		if len(list) < 100 {
			break
		}
	}
	for i := range releases {
		r := &releases[i]
		if r.Tag == "" {
			continue
		}
		gh, ok := byTag[r.Tag]
		if !ok {
			r.Evidence.Unavailable = append(r.Evidence.Unavailable, "No GitHub release exists for this tag, so it has no publication time or image record.")
		} else {
			r.PublishedAt = gh.PublishedAt
			r.Evidence.ReleaseURL = gh.HTMLURL
			image := &Image{}
			if m := containerLine.FindStringSubmatch(gh.Body); m != nil {
				image.Reference = m[1]
			}
			if m := digestLine.FindStringSubmatch(gh.Body); m != nil {
				image.Digest = m[1]
			}
			if image.Reference != "" || image.Digest != "" {
				r.Evidence.Image = image
			}
			if image.Digest == "" {
				r.Evidence.Unavailable = append(r.Evidence.Unavailable, "The GitHub release does not record an image digest.")
			}
		}
		if r.Evidence.SourceCommit == "" {
			continue
		}
		var runs struct {
			WorkflowRuns []ghRun `json:"workflow_runs"`
		}
		if err := g.get(ctx, fmt.Sprintf("/repos/%s/actions/runs?head_sha=%s&per_page=50", repository, url.QueryEscape(r.Evidence.SourceCommit)), &runs); err != nil {
			r.Evidence.Unavailable = append(r.Evidence.Unavailable, "The CI runs for this commit could not be read.")
			continue
		}
		// Runs come newest first: the first CI run for the commit, and the Release run for the tag.
		for _, run := range runs.WorkflowRuns {
			item := &Run{Name: run.Name, URL: run.HTMLURL, Status: run.Status, Conclusion: run.Conclusion}
			switch {
			case run.Name == "CI" && r.Evidence.CI == nil:
				r.Evidence.CI = item
			case run.Name == "Release" && r.Evidence.ReleaseRun == nil && (run.HeadBranch == "" || run.HeadBranch == r.Tag):
				r.Evidence.ReleaseRun = item
			}
		}
		if r.Evidence.CI == nil {
			r.Evidence.Unavailable = append(r.Evidence.Unavailable, "No CI run was found for the source commit.")
		}
		if r.Evidence.ReleaseRun == nil {
			r.Evidence.Unavailable = append(r.Evidence.Unavailable, "No Release workflow run was found for this tag.")
		}
	}
	return nil
}
