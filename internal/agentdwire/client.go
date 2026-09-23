// SPDX-License-Identifier: AGPL-3.0-only

// Package agentdwire is the authenticated local client for AEON agentd.
package agentdwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/inspr-at/aeon/internal/agentd"
)

type Client struct {
	Socket    string
	TokenFile string
}

func (c Client) token() (string, error) {
	if !filepath.IsAbs(c.Socket) || !filepath.IsAbs(c.TokenFile) {
		return "", errors.New("local agentd path must be absolute")
	}
	info, err := os.Lstat(c.TokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return "", errors.New("agentd token file unavailable")
	}
	raw, err := os.ReadFile(c.TokenFile)
	if err != nil || len(raw) != 32 {
		return "", errors.New("agentd token file invalid")
	}
	return string(raw), nil
}

func (c Client) Control(ctx context.Context, req agentd.ControlRequest) (agentd.Receipt, error) {
	token, err := c.token()
	if err != nil {
		return agentd.Receipt{}, err
	}
	if req.RunID == "" {
		return agentd.Receipt{}, errors.New("missing run ID")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return agentd.Receipt{}, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", c.Socket)
	}}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	r, err := http.NewRequestWithContext(ctx, "POST", "http://agentd/v1/runs/"+req.RunID+"/control", bytes.NewReader(body))
	if err != nil {
		return agentd.Receipt{}, err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(r)
	if err != nil {
		return agentd.Receipt{}, errors.New("local agentd unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return agentd.Receipt{}, errors.New("local agentd rejected control")
	}
	var receipt agentd.Receipt
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || receipt.RunID != req.RunID || receipt.Generation != req.Generation || receipt.CorrelationID != req.CorrelationID {
		return agentd.Receipt{}, errors.New("local agentd returned an invalid receipt")
	}
	return receipt, nil
}
