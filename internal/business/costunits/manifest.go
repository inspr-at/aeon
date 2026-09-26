// SPDX-License-Identifier: AGPL-3.0-only

package costunits

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
)

const (
	// PluginID is the compiled registry id.
	PluginID = "business_costs"
	// Version is the manifest version pinned by an installation.
	Version = "1"
	// Owner is the first-party package accountable for this manifest.
	Owner = "internal/business/costunits"
	// KindSlug is the R1 node kind, including imported classic cost units.
	KindSlug = "cost_unit"
	// ViewID is the cost_units view declared by the manifest.
	ViewID = "cost_units"
	// StepKey is the workflow step bound to steps.apply.
	StepKey = "cost_rate_change"
	// OutcomeRecorded is the only terminal claim ApplyResult can accept.
	OutcomeRecorded = "recorded"

	eventCreated = "cost_unit_rate.created"
	eventClosed  = "cost_unit_rate.closed"
	panelRates   = "rates"
)

// Facts is the observed state for cost_rate_change. The host fills it from
// the database and the signed-in person. A true field here does not grant
// authority to write a rate.
type Facts struct {
	Operation      string
	CostUnitNodeID string
	Live           bool
	Unit           string
	Currency       string
	InternalAmount string
	BillAmount     string
	EffectiveFrom  string
	EffectiveUntil string
	Overlap        bool
	PersonAdmin    bool
}

// Plugin returns the compiled business_costs plugin. Register it before the
// registry is sealed.
func Plugin() (plugins.Plugin, error) {
	schema, err := fieldSchema()
	if err != nil {
		return plugins.Plugin{}, err
	}
	p := plugins.Plugin{
		Manifest: plugins.Manifest{
			ID:      PluginID,
			Version: Version,
			Owner:   Owner,
			Permissions: []string{
				fence.PermNodesContribute,
				fence.PermViewsProvide,
				fence.PermStepsApply,
			},
			NodeKinds: []plugins.NodeKind{{
				Slug:              KindSlug,
				FieldSchema:       schema,
				AllowedChildKinds: []string{},
			}},
			Views: []plugins.View{{
				ID:     ViewID,
				Panels: []string{panelRates},
			}},
			WorkflowSteps: []plugins.WorkflowStep{{
				Key:   StepKey,
				Gates: []string{fence.GateObservedState, fence.GatePersonDecision},
			}},
		},
		StepPermissions: map[string]string{StepKey: fence.PermStepsApply},
		Kinds:           costKinds{},
		Views:           costViews{},
		Steps:           costSteps{},
	}
	sum, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = sum
	return p, nil
}

var (
	schemaOnce sync.Once
	schemaRaw  json.RawMessage
	schemaErr  error
)

func fieldSchema() (json.RawMessage, error) {
	schemaOnce.Do(func() {
		schemaRaw, schemaErr = canonicalJSON(map[string]any{
			"$id":                  "urn:aeon:business_costs:cost_unit",
			"type":                 "object",
			"description":          "Display metadata for a cost unit. Classic imported fields, including classic, stay valid. The node keeps its original key.",
			"additionalProperties": true,
			"properties": map[string]any{
				"display_name":     map[string]any{"type": "string", "minLength": 1, "maxLength": 200},
				"billing_note":     map[string]any{"type": "string", "maxLength": 2000},
				"default_unit":     map[string]any{"type": "string", "enum": []any{"hour", "day", "item"}},
				"default_currency": map[string]any{"type": "string", "pattern": "^[A-Z]{3}$"},
				"classic": map[string]any{
					"type":                 []any{"object", "null"},
					"additionalProperties": true,
				},
			},
		})
	})
	if schemaErr != nil {
		return nil, schemaErr
	}
	return append(json.RawMessage(nil), schemaRaw...), nil
}

type costKinds struct{}

func (costKinds) NodeKinds(ctx context.Context, call plugins.Call) ([]plugins.NodeKind, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !call.Grant.Allows(fence.PermNodesContribute) {
		return nil, plugins.ErrDenied
	}
	schema, err := fieldSchema()
	if err != nil {
		return nil, err
	}
	return []plugins.NodeKind{{
		Slug:              KindSlug,
		FieldSchema:       schema,
		AllowedChildKinds: []string{},
	}}, nil
}

type costViews struct{}

func (costViews) Views(ctx context.Context, call plugins.Call) ([]plugins.View, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !call.Grant.Allows(fence.PermViewsProvide) {
		return nil, plugins.ErrDenied
	}
	return []plugins.View{{ID: ViewID, Panels: []string{panelRates}}}, nil
}

type costSteps struct{}

func (costSteps) Evaluate(ctx context.Context, call plugins.Call, in plugins.StepRequest) (plugins.StepDecision, error) {
	facts, err := factsFrom(ctx, call, in)
	if err != nil {
		return plugins.StepDecision{}, err
	}
	return decide(facts, "", false)
}

func (costSteps) Request(ctx context.Context, call plugins.Call, in plugins.StepRequest) (plugins.StepDecision, error) {
	facts, err := factsFrom(ctx, call, in)
	if err != nil {
		return plugins.StepDecision{}, err
	}
	return decide(facts, "", false)
}

func (costSteps) ApplyResult(ctx context.Context, call plugins.Call, in plugins.StepResult) (plugins.StepDecision, error) {
	facts, err := factsFrom(ctx, call, in.StepRequest)
	if err != nil {
		return plugins.StepDecision{}, err
	}
	return decide(facts, in.ClaimedOutcome, true)
}

func factsFrom(ctx context.Context, call plugins.Call, in plugins.StepRequest) (Facts, error) {
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	if !call.Grant.Allows(fence.PermStepsApply) {
		return Facts{}, plugins.ErrDenied
	}
	facts, ok := in.Payload.(Facts)
	if !ok || facts.Operation != StepKey || facts.Operation != in.Operation {
		return Facts{}, errors.New("cost rate change facts are required")
	}
	if !validUUID(facts.CostUnitNodeID) || !validUnit(facts.Unit) || !validCurrency(facts.Currency) {
		return Facts{}, errors.New("cost rate change facts are incomplete")
	}
	if _, err := parseAmount(json.Number(facts.InternalAmount)); err != nil {
		return Facts{}, err
	}
	if _, err := parseAmount(json.Number(facts.BillAmount)); err != nil {
		return Facts{}, err
	}
	from, err := parseDate(facts.EffectiveFrom)
	if err != nil {
		return Facts{}, err
	}
	if facts.EffectiveUntil != "" {
		until, err := parseDate(facts.EffectiveUntil)
		if err != nil || until <= from {
			return Facts{}, errors.New("effective_until must be a date after effective_from")
		}
	}
	return facts, nil
}

func decide(facts Facts, claimed string, apply bool) (plugins.StepDecision, error) {
	internal, err := parseAmount(json.Number(facts.InternalAmount))
	if err != nil {
		return plugins.StepDecision{}, err
	}
	bill, err := parseAmount(json.Number(facts.BillAmount))
	if err != nil {
		return plugins.StepDecision{}, err
	}
	raw, err := json.Marshal(struct {
		CostUnitNodeID string `json:"cost_unit_node_id"`
		Unit           string `json:"unit"`
		Currency       string `json:"currency"`
		EffectiveFrom  string `json:"effective_from"`
		EffectiveUntil string `json:"effective_until,omitempty"`
		InternalAmount string `json:"internal_amount"`
		BillAmount     string `json:"bill_amount"`
	}{
		CostUnitNodeID: facts.CostUnitNodeID,
		Unit:           facts.Unit,
		Currency:       facts.Currency,
		EffectiveFrom:  facts.EffectiveFrom,
		EffectiveUntil: facts.EffectiveUntil,
		InternalAmount: internal.String(),
		BillAmount:     bill.String(),
	})
	if err != nil {
		return plugins.StepDecision{}, err
	}
	admitted := facts.Live && facts.PersonAdmin && !facts.Overlap
	proceed := admitted && (!apply || claimed == OutcomeRecorded)
	decision := plugins.StepDecision{
		Proceed:         proceed,
		Outcome:         "refused",
		Blocker:         fence.BlockerPolicyRefused,
		Evidence:        raw,
		AdvancesRelease: false,
		AdvancesAccess:  false,
	}
	if proceed {
		decision.Outcome = OutcomeRecorded
		decision.Blocker = ""
	}
	return decision, nil
}
