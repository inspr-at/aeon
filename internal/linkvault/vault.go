// SPDX-License-Identifier: AGPL-3.0-only

// Package linkvault encrypts retained public quote capabilities. Verifiers
// remain SHA-256 hashes in quote_public_links, never ciphertext lookups.
package linkvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

const version byte = 1

func associatedData(tenantID, linkID string) []byte {
	return []byte(tenantID + "/" + linkID)
}

// Encrypt binds a token to its tenant and link. The stored bytes are
// version || nonce || ciphertext || GCM tag.
func Encrypt(key []byte, tenantID, linkID, token string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	result := append([]byte{version}, nonce...)
	return aead.Seal(result, nonce, []byte(token), associatedData(tenantID, linkID)), nil
}

// Decrypt rejects unknown formats, wrong keys and ciphertext moved between
// links. Its errors intentionally contain no token or ciphertext bytes.
func Decrypt(key []byte, tenantID, linkID string, stored []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("invalid link key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(stored) < 1+aead.NonceSize()+aead.Overhead() || stored[0] != version {
		return "", errors.New("invalid link ciphertext")
	}
	nonce := stored[1 : 1+aead.NonceSize()]
	plain, err := aead.Open(nil, nonce, stored[1+aead.NonceSize():], associatedData(tenantID, linkID))
	if err != nil {
		return "", errors.New("link cannot be decrypted")
	}
	return string(plain), nil
}
