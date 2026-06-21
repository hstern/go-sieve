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
