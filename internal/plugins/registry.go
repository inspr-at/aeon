// SPDX-License-Identifier: AGPL-3.0-only

// Package plugins registers compiled first-party capabilities and tenant installations.
package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Manifest is the startup-time ceiling for a compiled plugin. DigestSHA256 is
// the SHA-256 of its JSON representation with DigestSHA256 empty.
type Manifest struct {
	ID             string         `json:"id"`
	Version        string         `json:"version"`
	DigestSHA256   string         `json:"digest_sha256"`
	Owner          string         `json:"owner"`
	Permissions    []string       `json:"permissions"`
	NodeKinds      []NodeKind     `json:"node_kinds"`
	Views          []View         `json:"views"`
	WorkflowSteps  []WorkflowStep `json:"workflow_steps"`
	AgentTools     []Capability   `json:"agent_tools"`
	Integrations   []Capability   `json:"integrations"`
	BackgroundJobs []Capability   `json:"background_jobs"`
}
type NodeKind struct {
	Slug              string          `json:"slug"`
	FieldSchema       json.RawMessage `json:"field_schema"`
	AllowedChildKinds []string        `json:"allowed_child_kinds,omitempty"`
}
type View struct {
	ID     string   `json:"id"`
	Panels []string `json:"panels"`
}
type WorkflowStep struct {
	Key   string   `json:"key"`
	Gates []string `json:"gates"`
}
type Capability struct {
	ID         string `json:"id"`
	Permission string `json:"permission"`
}

// Capabilities contains only installed, declared permissions, never a DB pool.
type Capabilities struct{ allowed map[string]struct{} }

func Narrow(allowed []string) Capabilities {
	c := Capabilities{allowed: map[string]struct{}{}}
	for _, p := range allowed {
		c.allowed[p] = struct{}{}
	}
	return c
}
func (c Capabilities) Has(permission string) bool { _, ok := c.allowed[permission]; return ok }

type Call struct {
	TenantID     string
	PrincipalID  string
	Capabilities Capabilities
}
type NodeKindContributor interface {
	NodeKinds(context.Context, Call) []NodeKind
}
type ViewProvider interface {
	Views(context.Context, Call) []View
}
type StepPlugin interface {
	Evaluate(context.Context, Call, string) error
	Request(context.Context, Call, string) error
	ApplyResult(context.Context, Call, string) error
}
type ToolProvider interface {
	AgentTools(context.Context, Call) []Capability
}
type IntegrationProvider interface {
	Integrations(context.Context, Call) []Capability
}
type JobProvider interface {
	BackgroundJobs(context.Context, Call) []Capability
}

type Registry struct {
	entries      map[string]Manifest
	schemaOwners map[string]string
	steps        map[string]StepPlugin
}

func NewRegistry() *Registry {
	return &Registry{entries: map[string]Manifest{}, schemaOwners: map[string]string{}, steps: map[string]StepPlugin{}}
}

func Digest(m Manifest) (string, error) {
	m.DigestSHA256 = ""
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func Seal(m Manifest) Manifest {
	digest, err := Digest(m)
	if err != nil {
		panic(err)
	}
	m.DigestSHA256 = digest
	return m
}
func (r *Registry) Register(m Manifest) error {
	if !nameRE.MatchString(m.ID) || m.Version == "" || m.Owner == "" {
		return errors.New("invalid plugin identity")
	}
	if _, ok := r.entries[m.ID]; ok {
		return fmt.Errorf("duplicate plugin %s", m.ID)
	}
	d, err := Digest(m)
	if err != nil {
		return err
	}
	if m.DigestSHA256 != d {
		return fmt.Errorf("plugin %s digest mismatch", m.ID)
	}
	perms := map[string]bool{}
	for _, p := range m.Permissions {
		if !nameRE.MatchString(p) || perms[p] {
			return fmt.Errorf("invalid or duplicate permission %s", p)
		}
		perms[p] = true
	}
	claims := map[string]bool{}
	claim := func(group, id string) error {
		if id == "" || claims[group+":"+id] {
			return fmt.Errorf("duplicate %s %s", group, id)
		}
		claims[group+":"+id] = true
		return nil
	}
	for _, k := range m.NodeKinds {
		if err := claim("kind", k.Slug); err != nil {
			return err
		}
		if !json.Valid(k.FieldSchema) {
			return fmt.Errorf("invalid schema %s", k.Slug)
		}
		var schema map[string]any
		if err := json.Unmarshal(k.FieldSchema, &schema); err != nil || schema == nil {
			return fmt.Errorf("invalid schema %s", k.Slug)
		}
		if owner := r.schemaOwners[k.Slug]; owner != "" {
			return fmt.Errorf("schema collision %s with %s", k.Slug, owner)
		}
	}
	for _, v := range m.Views {
		if err := claim("view", v.ID); err != nil {
			return err
		}
	}
	for _, s := range m.WorkflowSteps {
		if err := claim("step", s.Key); err != nil {
			return err
		}
	}
	for _, set := range [][]Capability{m.AgentTools, m.Integrations, m.BackgroundJobs} {
		for _, c := range set {
			if !perms[c.Permission] {
				return fmt.Errorf("unknown permission %s", c.Permission)
			}
			if err := claim("capability", c.ID); err != nil {
				return err
			}
		}
	}
	for _, k := range m.NodeKinds {
		r.schemaOwners[k.Slug] = m.ID
	}
	r.entries[m.ID] = m
	return nil
}
func (r *Registry) Get(id string) (Manifest, bool) { m, ok := r.entries[id]; return m, ok }
func (r *Registry) List() []Manifest {
	out := make([]Manifest, 0, len(r.entries))
	for _, m := range r.entries {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// BindStep attaches a compiled implementation after its manifest is registered.
// It cannot enlarge the manifest's declared permissions.
func (r *Registry) BindStep(id string, step StepPlugin) error {
	if _, ok := r.entries[id]; !ok || step == nil {
		return fmt.Errorf("unregistered step plugin %s", id)
	}
	if _, ok := r.steps[id]; ok {
		return fmt.Errorf("step plugin %s already bound", id)
	}
	r.steps[id] = step
	return nil
}
func (r *Registry) Step(id string) (StepPlugin, bool) { step, ok := r.steps[id]; return step, ok }
