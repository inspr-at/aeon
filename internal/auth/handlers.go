// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/inspr-at/aeon/internal/tenant"
)

var (
	errShortKey          = errors.New("session key must be at least 32 bytes")
	errOIDCNotConfigured = errors.New("oidc is not configured")
)

func (m *Module) homeURL() string {
	if m.cfg.PublicURL == "" {
		return "/"
	}
	return m.cfg.PublicURL + "/"
}

func (m *Module) handleLogin(w http.ResponseWriter, r *http.Request) {
	_, oc, err := m.oidcProvider(r.Context())
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	state, err := randomString(16)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	nonce, err := randomString(16)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	verifier := oauth2.GenerateVerifier()
	payload := oidcPayload{State: state, Nonce: nonce, Verifier: verifier, Exp: time.Now().Add(oidcTTL).Unix()}
	if err := m.setOIDCCookie(w, payload); err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, oc.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (m *Module) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("error") != "" || q.Get("code") == "" {
		m.clearOIDCCookie(w)
		writeHTML(w, http.StatusBadRequest, signInFailedPage)
		return
	}
	payload, err := m.readOIDCCookie(r)
	if err != nil || !hmacEqual(payload.State, q.Get("state")) {
		m.clearOIDCCookie(w)
		writeHTML(w, http.StatusBadRequest, signInFailedPage)
		return
	}
	m.clearOIDCCookie(w)
	provider, oc, err := m.oidcProvider(r.Context())
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	tok, err := oc.Exchange(r.Context(), q.Get("code"), oauth2.VerifierOption(payload.Verifier))
	if err != nil {
		writeHTML(w, http.StatusBadRequest, signInFailedPage)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := provider.Verifier(&oidc.Config{ClientID: m.cfg.OIDCClientID}).Verify(r.Context(), rawID)
	if err != nil || idt.Subject == "" || !hmacEqual(idt.Nonce, payload.Nonce) {
		writeHTML(w, http.StatusBadRequest, signInFailedPage)
		return
	}
	var claims struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		Preferred string `json:"preferred_username"`
	}
	if err := idt.Claims(&claims); err != nil {
		writeHTML(w, http.StatusBadRequest, signInFailedPage)
		return
	}
	display := firstNonEmpty(claims.Name, claims.Preferred, claims.Email, idt.Subject)
	identityID, err := m.upsertIdentity(r.Context(), idt.Issuer, idt.Subject, claims.Email, display)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	tenantID, err := m.bootstrapTenant(r.Context())
	if errors.Is(err, errNoTenant) {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	principal, err := m.ensurePerson(r.Context(), tenantID, identityID, claims.Email, display)
	if errors.Is(err, errNotMember) {
		writeHTML(w, http.StatusForbidden, notMemberPage)
		return
	}
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	token, err := m.startSession(r.Context(), identityID, principal.TenantID, principal.ID)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, notReadyPage)
		return
	}
	m.setSessionCookie(w, token)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, m.homeURL(), http.StatusFound)
}

func (m *Module) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if raw, err := decodeSessionToken(c.Value); err == nil {
			if err := m.deleteSession(r.Context(), raw); err != nil {
				writeInternal(w)
				return
			}
		}
	}
	m.clearSessionCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) handleMe(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	view, err := m.loadMe(r.Context(), p)
	if errors.Is(err, pgx.ErrNoRows) {
		writeUnauthorized(w)
		return
	}
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, meJSONFrom(view))
}

func (m *Module) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !m.cfg.Dev() {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	email := strings.TrimSpace(body.Email)
	if !validEmail(email) {
		writeBadRequest(w, "email is required")
		return
	}
	tenantID, err := m.bootstrapTenant(r.Context())
	if errors.Is(err, errNoTenant) {
		writeJSON(w, http.StatusInternalServerError, errorJSON{Error: "workspace is not ready"})
		return
	}
	if err != nil {
		writeInternal(w)
		return
	}
	principal, identityID, err := m.personByEmail(r.Context(), tenantID, email)
	if errors.Is(err, pgx.ErrNoRows) {
		if !adminEmail(email, m.cfg.BootstrapAdminEmail) {
			writeJSON(w, http.StatusForbidden, errorJSON{Error: notMemberSentence})
			return
		}
		identityID, err = m.upsertIdentity(r.Context(), devIssuer, strings.ToLower(email), email, email)
		if err != nil {
			writeInternal(w)
			return
		}
		principal, err = m.ensurePerson(r.Context(), tenantID, identityID, email, email)
	}
	if errors.Is(err, errNotMember) {
		writeJSON(w, http.StatusForbidden, errorJSON{Error: notMemberSentence})
		return
	}
	if err != nil {
		writeInternal(w)
		return
	}
	token, err := m.startSession(r.Context(), identityID, principal.TenantID, principal.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	m.setSessionCookie(w, token)
	view, err := m.loadMe(r.Context(), principal)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, meJSONFrom(view))
}

func hmacEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func validEmail(s string) bool {
	if s == "" || len(s) > 320 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	return strings.Count(s, "@") == 1 && !strings.HasPrefix(s, "@") && !strings.HasSuffix(s, "@")
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeBadRequest(w, "bad request")
		return false
	}
	return true
}
