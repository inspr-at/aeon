// SPDX-License-Identifier: AGPL-3.0-only

package crm

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
)

const (
	// ID is the compiled plugin id.
	ID = "business_crm"
	// Version is the compiled manifest version.
	Version = "2"
	// Owner is the accountable first-party package.
	Owner = "internal/business/crm"
	// OperationBind is the workflow step plugins.Enabled checks for a binding.
	OperationBind = "crm_bind"
	// EventContactBound is the tenant event appended when a binding is created.
	EventContactBound = "crm.contact_bound"
	viewID            = "crm"
)

// Plugin returns the compiled business_crm manifest. The coordinator registers
// it and passes the sealed registry to New. DigestSHA256 is the canonical
// digest; a mismatched installation fails closed.
func Plugin() (plugins.Plugin, error) {
	contactSchema, err := schema("urn:aeon:business_crm:contact", map[string]any{
		"email":             map[string]any{"type": "string", "maxLength": 320},
		"phone":             map[string]any{"type": "string", "maxLength": 100},
		"role":              map[string]any{"type": "string", "maxLength": 200},
		"note":              map[string]any{"type": "string", "maxLength": 20000},
		"external_provider": map[string]any{"type": "string", "maxLength": 100},
		"external_id":       map[string]any{"type": "string", "maxLength": 500},
		"external_url":      map[string]any{"type": "string", "maxLength": 1000},
	})
	if err != nil {
		return plugins.Plugin{}, err
	}
	orgSchema, err := schema("urn:aeon:business_crm:organisation", map[string]any{
		"legal_name":           map[string]any{"type": "string", "maxLength": 200},
		"website":              map[string]any{"type": "string", "maxLength": 500},
		"industry":             map[string]any{"type": "string", "maxLength": 200},
		"domain":               map[string]any{"type": "string", "maxLength": 255},
		"phone":                map[string]any{"type": "string", "maxLength": 100},
		"description":          map[string]any{"type": "string", "maxLength": 20000},
		"customer_notes":       map[string]any{"type": "string", "maxLength": 20000},
		"vat_id":               map[string]any{"type": "string", "maxLength": 100},
		"tax_id":               map[string]any{"type": "string", "maxLength": 100},
		"register_no":          map[string]any{"type": "string", "maxLength": 100},
		"employee_count":       map[string]any{"type": []string{"integer", "null"}, "minimum": 0},
		"annual_revenue_minor": map[string]any{"type": []string{"integer", "null"}, "minimum": 0},
		"currency":             map[string]any{"type": "string", "maxLength": 3},
		"billing_address":      map[string]any{"type": []string{"object", "null"}},
		"visiting_address":     map[string]any{"type": []string{"object", "null"}},
		"hourly_rate_minor":    map[string]any{"type": []string{"integer", "null"}, "minimum": 0},
		"lp_rate_minor":        map[string]any{"type": []string{"integer", "null"}, "minimum": 0},
		"external_provider":    map[string]any{"type": "string", "maxLength": 100},
		"external_id":          map[string]any{"type": "string", "maxLength": 500},
		"external_url":         map[string]any{"type": "string", "maxLength": 1000},
	})
	if err != nil {
		return plugins.Plugin{}, err
	}
	kinds := []plugins.NodeKind{
		{Slug: Contact, FieldSchema: contactSchema, AllowedChildKinds: []string{}},
		{Slug: Organisation, FieldSchema: orgSchema, AllowedChildKinds: []string{}},
	}
	views := []plugins.View{{ID: viewID, Panels: []string{"organisations", "contacts", "links"}}}
	h := host{kinds: kinds, views: views}
	p := plugins.Plugin{
		Manifest: plugins.Manifest{
			ID:      ID,
			Version: Version,
			Owner:   Owner,
			Permissions: []string{
				fence.PermNodesContribute,
				fence.PermViewsProvide,
				fence.PermStepsApply,
				fence.PermIntegrationsCall,
			},
			NodeKinds:      kinds,
			Views:          views,
			WorkflowSteps:  []plugins.WorkflowStep{{Key: OperationBind, Gates: []string{fence.GateObservedState, fence.GatePersonDecision}}},
			AgentTools:     []plugins.Capability{},
			Integrations:   []plugins.Capability{{ID: "crm_provider", Permission: fence.PermIntegrationsCall}},
			BackgroundJobs: []plugins.Capability{},
		},
		StepPermissions: map[string]string{OperationBind: fence.PermStepsApply},
		Kinds:           h,
		Views:           h,
		Steps:           bindStep{},
		Integrations:    providerIntegration{},
	}
	sum, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = sum
	return p, nil
}

func schema(id string, properties map[string]any) (json.RawMessage, error) {
	return canonical(map[string]any{
		"$id":                  id,
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	})
}

func canonical(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}

type host struct {
	kinds []plugins.NodeKind
	views []plugins.View
}

func (h host) NodeKinds(ctx context.Context, call plugins.Call) ([]plugins.NodeKind, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !call.Grant.Allows(fence.PermNodesContribute) {
		return nil, plugins.ErrDenied
	}
	return copyKinds(h.kinds), nil
}

func (h host) Views(ctx context.Context, call plugins.Call) ([]plugins.View, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !call.Grant.Allows(fence.PermViewsProvide) {
		return nil, plugins.ErrDenied
	}
	return copyViews(h.views), nil
}

func copyKinds(in []plugins.NodeKind) []plugins.NodeKind {
	out := make([]plugins.NodeKind, len(in))
	for i, kind := range in {
		kind.FieldSchema = append(json.RawMessage(nil), kind.FieldSchema...)
		kind.AllowedChildKinds = append([]string(nil), kind.AllowedChildKinds...)
		out[i] = kind
	}
	return out
}

func copyViews(in []plugins.View) []plugins.View {
	out := make([]plugins.View, len(in))
	for i, view := range in {
		view.Panels = append([]string(nil), view.Panels...)
		out[i] = view
	}
	return out
}
