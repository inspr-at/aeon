// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIdenticalSyntheticPDFsMatch(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		if _, statErr := os.Stat("/opt/homebrew/bin/pdftoppm"); statErr != nil {
			t.Skip("pdftoppm unavailable")
		}
	}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "same.pdf")
	if err := os.WriteFile(pdf, rectanglePDF(0, 0, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	code, err := Run(pdf, pdf, Options{OutDir: out, DPI: 150, Tolerance: 16, Threshold: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	html, err := os.ReadFile(filepath.Join(out, "summary.html"))
	if err != nil || len(html) == 0 {
		t.Fatalf("summary missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "page-01-diff.png")); err != nil {
		t.Fatal(err)
	}
}

func TestShiftedRectangleExceedsThreshold(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		if _, statErr := os.Stat("/opt/homebrew/bin/pdftoppm"); statErr != nil {
			t.Skip("pdftoppm unavailable")
		}
	}
	dir := t.TempDir()
	left := filepath.Join(dir, "left.pdf")
	right := filepath.Join(dir, "right.pdf")
	if err := os.WriteFile(left, rectanglePDF(0, 0, 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, rectanglePDF(80, 0, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	code, err := Run(left, right, Options{OutDir: out, DPI: 150, Tolerance: 16, Threshold: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Fatal("expected nonzero exit above threshold")
	}
}

func rectanglePDF(x, y, black int) []byte {
	// A4 media box with one filled rectangle. black=1 paints black, else red.
	color := "1 0 0"
	if black == 1 {
		color = "0 0 0"
	}
	stream := "q " + color + " rg " + itoa(72+x) + " " + itoa(700+y) + " 120 36 re f Q"
	return []byte("%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595.28 841.89]/Contents 4 0 R>>endobj\n" +
		"4 0 obj<</Length " + itoa(len(stream)) + ">>stream\n" + stream + "\nendstream\nendobj\n" +
		"trailer<</Size 5/Root 1 0 R>>\n%%EOF\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
