// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"errors"
	"testing"
)

func TestValidateValid(t *testing.T) {
	// An encoded script is always valid: auto-require covers everything.
	s := &Script{Commands: []Command{
		&Require{Capabilities: []string{"fileinto"}},
		&FileInto{Mailbox: "Junk"},
	}}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	d := s.Check()
	if d.HasErrors() {
		t.Errorf("unexpected errors: %v", d.Errors)
	}
	if len(d.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", d.Warnings)
	}
}

func TestValidateMissingRequire(t *testing.T) {
	s := &Script{Commands: []Command{
		&FileInto{Mailbox: "Junk"}, // uses fileinto, no require
	}}
	err := s.Validate()
	if err == nil {
		t.Fatal("Validate: want error for missing require")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T, want *ValidationError", err)
	}
	if !ve.Diagnostics.HasErrors() {
		t.Fatal("ValidationError carries no diagnostics")
	}
}

func TestValidateRequireOrdering(t *testing.T) {
	// require after a non-require command is a §3.2 violation.
	s := &Script{Commands: []Command{
		&Keep{},
		&Require{Capabilities: []string{"fileinto"}},
		&FileInto{Mailbox: "X"},
	}}
	d := s.Check()
	if !d.HasErrors() {
		t.Fatal("want a placement error for require after keep")
	}
}

func TestValidateNestedRequire(t *testing.T) {
	s := &Script{Commands: []Command{
		&If{
			Test: &True{},
			Then: []Command{&Require{Capabilities: []string{"fileinto"}}},
		},
	}}
	d := s.Check()
	if !d.HasErrors() {
		t.Fatal("want an error for require nested in a block")
	}
}

func TestCheckWarnsOnCarrier(t *testing.T) {
	s, err := Parse([]byte("require \"vacation\";\nvacation \"away\";\n"))
	if err != nil {
		t.Fatal(err)
	}
	d := s.Check()
	if d.HasErrors() {
		t.Errorf("unmodelled vacation should not be a fatal error: %v", d.Errors)
	}
	if len(d.Warnings) == 0 {
		t.Error("want a warning that vacation is unmodelled")
	}
}

func TestCheckWarnsOnUnusedCapability(t *testing.T) {
	s := &Script{Commands: []Command{
		&Require{Capabilities: []string{"imap4flags"}},
		&Keep{}, // imap4flags declared but unused
	}}
	d := s.Check()
	if d.HasErrors() {
		t.Errorf("unused capability should be a warning, not an error: %v", d.Errors)
	}
	if len(d.Warnings) != 1 {
		t.Fatalf("want 1 unused-capability warning, got %d: %v", len(d.Warnings), d.Warnings)
	}
}

func TestValidateComparatorLegality(t *testing.T) {
	// A non-built-in comparator requires comparator-<name>.
	used := &HeaderTest{Comparator: "i;ascii-numeric", Headers: []string{"x-priority"}, Keys: []string{"1"}}
	missing := &Script{Commands: []Command{&If{Test: used, Then: []Command{&Keep{}}}}}
	if err := missing.Validate(); err == nil {
		t.Fatal("want error: comparator-i;ascii-numeric not required")
	}

	// Declaring it makes the script valid.
	ok := &Script{Commands: []Command{
		&Require{Capabilities: []string{"comparator-i;ascii-numeric"}},
		&If{Test: used, Then: []Command{&Keep{}}},
	}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}

	// The built-in comparators need no capability.
	for _, comp := range []string{"", "i;ascii-casemap", "i;octet"} {
		s := &Script{Commands: []Command{&If{
			Test: &HeaderTest{Comparator: comp, Headers: []string{"subject"}, Keys: []string{"x"}},
			Then: []Command{&Keep{}},
		}}}
		if err := s.Validate(); err != nil {
			t.Errorf("built-in comparator %q should validate: %v", comp, err)
		}
	}
}

func TestEncoderDerivesComparatorRequire(t *testing.T) {
	s := &Script{Commands: []Command{&If{
		Test: &HeaderTest{Comparator: "i;ascii-numeric", Headers: []string{"x"}, Keys: []string{"1"}},
		Then: []Command{&Keep{}},
	}}}
	out, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	const want = "require \"comparator-i;ascii-numeric\";\n"
	if got := string(out); len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("auto-require missing comparator capability\n got: %q", got)
	}
}

func TestCheckWarnsOnDanglingMatchEscape(t *testing.T) {
	s := &Script{Commands: []Command{&If{
		Test: &HeaderTest{MatchType: MatchMatches, Headers: []string{"subject"}, Keys: []string{`foo\`}},
		Then: []Command{&Keep{}},
	}}}
	d := s.Check()
	if d.HasErrors() {
		t.Errorf("dangling escape should be a warning, not an error: %v", d.Errors)
	}
	found := false
	for _, w := range d.Warnings {
		if w.Path != "" {
			found = true
		}
	}
	if !found || len(d.Warnings) == 0 {
		t.Errorf("want a located :matches warning, got %v", d.Warnings)
	}
}

func TestCheckWarnsOnDegenerateShapes(t *testing.T) {
	s := &Script{Commands: []Command{
		&Require{Capabilities: []string{"fileinto"}}, // cover fileinto so only structural warnings remain
		&If{Test: &ExistsTest{}, Then: []Command{&FileInto{Mailbox: ""}}},
		&If{Test: &AllOf{}, Then: []Command{&Keep{}}},
	}}
	d := s.Check()
	if d.HasErrors() {
		t.Errorf("degenerate shapes should warn, not error: %v", d.Errors)
	}
	// empty exists header list, empty fileinto mailbox, empty allof => >= 3 warnings.
	if len(d.Warnings) < 3 {
		t.Errorf("want >=3 structural warnings, got %d: %v", len(d.Warnings), d.Warnings)
	}
	for _, w := range d.Warnings {
		if w.Path == "" {
			t.Errorf("structural warning lacks a Path: %q", w.Message)
		}
	}
}

func TestValidateExtensionMatchArgs(t *testing.T) {
	// Each new match-arg construct requires its capability.
	cases := []struct {
		name string
		test Test
		cap  string
	}{
		{"relational :count", &HeaderTest{MatchType: MatchCount, Relational: "ge", Headers: []string{"x"}, Keys: []string{"1"}}, "relational"},
		{"regex :regex", &HeaderTest{MatchType: MatchRegex, Headers: []string{"x"}, Keys: []string{".*"}}, "regex"},
		{"subaddress :user", &AddressTest{AddressPart: AddressUser, Headers: []string{"to"}, Keys: []string{"x"}}, "subaddress"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing := &Script{Commands: []Command{&If{Test: tc.test, Then: []Command{&Keep{}}}}}
			if err := missing.Validate(); err == nil {
				t.Errorf("want error for missing require %q", tc.cap)
			}
			ok := &Script{Commands: []Command{
				&Require{Capabilities: []string{tc.cap}},
				&If{Test: tc.test, Then: []Command{&Keep{}}},
			}}
			if err := ok.Validate(); err != nil {
				t.Errorf("Validate with %q declared: %v", tc.cap, err)
			}
		})
	}
}

func TestEncodedScriptsAlwaysValidate(t *testing.T) {
	// Every canonical script in the round-trip corpus must parse and
	// validate cleanly (auto-require guarantees coverage).
	for _, src := range canonicalScripts {
		s, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%q): %v", src, err)
		}
	}
}
