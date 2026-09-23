// SPDX-License-Identifier: AGPL-3.0-only

package agentd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/inspr-at/aeon/internal/localjournal"
)

type Config struct {
	API               API
	StateRoot         string
	DaemonID          string
	Workspace         string
	Adapters          []Adapter
	EstimatedUnits    map[string]int64
	Accounts          []EnrolledAccount
	HeartbeatInterval time.Duration
	MaxRunDuration    time.Duration
}

type replay struct {
	Digest  string  `json:"digest"`
	Receipt Receipt `json:"receipt"`
}

// Record contains only local process provenance and bounded control digests.
// The journal is AEON v2; classic journals are never opened implicitly.
type Record struct {
	TenantID    string            `json:"tenant_id"`
	PrincipalID string            `json:"principal_id"`
	RunID       string            `json:"run_id"`
	WorkOrderID string            `json:"work_order_id,omitempty"`
	Generation  string            `json:"generation"`
	Workspace   string            `json:"workspace"`
	PID         int               `json:"pid"`
	State       string            `json:"state"`
	Sequence    int64             `json:"sequence"`
	Controls    map[string]replay `json:"controls,omitempty"`
}

type owned struct {
	mu            sync.Mutex
	record        Record
	process       Process
	monitorDone   chan struct{}
	stopRequested bool
}

type Supervisor struct {
	mu                sync.Mutex
	api               API
	journal           *localjournal.Journal[Record]
	lock              *os.File
	adapters          map[string]Adapter
	runs              map[string]*owned
	tenantID          string
	principalID       string
	daemonID          string
	generation        string
	workspace         string
	estimates         map[string]int64
	accounts          []EnrolledAccount
	heartbeatInterval time.Duration
	maxRunDuration    time.Duration
}

func NewSupervisor(ctx context.Context, c Config) (*Supervisor, error) {
	if c.API == nil || c.DaemonID == "" || len(c.DaemonID) > 128 || strings.ContainsAny(c.DaemonID, "/\\\x00\r\n") ||
		c.StateRoot == "" || !filepath.IsAbs(c.StateRoot) {
		return nil, errors.New("invalid daemon configuration")
	}
	physical, err := filepath.EvalSymlinks(c.Workspace)
	if err != nil || !filepath.IsAbs(c.Workspace) || physical != c.Workspace {
		return nil, errors.New("workspace must be an existing physical absolute path")
	}
	info, err := os.Stat(physical)
	if err != nil || !info.IsDir() {
		return nil, errors.New("workspace is not a directory")
	}
	tenantID, principalID, err := c.API.Identity(ctx)
	if err != nil {
		return nil, errors.New("AEON agent identity preflight failed")
	}
	if tenantID == "" || principalID == "" {
		return nil, errors.New("agent identity unavailable")
	}
	gen, err := randomID()
	if err != nil {
		return nil, err
	}
	adapters := make(map[string]Adapter)
	for _, a := range c.Adapters {
		if a == nil || a.Name() == "" || adapters[a.Name()] != nil {
			return nil, errors.New("duplicate or invalid adapter")
		}
		adapters[a.Name()] = a
	}
	if len(adapters) == 0 {
		return nil, errors.New("no adapters configured")
	}
	heartbeat := c.HeartbeatInterval
	if heartbeat == 0 {
		heartbeat = 15 * time.Second
	}
	if heartbeat < time.Second || heartbeat > time.Minute {
		return nil, errors.New("invalid heartbeat interval")
	}
	maxRun := c.MaxRunDuration
	if maxRun == 0 {
		maxRun = 4 * time.Hour
	}
	if maxRun < time.Second || maxRun > 24*time.Hour {
		return nil, errors.New("invalid child duration bound")
	}
	for unit, estimate := range c.EstimatedUnits {
		if (unit != "requests" && unit != "tokens" && unit != "cost_micros") || estimate < 1 {
			return nil, errors.New("invalid allowance estimate")
		}
	}
	for _, account := range c.Accounts {
		if account.ID == "" || account.Key == "" || adapters[account.Harness] == nil {
			return nil, errors.New("invalid local account enrollment")
		}
		if _, ok := adapters[account.Harness].(AccountProber); !ok {
			return nil, errors.New("adapter has no account probe")
		}
	}
	lock, err := acquireInstanceLock(c.StateRoot, c.DaemonID)
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	j, err := localjournal.Open(localjournal.Config[Record]{
		Directory: c.StateRoot, Prefix: "aeon-agentd-" + c.DaemonID, Version: 2,
		MaxBytes: 4 << 20, MaxRecords: 4096,
		Key: func(r Record) (string, error) {
			if r.RunID == "" {
				return "", errors.New("missing run")
			}
			return r.RunID, nil
		},
		Validate: func(r Record) error {
			if r.TenantID != tenantID || r.PrincipalID != principalID || r.Generation == "" || r.RunID == "" || r.Sequence < 0 || len(r.Controls) > 256 {
				return errors.New("invalid AEON journal binding")
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	s := &Supervisor{api: c.API, journal: j, lock: lock, adapters: adapters, runs: map[string]*owned{}, tenantID: tenantID,
		principalID: principalID, daemonID: c.DaemonID, generation: gen, workspace: physical, estimates: c.EstimatedUnits, accounts: c.Accounts,
		heartbeatInterval: heartbeat, maxRunDuration: maxRun}
	for _, rec := range j.Snapshot() {
		// A persisted PID is never proof of ownership after a restart.
		if rec.State == "running" || rec.State == "starting" || rec.State == "waiting" {
			rec.State = "ownership_lost"
			if err := j.Put(rec); err != nil {
				return nil, err
			}
		}
		s.runs[rec.RunID] = &owned{record: rec}
	}
	keepLock = true
	return s, nil
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Supervisor) Generation() string  { return s.generation }
func (s *Supervisor) DaemonID() string    { return s.daemonID }
func (s *Supervisor) TenantID() string    { return s.tenantID }
func (s *Supervisor) PrincipalID() string { return s.principalID }

func (s *Supervisor) Status() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.runs))
	for _, entry := range s.runs {
		entry.mu.Lock()
		view := entry.record
		view.Workspace = ""
		view.Controls = nil
		out = append(out, view)
		entry.mu.Unlock()
	}
	return out
}

// PollOnce fetches queued work and pending inbox messages. A missing or stale
// reservation fails closed and leaves the run queued for later reconciliation.
func (s *Supervisor) PollOnce(ctx context.Context) error {
	for _, account := range s.accounts {
		probe := s.adapters[account.Harness].(AccountProber)
		available := probe.Probe(ctx, account.Key)
		if err := s.api.Probe(ctx, account.ID, s.daemonID, s.generation, available); err != nil {
			return err
		}
	}
	runs, err := s.api.Queued(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.AgentPrincipalID != s.principalID {
			return ErrScope
		}
		if err := s.StartRun(ctx, run); err != nil {
			return err
		}
	}
	return s.DeliverInbox(ctx)
}

func (s *Supervisor) StartRun(ctx context.Context, run Run) error {
	if run.ID == "" || run.WorkOrderID == "" || run.AgentPrincipalID != s.principalID || run.Status != "queued" {
		return ErrScope
	}
	s.mu.Lock()
	if s.runs[run.ID] != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	profiles, err := s.api.Profiles(ctx)
	if err != nil {
		return err
	}
	var profile Profile
	for _, p := range profiles {
		if p.ID == run.ModelProfileID {
			profile = p
			break
		}
	}
	adapter := s.adapters[profile.Harness]
	if adapter == nil || profile.ID == "" {
		return ErrUnsupported
	}
	node, err := s.api.Node(ctx, run.WorkOrderID)
	if err != nil {
		return err
	}
	if node.ID != run.WorkOrderID || strings.TrimSpace(node.Title) == "" {
		return errors.New("work order node unavailable")
	}
	order, err := s.api.WorkOrder(ctx, run.WorkOrderID)
	if err != nil {
		return err
	}
	if order.NodeID != run.WorkOrderID || (order.Status != "ready" && order.Status != "running") {
		return errors.New("work order is not dispatchable")
	}
	duration := s.maxRunDuration
	if order.MaxDurationSeconds != nil {
		if *order.MaxDurationSeconds <= 0 {
			return errors.New("work order duration invalid")
		}
		if *order.MaxDurationSeconds < int64(duration/time.Second) {
			duration = time.Duration(*order.MaxDurationSeconds) * time.Second
		}
	}
	prompt := node.Title
	if node.Body != "" {
		prompt += "\n\n" + node.Body
	}
	if len(prompt) > 256<<10 {
		return errors.New("work order prompt exceeds local bound")
	}
	if len(s.estimates) == 0 {
		return errors.New("allowance estimates required")
	}
	route, err := s.api.Route(ctx, run.ID, s.estimates)
	if err != nil {
		return err
	}
	if route.DaemonID != s.daemonID || route.AccountKey == "" || len(route.Reservations) == 0 {
		return errors.New("account route is not bound to daemon")
	}
	localBinding := false
	for _, account := range s.accounts {
		if account.ID == route.AccountID && account.Key == route.AccountKey && account.Harness == profile.Harness {
			localBinding = true
			break
		}
	}
	if !localBinding {
		return errors.New("account route has no matching local enrollment")
	}
	ids := make([]string, 0, len(route.Reservations))
	for _, reservation := range route.Reservations {
		if reservation.ID == "" {
			return errors.New("empty account reservation")
		}
		ids = append(ids, reservation.ID)
	}
	if err := s.api.Claim(ctx, run.ID, s.daemonID, s.generation, ids); err != nil {
		return err
	}
	rec := Record{TenantID: s.tenantID, PrincipalID: s.principalID, RunID: run.ID, WorkOrderID: run.WorkOrderID, Generation: s.generation, Workspace: s.workspace, State: "starting", Controls: map[string]replay{}}
	if err := s.journal.Put(rec); err != nil {
		return err
	}
	entry := &owned{record: rec}
	s.mu.Lock()
	s.runs[run.ID] = entry
	s.mu.Unlock()
	observe := func(ev AdapterEvent) { s.observe(entry, ev) }
	proc, err := adapter.Start(ctx, StartRequest{TenantID: s.tenantID, PrincipalID: s.principalID, Run: run, Profile: profile,
		AccountKey: route.AccountKey, Workspace: s.workspace, StateRoot: filepath.Dir(s.journal.JournalPath()), Prompt: prompt, Generation: s.generation}, observe)
	if err != nil {
		_ = s.update(ctx, entry, Telemetry{Kind: "finished", Status: "failed", ErrorCode: "child_exit_failed"})
		return err
	}
	entry.mu.Lock()
	entry.process = proc
	entry.record.PID = proc.PID()
	entry.record.State = "running"
	saveErr := s.journal.Put(entry.record)
	entry.mu.Unlock()
	if saveErr != nil {
		_ = proc.Stop(ctx)
		return saveErr
	}
	if err := s.update(ctx, entry, Telemetry{Kind: "started", Status: "running"}); err != nil {
		_ = proc.Stop(ctx)
		return err
	}
	entry.mu.Lock()
	entry.monitorDone = make(chan struct{})
	entry.mu.Unlock()
	go s.monitor(entry)
	go s.heartbeat(entry, duration)
	return nil
}

func (s *Supervisor) heartbeat(entry *owned, duration time.Duration) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	entry.mu.Lock()
	done := entry.monitorDone
	entry.mu.Unlock()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := s.update(ctx, entry, Telemetry{Kind: "heartbeat"})
			cancel()
			if err != nil {
				entry.mu.Lock()
				proc := entry.process
				entry.mu.Unlock()
				if proc != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = proc.Stop(ctx)
					cancel()
				}
				return
			}
		case <-deadline.C:
			entry.mu.Lock()
			proc := entry.process
			entry.stopRequested = true
			entry.mu.Unlock()
			if proc != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = proc.Stop(ctx)
				cancel()
			}
			return
		}
	}
}

func (s *Supervisor) observe(entry *owned, ev AdapterEvent) {
	if ev.Kind == "" {
		return
	}
	kind := ev.Kind
	if kind != "turn" && kind != "tool" && kind != "usage" && kind != "heartbeat" {
		kind = "status"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.update(ctx, entry, Telemetry{Kind: kind, InputTokensDelta: ev.InputTokensDelta,
		OutputTokensDelta: ev.OutputTokensDelta, CostMicrosDelta: ev.CostMicrosDelta,
		EffectiveModel: ev.EffectiveModel, ModelEvidence: ev.ModelEvidence, ErrorCode: ev.ErrorCode}); err != nil {
		entry.mu.Lock()
		proc := entry.process
		entry.mu.Unlock()
		if proc != nil {
			_ = proc.Stop(ctx)
		}
	}
}

func (s *Supervisor) monitor(entry *owned) {
	entry.mu.Lock()
	proc := entry.process
	done := entry.monitorDone
	entry.mu.Unlock()
	if done != nil {
		defer close(done)
	}
	if proc == nil {
		return
	}
	err := proc.Wait()
	entry.mu.Lock()
	stopped := entry.stopRequested
	entry.mu.Unlock()
	if err == nil {
		if evidence, ok := proc.(EvidenceProcess); ok {
			answer := evidence.Evidence()
			if answer == "" {
				err = errors.New("agent output unavailable")
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err = s.api.AddEvidence(ctx, entry.record.WorkOrderID, entry.record.RunID, answer)
				cancel()
			}
		}
	}
	status, code := "completed", ""
	if err != nil {
		status, code = "failed", "child_exit_failed"
	}
	if stopped {
		status, code = "cancelled", ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.update(ctx, entry, Telemetry{Kind: "finished", Status: status, ErrorCode: code})
}

func (s *Supervisor) update(ctx context.Context, entry *owned, t Telemetry) error {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.record.Generation != s.generation {
		return ErrGeneration
	}
	if t.Kind == "heartbeat" && (entry.record.State == "completed" || entry.record.State == "failed" || entry.record.State == "cancelled" || entry.record.State == "ownership_lost") {
		return nil
	}
	t.Sequence = entry.record.Sequence + 1
	if err := s.api.Report(ctx, entry.record.RunID, t); err != nil {
		return err
	}
	entry.record.Sequence = t.Sequence
	if t.Status != "" {
		entry.record.State = t.Status
	}
	return s.journal.Put(entry.record)
}

func (s *Supervisor) Control(ctx context.Context, req ControlRequest) (Receipt, error) {
	if req.TenantID != s.tenantID || req.PrincipalID != s.principalID || req.Generation != s.generation ||
		req.RunID == "" || req.CorrelationID == "" || len(req.CorrelationID) > 128 {
		return Receipt{}, ErrScope
	}
	if req.Operation != "steer" && req.Operation != "interrupt" && req.Operation != "resume" && req.Operation != "stop" {
		return Receipt{}, ErrUnsupported
	}
	if len(req.Text) > 64<<10 || (req.Operation != "steer" && req.Text != "") {
		return Receipt{}, errors.New("invalid control body")
	}
	s.mu.Lock()
	entry := s.runs[req.RunID]
	s.mu.Unlock()
	if entry == nil {
		return Receipt{}, ErrNotOwned
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.record.Generation != s.generation || entry.process == nil || entry.record.State != "running" {
		return Receipt{}, ErrNotOwned
	}
	digest := sha256.Sum256([]byte(req.Operation + "\x00" + req.Text))
	key := hex.EncodeToString(digest[:])
	if prior, ok := entry.record.Controls[req.CorrelationID]; ok {
		if prior.Digest != key {
			return Receipt{}, ErrReplay
		}
		return prior.Receipt, nil
	}
	if len(entry.record.Controls) >= 256 {
		return Receipt{}, errors.New("control replay capacity reached")
	}
	var err error
	if req.Operation == "stop" {
		entry.stopRequested = true
		err = entry.process.Stop(ctx)
		if err != nil {
			entry.stopRequested = false
		}
	} else {
		err = entry.process.Control(ctx, req.Operation, req.Text)
	}
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{RunID: req.RunID, Generation: s.generation, CorrelationID: req.CorrelationID, Operation: req.Operation, AppliedAt: time.Now().UTC()}
	entry.record.Controls[req.CorrelationID] = replay{Digest: key, Receipt: receipt}
	if err := s.journal.Put(entry.record); err != nil {
		return Receipt{}, fmt.Errorf("persist control receipt: %w", err)
	}
	return receipt, nil
}

func (s *Supervisor) Close(ctx context.Context) error {
	s.mu.Lock()
	entries := make([]*owned, 0, len(s.runs))
	for _, e := range s.runs {
		entries = append(entries, e)
	}
	s.mu.Unlock()
	var first error
	for _, e := range entries {
		e.mu.Lock()
		proc := e.process
		done := e.monitorDone
		e.mu.Unlock()
		if proc != nil {
			e.mu.Lock()
			e.stopRequested = true
			e.mu.Unlock()
			if err := proc.Stop(ctx); err != nil {
				e.mu.Lock()
				e.stopRequested = false
				e.mu.Unlock()
				if first == nil {
					first = err
				}
			}
		}
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				if first == nil {
					first = ctx.Err()
				}
			}
		}
	}
	if s.lock != nil {
		if err := s.lock.Close(); err != nil && first == nil {
			first = err
		}
		s.lock = nil
	}
	return first
}
