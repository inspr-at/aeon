// SPDX-License-Identifier: AGPL-3.0-only

// Package search serves GET /api/search.
//
// New returns the httpapi.Module the coordinator mounts. This package does
// not edit cmd/aeon.
//
//	srv.Modules = append(srv.Modules, search.New(pool, provider))
//
// provider is an embedding.Provider and may be nil. German and English
// full-text ranks and pgvector cosine ranks are fused inside aeon_search_nodes
// (reciprocal rank fusion, k=60). A nil provider, or a provider error, passes
// a NULL query vector so the same statement stays lexical-only. Queued
// embedding jobs hide stale vectors inside that function. Pass the same
// provider to embedding.NewWorker so stored vectors and query vectors share
// a model, and start the queue with Worker.Run.
//
// Cursors are opaque and bound to the tenant, q, kind_id and state. The
// module pages the function's top 200 fused rows by score descending, then
// node id ascending.
package search
