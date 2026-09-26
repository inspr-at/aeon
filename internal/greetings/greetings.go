// SPDX-License-Identifier: AGPL-3.0-only

// Package greetings serves a warm, personal greeting on each page load.
// New returns an httpapi.Module for GET /api/me/greeting; the coordinator mounts
// it in cmd/aeon. History is ephemeral personal state and writes no tenant event.
package greetings

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/httpapi"
	"github.com/inspr-at/paimos/internal/tenant"
)

//go:embed data/en.json
var corpusFS embed.FS

type Line struct {
	ID   string   `json:"id"`
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

type Greeting struct {
	Salutation string `json:"salutation"`
	Name       string `json:"name"`
	Message    string `json:"message"`
	ID         string `json:"id"`
}

type Module struct {
	pool  *pgxpool.Pool
	lines []Line
	now   func() time.Time
}

// New loads and validates the embedded English corpus and returns the HTTP
// module. The caller mounts it beside other modules; no server wiring is done
// here. The timezone preference is {"timezone":"Europe/Vienna"} under key
// "timezone"; preferred first name is {"first_name":"Ada"} under "profile".
func New(pool *pgxpool.Pool) (*Module, error) {
	data, err := corpusFS.ReadFile("data/en.json")
	if err != nil {
		return nil, fmt.Errorf("read greetings: %w", err)
	}
	var lines []Line
	if err := json.Unmarshal(data, &lines); err != nil {
		return nil, fmt.Errorf("decode greetings: %w", err)
	}
	if err := validateCorpus(lines); err != nil {
		return nil, err
	}
	return &Module{pool: pool, lines: lines, now: time.Now}, nil
}

var _ httpapi.Module = (*Module)(nil)

func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me/greeting", m.get)
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var out Greeting
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		// This row lock serializes concurrent draws for a principal, including
		// history read, selection, insertion and pruning.
		var fullName string
		if err := tx.QueryRow(r.Context(), `SELECT name FROM principals WHERE tenant_id=$1::uuid AND id=$2::uuid FOR UPDATE`, p.TenantID, p.ID).Scan(&fullName); err != nil {
			return err
		}
		prefs, err := readPreferences(r.Context(), tx, p.ID)
		if err != nil {
			return err
		}
		loc := location(prefs.timezone, r.Header.Get("X-Timezone"))
		now := m.now()
		local := now.In(loc)
		var history []shown
		rows, err := tx.Query(r.Context(), `SELECT greeting_id, shown_at FROM greeting_history WHERE principal_id=$1::uuid ORDER BY id DESC LIMIT 300`, p.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var item shown
			if err := rows.Scan(&item.id, &item.at); err != nil {
				rows.Close()
				return err
			}
			history = append(history, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		returning := len(history) > 0 && now.Sub(history[0].at) >= 72*time.Hour
		line, err := selectLine(m.lines, tagsAt(local, returning), history)
		if err != nil {
			return err
		}
		name := firstName(prefs.firstName, fullName)
		out = Greeting{Salutation: salutation(local, returning), Name: name, Message: line.Text, ID: line.ID}
		if _, err := tx.Exec(r.Context(), `INSERT INTO greeting_history (tenant_id, principal_id, greeting_id, shown_at) VALUES ($1::uuid,$2::uuid,$3,$4)`, p.TenantID, p.ID, line.ID, now); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `DELETE FROM greeting_history WHERE tenant_id=$1::uuid AND principal_id=$2::uuid AND id NOT IN (SELECT id FROM greeting_history WHERE tenant_id=$1::uuid AND principal_id=$2::uuid ORDER BY id DESC LIMIT 300)`, p.TenantID, p.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, http.StatusOK, out)
}

type preferences struct{ timezone, firstName string }

func readPreferences(ctx context.Context, tx pgx.Tx, principalID string) (preferences, error) {
	rows, err := tx.Query(ctx, `SELECT key, value FROM user_preferences WHERE principal_id=$1::uuid AND key IN ('timezone','profile')`, principalID)
	if err != nil {
		return preferences{}, err
	}
	defer rows.Close()
	var out preferences
	for rows.Next() {
		var key string
		var value struct {
			Timezone  string `json:"timezone"`
			FirstName string `json:"first_name"`
		}
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return preferences{}, err
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return preferences{}, err
		}
		if key == "timezone" {
			out.timezone = value.Timezone
		} else {
			out.firstName = value.FirstName
		}
	}
	return out, rows.Err()
}

func location(preferred, header string) *time.Location {
	for _, value := range []string{preferred, header} {
		if value == "" || len(value) > 128 || strings.ContainsAny(value, "\x00\\") {
			continue
		}
		if loc, err := time.LoadLocation(value); err == nil {
			return loc
		}
	}
	return time.UTC
}

func firstName(preferred, full string) string {
	for _, value := range []string{preferred, full} {
		if words := strings.Fields(value); len(words) > 0 {
			return words[0]
		}
	}
	return "there"
}

func salutation(local time.Time, returning bool) string {
	if returning {
		return "Welcome back"
	}
	switch h := local.Hour(); {
	case h >= 5 && h < 9:
		return "Morning"
	case h >= 9 && h < 12:
		return "Good morning"
	case h >= 12 && h < 17:
		return "Good afternoon"
	case h >= 17 && h < 22:
		return "Good evening"
	default:
		return "Hello"
	}
}

func tagsAt(local time.Time, returning bool) map[string]bool {
	tags := map[string]bool{"any": true}
	switch h := local.Hour(); {
	case h >= 5 && h < 12:
		tags["morning"] = true
	case h >= 12 && h < 17:
		tags["afternoon"] = true
	case h >= 17 && h < 22:
		tags["evening"] = true
	default:
		tags["night"] = true
	}
	switch local.Weekday() {
	case time.Monday:
		tags["monday"] = true
	case time.Friday:
		tags["friday"] = true
	case time.Saturday, time.Sunday:
		tags["weekend"] = true
	}
	if returning {
		tags["return"] = true
	}
	return tags
}

type shown struct {
	id string
	at time.Time
}

func selectLine(lines []Line, active map[string]bool, history []shown) (Line, error) {
	recent := make(map[string]bool, len(history))
	last := make(map[string]int, len(history))
	for i, item := range history {
		recent[item.id] = true
		if _, ok := last[item.id]; !ok {
			last[item.id] = i
		}
	}
	eligible := make([]Line, 0, len(lines))
	var fallback *Line
	fallbackAge := -1
	for i := range lines {
		line := lines[i]
		if !matches(line.Tags, active) {
			continue
		}
		if !recent[line.ID] {
			eligible = append(eligible, line)
		}
		age, seen := last[line.ID]
		if !seen {
			age = len(history) + 1
		}
		if age > fallbackAge {
			fallback, fallbackAge = &lines[i], age
		}
	}
	if len(eligible) == 0 {
		if fallback == nil {
			return Line{}, errors.New("no greeting matches context")
		}
		return *fallback, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(eligible))))
	if err != nil {
		return Line{}, err
	}
	return eligible[n.Int64()], nil
}

func matches(tags []string, active map[string]bool) bool {
	for _, tag := range tags {
		if active[tag] {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}), " ")
}

func lineID(s string) string {
	sum := sha256.Sum256([]byte(normalize(s)))
	return hex.EncodeToString(sum[:6])
}

func validateCorpus(lines []Line) error {
	if len(lines) != 1000 {
		return fmt.Errorf("greetings: got %d lines, want 1000", len(lines))
	}
	allowed := map[string]bool{"any": true, "morning": true, "afternoon": true, "evening": true, "night": true, "monday": true, "friday": true, "weekend": true, "return": true}
	ids := make(map[string]bool, len(lines))
	texts := make(map[string]bool, len(lines))
	tokens := make([]map[string]bool, 0, len(lines))
	anyCount := 0
	for i, line := range lines {
		if utf8.RuneCountInString(line.Text) > 80 || strings.TrimSpace(line.Text) != line.Text || line.Text == "" {
			return fmt.Errorf("greetings: line %d length or whitespace", i)
		}
		key := normalize(line.Text)
		if key == "" {
			return fmt.Errorf("greetings: line %d has no words", i)
		}
		if line.ID != lineID(line.Text) || ids[line.ID] || texts[key] {
			return fmt.Errorf("greetings: line %d id or text duplicate", i)
		}
		ids[line.ID], texts[key] = true, true
		if len(line.Tags) == 0 {
			return fmt.Errorf("greetings: line %d has no tags", i)
		}
		seenTags := map[string]bool{}
		for _, tag := range line.Tags {
			if !allowed[tag] || seenTags[tag] {
				return fmt.Errorf("greetings: line %d invalid tag %q", i, tag)
			}
			seenTags[tag] = true
		}
		if seenTags["any"] {
			anyCount++
		}
		set := make(map[string]bool)
		for _, word := range strings.Fields(key) {
			set[word] = true
		}
		for j, prior := range tokens {
			if jaccard(set, prior) >= 0.8 {
				return fmt.Errorf("greetings: lines %d and %d are near duplicates", j, i)
			}
		}
		tokens = append(tokens, set)
	}
	if anyCount < 600 {
		return fmt.Errorf("greetings: only %d any lines", anyCount)
	}
	return nil
}

func jaccard(a, b map[string]bool) float64 {
	intersection := 0
	for word := range a {
		if b[word] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(a)+len(b)-intersection)
}
