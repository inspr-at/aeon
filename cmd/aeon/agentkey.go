// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/auth"
)

// agentKeyCommand runs the operator-only agent key commands:
//
//	aeon agent-key create --tenant SLUG --name AGENT --out-file PATH [--scopes a,b] [--expires 720h]
//	aeon agent-key revoke --tenant SLUG --id KEY_ID
//
// The token is written only to --out-file (created with mode 0600, never
// overwritten) and is never printed.
func agentKeyCommand(args []string, stdout io.Writer) error {
	const usage = "usage: aeon agent-key create --tenant SLUG --name AGENT --out-file PATH [--scopes a,b] [--expires DURATION] | aeon agent-key revoke --tenant SLUG --id KEY_ID"
	if len(args) == 0 || (args[0] != "create" && args[0] != "revoke") {
		return errors.New(usage)
	}
	flags := flag.NewFlagSet("aeon agent-key "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tenantSlug := flags.String("tenant", "", "tenant slug")
	name := flags.String("name", "", "agent name")
	outFile := flags.String("out-file", "", "file to write the key to (0600, must not exist)")
	scopes := flags.String("scopes", "", "comma-separated scopes")
	expires := flags.Duration("expires", 0, "lifetime, e.g. 720h (default: no expiry)")
	id := flags.String("id", "", "key id (revoke)")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *tenantSlug == "" {
		return errors.New(usage)
	}
	if args[0] == "create" && (*name == "" || *outFile == "") || args[0] == "revoke" && *id == "" {
		return errors.New(usage)
	}
	return withPool(func(ctx context.Context, pool *pgxpool.Pool) error {
		var tenantID string
		if err := pool.QueryRow(ctx, `SELECT id FROM tenants WHERE slug=$1`, *tenantSlug).Scan(&tenantID); err != nil {
			return fmt.Errorf("tenant %q: %w", *tenantSlug, err)
		}
		if args[0] == "revoke" {
			if err := auth.OperatorRevokeAgentKey(ctx, pool, tenantID, *id); err != nil {
				return err
			}
			return json.NewEncoder(stdout).Encode(map[string]string{"revoked": *id})
		}
		f, err := os.OpenFile(*outFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("out-file: %w", err)
		}
		var exp *time.Time
		if *expires > 0 {
			t := time.Now().Add(*expires)
			exp = &t
		}
		var list []string
		for _, s := range strings.Split(*scopes, ",") {
			if s = strings.TrimSpace(s); s != "" {
				list = append(list, s)
			}
		}
		keyID, principalID, token, err := auth.OperatorCreateAgentKey(ctx, pool, tenantID, *name, list, exp)
		if err != nil {
			f.Close()
			os.Remove(*outFile)
			return err
		}
		if _, err := f.WriteString(token + "\n"); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(map[string]any{"id": keyID, "principal_id": principalID, "name": *name, "scopes": list, "file": *outFile})
	})
}
