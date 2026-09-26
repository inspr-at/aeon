// SPDX-License-Identifier: AGPL-3.0-only
package quotes

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// profileBundleEpoch is the modification time of every entry a bundle tar
// carries, so the same sources always give the same bytes (AEON-155).
var profileBundleEpoch = time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

// WriteProfileBundleTar writes bundle as the tar `aeon quote-profile apply
// --bundle -` reads: profile.json first, then every other file in path order,
// regular files only, with a fixed mode, owner and time and no directory
// entries. The same bundle always yields the same bytes.
func WriteProfileBundleTar(w io.Writer, bundle ProfileBundle) error {
	if len(bundle.Profile) == 0 || len(bundle.Profile) > 1<<20 {
		return errors.New("bundle requires profile.json (at most 1 MiB)")
	}
	names := make([]string, 0, len(bundle.Files))
	// ReadProfileBundleTar reads at most maxProfileBundle+1 bytes of the whole
	// stream, so the limit applies to the tar as written: a 512-byte header per
	// file, contents padded to 512 bytes, and the 1024-byte end marker.
	total := tarEntrySize(len(bundle.Profile)) + 1024
	for name, data := range bundle.Files {
		if !safeBundlePath(name) || name == "profile.json" || len(data) > 10<<20 {
			return fmt.Errorf("invalid or oversized bundle file %q", name)
		}
		total += tarEntrySize(len(data))
		names = append(names, name)
	}
	if total > maxProfileBundle {
		return errors.New("profile bundle exceeds size limit")
	}
	sort.Strings(names)
	out := tar.NewWriter(w)
	put := func(name string, data []byte) error {
		header := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(data)), ModTime: profileBundleEpoch, Format: tar.FormatUSTAR}
		if err := out.WriteHeader(header); err != nil {
			return fmt.Errorf("bundle file %q: %w", name, err)
		}
		_, err := out.Write(data)
		return err
	}
	if err := put("profile.json", bundle.Profile); err != nil {
		return err
	}
	for _, name := range names {
		if err := put(name, bundle.Files[name]); err != nil {
			return err
		}
	}
	return out.Close()
}

// tarEntrySize is the USTAR size of one regular file entry: header plus padded data.
func tarEntrySize(n int) int64 {
	return 512 + (int64(n)+511)/512*512
}
