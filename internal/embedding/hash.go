// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"crypto/sha256"
	"encoding/hex"
)

// Document is the text embedded for a node: title, a newline, then body.
func Document(title, body string) string {
	return title + "\n" + body
}

// ContentHash is the lowercase SHA-256 hex of Document. It matches the
// node_embeddings.content_hash check (64 hex characters).
func ContentHash(title, body string) string {
	sum := sha256.Sum256([]byte(Document(title, body)))
	return hex.EncodeToString(sum[:])
}
