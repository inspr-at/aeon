// SPDX-License-Identifier: AGPL-3.0-only

// Package embedding fills node_embeddings from node_embedding_jobs.
//
// Node title and body writes enqueue a job in the database. NewWorker claims
// those jobs inside db.InTenant, embeds the document (title, a newline, then
// body) with a Provider, then in one tenant transaction checks the content
// hash, upserts the 1536-wide halfvec and deletes the job. A job whose
// queued_at moved, or whose hash no longer matches, is not overwritten with
// the stale vector. Queued jobs hide the previous vector from aeon_search_nodes.
//
// Storing an embedding appends one node.embedded event in that same
// transaction. appendEvent is the stand-in for events.Writer.Append (the
// P1.2 writer: Append(ctx, tx, principal, Change) inside the caller's
// db.InTenant callback). Options.Append replaces it once internal/events is
// on this branch. The actor is the tenant agent named aeon-embedding.
//
// The coordinator starts the queue with Worker.Run. This package does not
// mount routes and does not edit cmd/aeon. FromEnv reads AEON_EMBEDDING_URL
// (full OpenAI-compatible embeddings URL; empty disables the provider),
// AEON_EMBEDDING_MODEL (default text-embedding-3-small) and
// AEON_EMBEDDING_API_KEY (optional bearer token, never logged).
package embedding
