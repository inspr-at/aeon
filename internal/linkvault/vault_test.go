// SPDX-License-Identifier: AGPL-3.0-only

package linkvault

import (
	"bytes"
	"strings"
	"testing"
)

func TestCiphertextIsRandomAndBoundToLink(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	token := strings.Repeat("A", 43)
	a, err := Encrypt(key, "tenant", "link-a", token)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(key, "tenant", "link-a", token)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) || bytes.Contains(a, []byte(token)) {
		t.Fatal("token exposed or nonce reused")
	}
	plain, err := Decrypt(key, "tenant", "link-a", a)
	if err != nil || plain != token {
		t.Fatal("round trip failed")
	}
	if _, err := Decrypt(key, "tenant", "link-b", a); err == nil {
		t.Fatal("ciphertext moved between links")
	}
	if _, err := Decrypt(bytes.Repeat([]byte{8}, 32), "tenant", "link-a", a); err == nil {
		t.Fatal("wrong key accepted")
	}
	a[len(a)-1] ^= 1
	if _, err := Decrypt(key, "tenant", "link-a", a); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
