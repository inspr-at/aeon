// SPDX-License-Identifier: AGPL-3.0-only
package quotes

import (
	"bytes"
	"fmt"
	"testing"
)

// The reader caps the whole tar stream at maxProfileBundle+1 bytes, so the
// writer must count tar headers and padding too: a bundle whose contents fit
// but whose tar does not must be refused before anything is written.
func TestWriteProfileBundleTarCountsTarOverhead(t *testing.T) {
	profile := []byte(`{"schema":"inspr.document-profile.v1"}`)
	const fileSize = 10 << 20 // the per-file maximum
	files := func(n int, last int) map[string][]byte {
		out := map[string][]byte{}
		for i := 0; i < n; i++ {
			out[fmt.Sprintf("assets/f%02d.bin", i)] = make([]byte, fileSize)
		}
		if last > 0 {
			out["assets/last.bin"] = make([]byte, last)
		}
		return out
	}
	// Six full files leave room for a last file whose contents alone fit the
	// limit but whose tar entry (header and padding) pushes the stream over it.
	room := maxProfileBundle - 6*tarEntrySize(fileSize) - tarEntrySize(len(profile)) - 1024
	contentsOnly := int(room) // fits by contents, overflows once its 512-byte header is added
	var buf bytes.Buffer
	if err := WriteProfileBundleTar(&buf, ProfileBundle{Profile: profile, Files: files(6, contentsOnly)}); err == nil {
		t.Fatal("a bundle whose tar exceeds the reader limit was written")
	}
	buf.Reset()
	fits := int(room) - 512 - 511
	if err := WriteProfileBundleTar(&buf, ProfileBundle{Profile: profile, Files: files(6, fits)}); err != nil {
		t.Fatalf("a bundle within the limit was refused: %v", err)
	}
	if int64(buf.Len()) > maxProfileBundle {
		t.Fatalf("written tar is %d bytes, over the %d limit", buf.Len(), maxProfileBundle)
	}
	back, err := ReadProfileBundleTar(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reader rejected a bundle the writer accepted: %v", err)
	}
	if len(back.Files) != 7 {
		t.Fatalf("read %d files, want 7", len(back.Files))
	}
}
