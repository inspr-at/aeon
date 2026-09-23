// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// errNoKey means the request did not present a live key for this agent.
var errNoKey = errors.New("api key not found")

// lookupKeyScopes reads the ceiling of the bearer key inside the tenant
// transaction. The token form is the same as auth: aeon_<prefix>_<secret>,
// and the stored hash is the hex sha256 of the secret.
func lookupKeyScopes(ctx context.Context, tx pgx.Tx, authorization, principalID string) ([]string, error) {
	prefix, secret, ok := parseBearer(authorization)
	if !ok {
		return nil, errNoKey
	}
	sum := sha256.Sum256([]byte(secret))
	var scopes pgtype.FlatArray[string]
	err := tx.QueryRow(ctx, `
		SELECT scopes FROM agent_keys
		WHERE prefix = $1 AND hash = $2 AND principal_id = $3::uuid
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())`,
		prefix, hex.EncodeToString(sum[:]), principalID).Scan(&scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNoKey
	}
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		return []string{}, nil
	}
	return []string(scopes), nil
}

func parseBearer(header string) (prefix, secret string, ok bool) {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", "", false
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(token), "aeon_")
	if !ok {
		return "", "", false
	}
	prefix, secret, ok = strings.Cut(rest, "_")
	if !ok || prefix == "" || secret == "" || strings.Contains(secret, "_") {
		return "", "", false
	}
	return prefix, secret, true
}
