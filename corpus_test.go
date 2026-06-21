// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCorpus checks every fixture under testdata/corpus: it must parse,
// validate without fatal errors, and be a canonical fixed point
// (Encode(Parse(f)) == f). The fixtures are transcribed from RFC 5228
// (and the RFC 5232/3894/5173 extension examples) plus realistic
// Dovecot Pigeonhole-style scripts.
func TestCorpus(t *testing.T) {
	files, err := filepath.Glob("testdata/corpus/*.sieve")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus fixtures found")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			s, err := Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if err := s.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
			out, err := s.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(out) != string(src) {
				t.Errorf("fixture is not canonical (round trip mismatch)\n got: %q\nwant: %q", out, src)
			}
		})
	}
}
