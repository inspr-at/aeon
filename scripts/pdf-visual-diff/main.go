// SPDX-License-Identifier: AGPL-3.0-only

// Command pdf-visual-diff rasterises two PDFs and reports per-page visual
// difference. Rasterisation shells out to poppler's pdftoppm so this tool
// adds no Go module dependency.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	left := flag.String("a", "", "left PDF")
	right := flag.String("b", "", "right PDF")
	out := flag.String("out", "", "directory for diff PNGs and summary.html")
	dpi := flag.Int("dpi", 150, "raster DPI")
	tolerance := flag.Int("tolerance", 16, "per-channel anti-alias tolerance, 0-255")
	threshold := flag.Float64("threshold", 0.5, "nonzero exit when any page mismatch percent exceeds this")
	flag.Parse()
	if *left == "" || *right == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: pdf-visual-diff -a left.pdf -b right.pdf -out dir [-dpi 150] [-tolerance 16] [-threshold 0.5]")
		os.Exit(1)
	}
	code, err := Run(*left, *right, Options{
		OutDir:    *out,
		DPI:       *dpi,
		Tolerance: *tolerance,
		Threshold: *threshold,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
