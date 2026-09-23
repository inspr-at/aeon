// SPDX-License-Identifier: AGPL-3.0-only

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMe(t *testing.T) {
	const token = "aeon_prefix_secretvalue"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("authorization header mismatch")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Me{
			Principal: Principal{ID: "p1", TenantID: "t1", Kind: "agent", Name: "cursor-grok", Roles: []string{"admin"}},
			Tenant:    Tenant{ID: "t1", Slug: "aeon", Name: "Aeon"},
		})
	}))
	defer srv.Close()

	me, err := New(srv.URL, token).Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Principal.Name != "cursor-grok" || me.Principal.Kind != "agent" || me.Tenant.Slug != "aeon" {
		t.Fatalf("me = %+v", me)
	}
	if me.Identity != nil {
		t.Fatal("expected null identity")
	}
}

func TestMeUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, "aeon_prefix_secretvalue").Me(context.Background())
	se, ok := err.(*StatusError)
	if !ok || se.Status != http.StatusUnauthorized || se.Message != "unauthorized" {
		t.Fatalf("err = %#v", err)
	}
	if strings.Contains(err.Error(), "secretvalue") {
		t.Fatal("error included the token")
	}
}

func TestDoPostsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["title"] != "hello" {
			t.Errorf("body %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	var out struct {
		ID string `json:"id"`
	}
	err := New(srv.URL+"/", "k").Do(context.Background(), http.MethodPost, "/api/items", map[string]string{"title": "hello"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != "1" {
		t.Fatalf("id %q", out.ID)
	}
}
