// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import "testing"

// FuzzParse asserts two invariants over arbitrary input:
//
//  1. Parse never panics.
//  2. When Parse succeeds, the encoded output is a fixed point: parsing
//     and re-encoding it yields byte-identical text. This is the
//     round-trip guarantee, checked against adversarial input.
func FuzzParse(f *testing.F) {
	for _, src := range canonicalScripts {
		f.Add([]byte(src))
	}
	f.Add([]byte("if true { keep; }"))
	f.Add([]byte("text:\n.\n"))
	f.Add([]byte(`require ["a", "b", "c"];`))

	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := Parse(data)
		if err != nil {
			return // syntax errors are an expected outcome, not a failure
		}
		out, err := s.Encode()
		if err != nil {
			t.Fatalf("Encode failed after a successful Parse: %v", err)
		}
		s2, err := Parse(out)
		if err != nil {
			t.Fatalf("re-Parse of canonical output failed: %v\noutput: %q", err, out)
		}
		out2, err := s2.Encode()
		if err != nil {
			t.Fatalf("re-Encode failed: %v", err)
		}
		if string(out) != string(out2) {
			t.Fatalf("encoder output is not a fixed point:\n first: %q\nsecond: %q", out, out2)
		}
	})
}
