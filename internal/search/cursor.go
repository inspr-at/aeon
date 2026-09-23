// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
)

const cursorVersion = 1

// cursorBody binds a page position to the filters that produced it.
// Score text round-trips through ParseFloat. Node IDs are canonical
// lowercase UUIDs, which sort the same as the uuid column.
type cursorBody struct {
	V      int    `json:"v"`
	Tenant string `json:"t"`
	Q      string `json:"q"`
	Kind   string `json:"k,omitempty"`
	State  string `json:"s,omitempty"`
	Score  string `json:"r"`
	ID     string `json:"i"`
}

func encodeCursor(c cursorBody) (string, error) {
	c.V = cursorVersion
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeCursor(raw string) (cursorBody, error) {
	if raw == "" || len(raw) > 2048 {
		return cursorBody{}, errors.New("cursor")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return cursorBody{}, err
	}
	var c cursorBody
	if err := json.Unmarshal(b, &c); err != nil {
		return cursorBody{}, err
	}
	if c.V != cursorVersion || c.Tenant == "" || c.ID == "" || c.Score == "" {
		return cursorBody{}, errors.New("cursor")
	}
	if _, err := strconv.ParseFloat(c.Score, 64); err != nil {
		return cursorBody{}, err
	}
	return c, nil
}

func cursorMatches(c cursorBody, tenant, q, kind, state string) bool {
	return c.Tenant == tenant && c.Q == q && c.Kind == kind && c.State == state
}

type scored struct {
	ID    string
	Score float64
}

// pageHits walks hits, which aeon_search_nodes already ordered by score
// descending and id ascending, and returns at most limit rows after cur.
func pageHits(hits []scored, cur *cursorBody, limit int, bind cursorBody) ([]scored, *string, error) {
	start := 0
	if cur != nil {
		for start < len(hits) && !afterKey(hits[start], *cur) {
			start++
		}
	}
	if start > len(hits) {
		start = len(hits)
	}
	end := start + limit
	if end > len(hits) {
		end = len(hits)
	}
	out := append([]scored(nil), hits[start:end]...)
	if end >= len(hits) || len(out) == 0 {
		return out, nil, nil
	}
	last := out[len(out)-1]
	bind.Score = strconv.FormatFloat(last.Score, 'g', -1, 64)
	bind.ID = last.ID
	encoded, err := encodeCursor(bind)
	if err != nil {
		return nil, nil, err
	}
	return out, &encoded, nil
}

func afterKey(hit scored, cur cursorBody) bool {
	cs, err := strconv.ParseFloat(cur.Score, 64)
	if err != nil {
		return false
	}
	if hit.Score < cs {
		return true
	}
	if hit.Score > cs {
		return false
	}
	return hit.ID > cur.ID
}
