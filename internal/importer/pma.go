// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"context"
	"errors"
	"net/http"
	"regexp"
)

var pmaInstancePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

// PMAAdapter reads a PMA instance through the same GET-only classic snapshot
// API. The caller must explicitly name the source instance; source access and
// target-tenant selection remain operator decisions outside this package.
type PMAAdapter struct {
	instanceID string
	reader     *HTTPSource
}

// NewPMAAdapter constructs an adapter without making any source request.
func NewPMAAdapter(instanceID, sourceURL, keyFile string, client *http.Client) (*PMAAdapter, error) {
	if !pmaInstancePattern.MatchString(instanceID) {
		return nil, errors.New("valid PMA source instance identifier is required")
	}
	reader, err := NewHTTPSource(sourceURL, keyFile, client)
	if err != nil {
		return nil, err
	}
	return &PMAAdapter{instanceID: "pma:" + instanceID + ":" + reader.InstanceID(), reader: reader}, nil
}

func (s *PMAAdapter) InstanceID() string { return s.instanceID }

func (s *PMAAdapter) Read(ctx context.Context, projectKey string) (Snapshot, error) {
	snap, err := s.reader.Read(ctx, projectKey)
	if err != nil {
		return snap, err
	}
	snap.SourceID = s.instanceID
	return snap, nil
}
