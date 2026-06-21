// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"fmt"
	"sort"
	"strings"
)

// Diagnostic is a single validation finding with human-readable text.
type Diagnostic struct {
	// Message is a human-readable description of the finding. ManageSieve
	// servers (RFC 5804) carry this text verbatim to the user.
	Message string
}

func (d Diagnostic) String() string { return d.Message }

// Diagnostics is the result of [Script.Check]: structural findings split
// into fatal Errors and non-fatal Warnings. The split is deliberate so a
// consumer such as a ManageSieve server backend (RFC 5804 CHECKSCRIPT,
// PUTSCRIPT) can map Errors to a NO response and Warnings to
// OK (WARNINGS "..."), without re-deriving the distinction.
type Diagnostics struct {
	Errors   []Diagnostic
	Warnings []Diagnostic
}

// HasErrors reports whether any fatal finding was recorded.
func (d Diagnostics) HasErrors() bool { return len(d.Errors) > 0 }

// ValidationError is returned by [Script.Validate] when a script has one
// or more fatal validation errors. It carries the full [Diagnostics] so
// callers can still reach the non-fatal warnings.
type ValidationError struct {
	Diagnostics Diagnostics
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Diagnostics.Errors))
	for i, d := range e.Diagnostics.Errors {
		msgs[i] = d.Message
	}
	return "sieve: " + strings.Join(msgs, "; ")
}

// Validate reports the script's fatal validation errors as a single
// *ValidationError, or nil if the script is structurally valid. It is a
// convenience wrapper over [Script.Check] for callers that only need a
// pass/fail answer; use Check to also reach the non-fatal warnings.
func (s *Script) Validate() error {
	d := s.Check()
	if !d.HasErrors() {
		return nil
	}
	return &ValidationError{Diagnostics: d}
}

// Check runs structural validation and returns all findings. It enforces
// the require-related MUSTs of RFC 5228 §3.2 as fatal errors:
//
//   - an extension used by a modelled command or test must be declared
//     with require;
//   - require commands must appear before any other command, at the top
//     level of the script.
//
// Findings that are not protocol violations — an unmodelled command or
// test preserved verbatim, or a declared-but-unused capability — are
// reported as non-fatal warnings. Check never inspects message content
// or mailbox existence; it is a wire/structure check only.
func (s *Script) Check() Diagnostics {
	var d Diagnostics

	// require-placement (§3.2): at the top level, require must precede any
	// other command.
	declared := map[string]struct{}{}
	seenOther := false
	for _, c := range s.Commands {
		r, ok := c.(*Require)
		if !ok {
			seenOther = true
			continue
		}
		if seenOther {
			d.Errors = append(d.Errors, Diagnostic{
				Message: fmt.Sprintf("require %s must appear before any other command (RFC 5228 §3.2)", formatCaps(r.Capabilities)),
			})
		}
		for _, cap := range r.Capabilities {
			declared[cap] = struct{}{}
		}
	}

	derived := map[string]struct{}{}
	carriers := map[string]struct{}{}
	d.walk(s.Commands, 0, derived, carriers)

	// require-coverage (§3.2): every derived capability must be declared.
	for _, capName := range sortedKeys(derived) {
		if _, ok := declared[capName]; !ok {
			d.Errors = append(d.Errors, Diagnostic{
				Message: fmt.Sprintf("extension %q is used but not declared with require (RFC 5228 §3.2)", capName),
			})
		}
	}

	// Warn about unmodelled constructs preserved by the carriers.
	for _, name := range sortedKeys(carriers) {
		d.Warnings = append(d.Warnings, Diagnostic{
			Message: fmt.Sprintf("%q is not a modelled command or test; it round-trips verbatim but its requirements were not validated", name),
		})
	}

	// Warn about declared-but-unused capabilities, unless a carrier is
	// present (an unmodelled construct may legitimately use them).
	if len(carriers) == 0 {
		for _, capName := range sortedKeys(declared) {
			if _, ok := derived[capName]; !ok {
				d.Warnings = append(d.Warnings, Diagnostic{
					Message: fmt.Sprintf("capability %q is required but never used", capName),
				})
			}
		}
	}

	return d
}

// walk collects the capabilities derived from modelled commands/tests and
// the names of carrier nodes, and flags any require nested below the top
// level (depth > 0) as a fatal error (§3.2).
func (d *Diagnostics) walk(cmds []Command, depth int, derived, carriers map[string]struct{}) {
	for _, c := range cmds {
		switch v := c.(type) {
		case *Require:
			if depth > 0 {
				d.Errors = append(d.Errors, Diagnostic{
					Message: fmt.Sprintf("require %s must appear at the top level of the script, not inside a block (RFC 5228 §3.2)", formatCaps(v.Capabilities)),
				})
			}
		case *FileInto:
			derived[capFileInto] = struct{}{}
			if v.Copy {
				derived[capCopy] = struct{}{}
			}
		case *Redirect:
			if v.Copy {
				derived[capCopy] = struct{}{}
			}
		case *SetFlag, *AddFlag, *RemoveFlag:
			derived[capImap4Flags] = struct{}{}
		case *If:
			d.walkTest(v.Test, derived, carriers)
			d.walk(v.Then, depth+1, derived, carriers)
			for _, b := range v.Elsif {
				d.walkTest(b.Test, derived, carriers)
				d.walk(b.Then, depth+1, derived, carriers)
			}
			d.walk(v.Else, depth+1, derived, carriers)
		case *RawCommand:
			carriers[v.Name] = struct{}{}
			d.walk(v.Block, depth+1, derived, carriers)
		}
	}
}

func (d *Diagnostics) walkTest(t Test, derived, carriers map[string]struct{}) {
	switch v := t.(type) {
	case *EnvelopeTest:
		derived[capEnvelope] = struct{}{}
	case *BodyTest:
		derived[capBody] = struct{}{}
	case *AllOf:
		for _, sub := range v.Tests {
			d.walkTest(sub, derived, carriers)
		}
	case *AnyOf:
		for _, sub := range v.Tests {
			d.walkTest(sub, derived, carriers)
		}
	case *Not:
		d.walkTest(v.Test, derived, carriers)
	case *RawTest:
		carriers[v.Name] = struct{}{}
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func formatCaps(caps []string) string {
	quoted := make([]string, len(caps))
	for i, c := range caps {
		quoted[i] = fmt.Sprintf("%q", c)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
