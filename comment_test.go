// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"strings"
	"testing"
)

func TestKeepCommentsRoundTrip(t *testing.T) {
	tests := []string{
		// No capabilities: comments interleave freely.
		"# top\nkeep;\n/* mid */\ndiscard;\n",
		// Comment after the auto-required line round-trips in place.
		"require \"fileinto\";\n# file it\nfileinto \"Spam\";\n",
		// Comment inside a block, indented with the block.
		"if true {\n\t# inner\n\tkeep;\n}\n",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			s, err := Parse([]byte(src), KeepComments())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := s.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(out) != src {
				t.Errorf("comment round trip\n got: %q\nwant: %q", out, src)
			}
		})
	}
}

func TestDefaultParseStripsComments(t *testing.T) {
	src := "# top\nkeep;\n/* mid */\ndiscard;\n"
	s, err := Parse([]byte(src)) // no KeepComments
	if err != nil {
		t.Fatal(err)
	}
	out, _ := s.Encode()
	if want := "keep;\ndiscard;\n"; string(out) != want {
		t.Errorf("default parse should strip comments\n got: %q\nwant: %q", out, want)
	}
	// No Comment nodes in the AST.
	for _, c := range s.Commands {
		if _, ok := c.(*Comment); ok {
			t.Errorf("default parse leaked a *Comment node")
		}
	}
}

func TestKeepCommentsRetainsInlineComments(t *testing.T) {
	// A comment inside an expression is retained (content not lost), though
	// repositioned to the nearest command boundary on its own line.
	src := `if header /* inline */ :contains "subject" "hi" { keep; }`
	s, err := Parse([]byte(src), KeepComments())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, _ := s.Encode()
	if !strings.Contains(string(out), "/* inline */") {
		t.Errorf("inline comment content was dropped\n got: %q", out)
	}
	// Without KeepComments it is dropped entirely.
	s2, _ := Parse([]byte(src))
	out2, _ := s2.Encode()
	if strings.Contains(string(out2), "inline") {
		t.Errorf("default parse should drop the comment\n got: %q", out2)
	}
}

func TestKeepCommentsLeadingBeforeRequire(t *testing.T) {
	// A comment at the very top stays above the auto-derived require.
	src := "# header comment\nrequire \"fileinto\";\nfileinto \"Spam\";\n"
	s, err := Parse([]byte(src), KeepComments())
	if err != nil {
		t.Fatal(err)
	}
	out, _ := s.Encode()
	if string(out) != src {
		t.Errorf("leading comment should precede require\n got: %q\nwant: %q", out, src)
	}
}

func TestKeepCommentsValidates(t *testing.T) {
	// Comment nodes are inert for validation.
	s, err := Parse([]byte("# c\nkeep;\n"), KeepComments())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if d := s.Check(); len(d.Warnings) != 0 || d.HasErrors() {
		t.Errorf("comments should produce no diagnostics: %+v", d)
	}
}
