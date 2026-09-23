// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"context"
	"encoding/json"
	"errors"
)

type inboxControl struct {
	RunID      string `json:"run_id"`
	Generation string `json:"generation"`
	Operation  string `json:"operation"`
	Text       string `json:"text"`
}

// DeliverInbox accepts only controls sent by this daemon's own principal.
// A message from another principal remains pending for a person or an
// explicitly authorized agent workflow; a bare inbox target is no authority.
// Receipt persistence precedes ACK, so at-least-once delivery replays safely.
func (s *Supervisor) DeliverInbox(ctx context.Context) error {
	var after int64
	for {
		page, err := s.api.Inbox(ctx, after)
		if err != nil {
			return err
		}
		for _, message := range page.Items {
			if message.SenderPrincipalID != s.principalID {
				continue
			}
			var in inboxControl
			if len(message.Body) > 64<<10 || json.Unmarshal([]byte(message.Body), &in) != nil || in.RunID == "" {
				continue
			}
			_, err := s.Control(ctx, ControlRequest{TenantID: s.tenantID, PrincipalID: s.principalID, RunID: in.RunID,
				Generation: in.Generation, CorrelationID: message.ID, Operation: in.Operation, Text: in.Text})
			if errors.Is(err, ErrNotOwned) || errors.Is(err, ErrGeneration) || errors.Is(err, ErrScope) || errors.Is(err, ErrUnsupported) {
				continue
			}
			if err != nil {
				return err
			}
			if err := s.api.Ack(ctx, message.ID); err != nil {
				return err
			}
		}
		if len(page.Items) < 100 || page.NextAfter <= after {
			return nil
		}
		after = page.NextAfter
	}
}
