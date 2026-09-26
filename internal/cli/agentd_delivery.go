// SPDX-License-Identifier: AGPL-3.0-only

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/inspr-at/paimos/internal/agentd"
	"github.com/inspr-at/paimos/internal/agentdwire"
	"github.com/inspr-at/paimos/internal/inbox"
)

// Managed targets keep the classic {socket,session_id} reference shape. In
// Aeon, session_id is the owned run UUID; its generation is read from the
// authenticated local daemon immediately before the fenced steer request.
func deliverAgentdMessaging(ctx context.Context, adapter string, work inbox.DeliveryWork) (localDeliveryResult, error) {
	if work.Message == nil || work.Message.Level != "steer" || work.MaximumLevel != "steer" {
		return localDeliveryResult{}, &localUnavailable{reason: "not_steerable"}
	}
	if adapter == "agentd_cursor" {
		return localDeliveryResult{}, &localUnavailable{reason: "unsupported"}
	}
	var target struct {
		Socket    string `json:"socket"`
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal([]byte(work.TargetRef), &target) != nil || !filepath.IsAbs(target.Socket) || !validUUID(target.SessionID) {
		return localDeliveryResult{}, errors.New("managed target is invalid")
	}
	tokenFile := target.Socket + ".token"
	info, err := os.Lstat(tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return localDeliveryResult{}, errors.New("local agentd token unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return localDeliveryResult{}, errors.New("local agentd token ownership invalid")
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil || len(token) != 32 {
		return localDeliveryResult{}, errors.New("local agentd token invalid")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", target.Socket)
	}}
	defer transport.CloseIdleConnections()
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(statusCtx, http.MethodGet, "http://agentd/v1/status", nil)
	if err != nil {
		return localDeliveryResult{}, errors.New("local agentd status unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+string(token))
	response, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return localDeliveryResult{}, errors.New("local agentd status unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return localDeliveryResult{}, errors.New("local agentd status refused")
	}
	var status struct {
		Generation string `json:"generation"`
		Runs       []struct {
			TenantID    string `json:"tenant_id"`
			PrincipalID string `json:"principal_id"`
			RunID       string `json:"run_id"`
			Generation  string `json:"generation"`
			State       string `json:"state"`
		} `json:"runs"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&status) != nil {
		return localDeliveryResult{}, errors.New("local agentd status invalid")
	}
	owned := false
	for _, run := range status.Runs {
		if run.RunID == target.SessionID && run.Generation == status.Generation && run.TenantID == work.TenantID && run.PrincipalID == work.PrincipalID && run.State == "running" {
			owned = true
			break
		}
	}
	if !owned {
		return localDeliveryResult{}, &localUnavailable{reason: "idle"}
	}
	control := agentd.ControlRequest{TenantID: work.TenantID, PrincipalID: work.PrincipalID, RunID: target.SessionID, Generation: status.Generation, CorrelationID: work.ID, Operation: "steer", Text: messagingFrame(*work.Message, work.ProjectKey)}
	receipt, err := (agentdwire.Client{Socket: target.Socket, TokenFile: tokenFile}).Control(ctx, control)
	if err != nil || receipt.Operation != "steer" || receipt.AppliedAt.IsZero() {
		return localDeliveryResult{}, errors.New("managed steer was not acknowledged")
	}
	return localDeliveryResult{EffectiveLevel: "steer"}, nil
}
