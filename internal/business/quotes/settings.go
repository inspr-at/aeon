// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/jackc/pgx/v5"
)

type quoteSettings struct {
	Revision                int64           `json:"revision"`
	NumberingTimeZone       string          `json:"numbering_time_zone"`
	DefaultCurrency         string          `json:"default_currency"`
	Sender                  json.RawMessage `json:"sender"`
	Defaults                json.RawMessage `json:"defaults"`
	Layout                  json.RawMessage `json:"layout"`
	SMTPConfirmationEnabled bool            `json:"smtp_confirmation_enabled"`
	SMTPConfigured          bool            `json:"smtp_configured"`
}

var timeZoneRe = regexp.MustCompile(`^[A-Za-z_]+(?:/[A-Za-z0-9_+\-]+)*$`)

type settingsWrite struct {
	ExpectedRevision        int64           `json:"expected_revision"`
	NumberingTimeZone       string          `json:"numbering_time_zone"`
	DefaultCurrency         string          `json:"default_currency"`
	Sender                  json.RawMessage `json:"sender"`
	Defaults                json.RawMessage `json:"defaults"`
	Layout                  json.RawMessage `json:"layout"`
	SMTPConfirmationEnabled bool            `json:"smtp_confirmation_enabled"`
}

func jsonObject(b json.RawMessage) bool {
	var v map[string]json.RawMessage
	return len(b) <= 128<<10 && json.Unmarshal(b, &v) == nil && v != nil
}
func validateDefaults(raw json.RawMessage) error {
	var v struct {
		Intro  string `json:"intro"`
		Blocks []struct {
			Heading string     `json:"heading"`
			Body    string     `json:"body"`
			Nodes   []textNode `json:"nodes"`
		} `json:"blocks"`
		AcceptText string `json:"accept_text"`
		VATNote    string `json:"vat_note"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil || len(v.Blocks) > 20 || len(v.Intro) > 100000 || len(v.AcceptText) > 100000 || len(v.VATNote) > 100000 {
		return bad("invalid quote defaults")
	}
	for _, b := range v.Blocks {
		if len(b.Heading) > 500 || len(b.Body) > 100000 || len(b.Nodes) > 100 {
			return bad("invalid quote defaults")
		}
	}
	probe := quoteDocument{SchemaVersion: 1, MinimumWriterVersion: 1, OfferDate: "2000-01-01", ValidUntil: "2000-01-01", Currency: "EUR", Sender: json.RawMessage(`{}`), Recipient: json.RawMessage(`{}`), Legal: json.RawMessage(`{}`), Layout: json.RawMessage(`{}`), Sections: []documentSection{}, Positions: []documentPosition{}}
	for _, b := range v.Blocks {
		id, err := newID()
		if err != nil {
			return err
		}
		section := documentSection{ID: id, Heading: b.Heading, Body: b.Body, Nodes: b.Nodes}
		for i := range section.Nodes {
			nodeID, err := newID()
			if err != nil {
				return err
			}
			section.Nodes[i].ID = nodeID
		}
		probe.Sections = append(probe.Sections, section)
	}
	if err := validateDocument(&probe, false); err != nil {
		return err
	}
	return nil
}
func readSettings(ctx context.Context, tx pgx.Tx) (quoteSettings, error) {
	var s quoteSettings
	err := tx.QueryRow(ctx, `SELECT revision,numbering_time_zone,default_currency,sender,defaults,layout,smtp_confirmation_enabled,smtp_configured FROM quote_settings`).Scan(&s.Revision, &s.NumberingTimeZone, &s.DefaultCurrency, &s.Sender, &s.Defaults, &s.Layout, &s.SMTPConfirmationEnabled, &s.SMTPConfigured)
	if errors.Is(err, pgx.ErrNoRows) {
		return quoteSettings{Sender: json.RawMessage(`{}`), Defaults: json.RawMessage(`{}`), Layout: json.RawMessage(`{}`)}, nil
	}
	return s, err
}
func (m *Module) settingsGet(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !staff(p) {
		respond(w, 0, nil, denied())
		return
	}
	var out quoteSettings
	e = m.tx(r.Context(), p, fence.PermViewsProvide, false, func(tx pgx.Tx) error { var err error; out, err = readSettings(r.Context(), tx); return err })
	respond(w, 200, out, e)
}
func (m *Module) settingsPatch(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !admin(p) {
		respond(w, 0, nil, denied())
		return
	}
	var in settingsWrite
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if in.ExpectedRevision < 0 || !jsonObject(in.Sender) || !jsonObject(in.Defaults) || !jsonObject(in.Layout) {
		respond(w, 0, nil, bad("invalid quote settings"))
		return
	}
	if !currencyRe.MatchString(in.DefaultCurrency) {
		respond(w, 0, nil, bad("invalid default currency"))
		return
	}
	if e = validateSnapshotFields(in.Sender, senderFields); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if e = validateSnapshotFields(in.Layout, layoutFields); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if e = validateDefaults(in.Defaults); e != nil {
		respond(w, 0, nil, e)
		return
	}
	if _, e = time.LoadLocation(in.NumberingTimeZone); e != nil || len(in.NumberingTimeZone) > 100 || in.NumberingTimeZone == "Local" || !timeZoneRe.MatchString(in.NumberingTimeZone) {
		respond(w, 0, nil, bad("invalid numbering time zone"))
		return
	}
	var out quoteSettings
	e = m.tx(r.Context(), p, fence.PermNodesContribute, true, func(tx pgx.Tx) error {
		// This also serializes first setup, when no settings row exists yet.
		var tenantID string
		if err := tx.QueryRow(r.Context(), `SELECT id::text FROM tenants WHERE id=$1::uuid FOR UPDATE`, p.TenantID).Scan(&tenantID); err != nil {
			return err
		}
		current, err := readSettings(r.Context(), tx)
		if err != nil {
			return err
		}
		if current.Revision != in.ExpectedRevision {
			return conflict("settings revision is stale")
		}
		if in.SMTPConfirmationEnabled && !current.SMTPConfigured {
			return bad("SMTP confirmation requires a configured transport")
		}
		if current.Revision == 0 {
			_, err = tx.Exec(r.Context(), `INSERT INTO quote_settings(tenant_id,revision,numbering_time_zone,default_currency,sender,defaults,layout,updated_by_principal_id) VALUES($1::uuid,1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::uuid)`, p.TenantID, in.NumberingTimeZone, in.DefaultCurrency, string(in.Sender), string(in.Defaults), string(in.Layout), p.ID)
		} else {
			_, err = tx.Exec(r.Context(), `UPDATE quote_settings SET revision=revision+1,numbering_time_zone=$1,default_currency=$8,sender=$2::jsonb,defaults=$3::jsonb,layout=$4::jsonb,smtp_confirmation_enabled=$7,updated_at=now(),updated_by_principal_id=$5::uuid WHERE tenant_id=$6::uuid`, in.NumberingTimeZone, string(in.Sender), string(in.Defaults), string(in.Layout), p.ID, p.TenantID, in.SMTPConfirmationEnabled, in.DefaultCurrency)
		}
		if err != nil {
			return err
		}
		out, err = readSettings(r.Context(), tx)
		if err != nil {
			return err
		}
		return appendEvent(r.Context(), tx, p, "", "quote.settings_updated", map[string]any{"revision": current.Revision}, map[string]any{"revision": out.Revision})
	})
	respond(w, 200, out, e)
}
