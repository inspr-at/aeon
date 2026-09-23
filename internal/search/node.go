// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"encoding/json"
	"time"
)

// nodeJSON is the Node schema returned inside a search hit.
type nodeJSON struct {
	ID        string          `json:"id"`
	Key       string          `json:"key"`
	KindID    string          `json:"kind_id"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Fields    json.RawMessage `json:"fields"`
	State     string          `json:"state"`
	ParentID  *string         `json:"parent_id"`
	Position  string          `json:"position"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	DeletedAt *string         `json:"deleted_at"`
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
