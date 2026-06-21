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
	// Path locates the offending construct within the script, e.g.
	// "commands[0].then[1]" or "commands[2].test.anyof[0]". It is empty
	// for script-level findings (such as a missing require).
	Path string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

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

	sc := &capScan{derived: map[string]struct{}{}, carriers: map[string]struct{}{}}
	d.walk(s.Commands, 0, "commands", sc)

	// require-coverage (§3.2): every derived capability must be declared.
	// This also enforces comparator legality — a non-built-in comparator
	// derives a "comparator-<name>" capability that must be required.
	for _, capName := range sortedKeys(sc.derived) {
		if _, ok := declared[capName]; !ok {
			d.Errors = append(d.Errors, Diagnostic{
				Message: fmt.Sprintf("extension %q is used but not declared with require (RFC 5228 §3.2)", capName),
			})
		}
	}

	// Warn about unmodelled constructs preserved by the carriers.
	for _, name := range sortedKeys(sc.carriers) {
		d.Warnings = append(d.Warnings, Diagnostic{
			Message: fmt.Sprintf("%q is not a modelled command or test; it round-trips verbatim but its requirements were not validated", name),
		})
	}

	// Warn about declared-but-unused capabilities, unless a carrier is
	// present (an unmodelled construct may legitimately use them).
	if len(sc.carriers) == 0 {
		for _, capName := range sortedKeys(declared) {
			if _, ok := sc.derived[capName]; !ok {
				d.Warnings = append(d.Warnings, Diagnostic{
					Message: fmt.Sprintf("capability %q is required but never used", capName),
				})
			}
		}
	}

	return d
}

// capScan accumulates the capabilities derived from modelled constructs
// and the names of carrier (unmodelled) constructs during a walk.
type capScan struct {
	derived  map[string]struct{}
	carriers map[string]struct{}
}

func (d *Diagnostics) addError(path, msg string) {
	d.Errors = append(d.Errors, Diagnostic{Message: msg, Path: path})
}

func (d *Diagnostics) addWarning(path, msg string) {
	d.Warnings = append(d.Warnings, Diagnostic{Message: msg, Path: path})
}

// walk collects the capabilities derived from modelled commands/tests and
// the names of carrier nodes, flags any require nested below the top level
// (depth > 0) as a fatal error (§3.2), and emits warnings for degenerate
// shapes the type system permits but the spec does not really allow.
func (d *Diagnostics) walk(cmds []Command, depth int, path string, sc *capScan) {
	for i, c := range cmds {
		cp := fmt.Sprintf("%s[%d]", path, i)
		switch v := c.(type) {
		case *Require:
			if depth > 0 {
				d.addError(cp, fmt.Sprintf("require %s must appear at the top level of the script, not inside a block (RFC 5228 §3.2)", formatCaps(v.Capabilities)))
			}
		case *FileInto:
			sc.derived[capFileInto] = struct{}{}
			if v.Copy {
				sc.derived[capCopy] = struct{}{}
			}
			if v.Mailbox == "" {
				d.addWarning(cp, "fileinto has an empty mailbox")
			}
		case *Redirect:
			if v.Copy {
				sc.derived[capCopy] = struct{}{}
			}
			if v.Address == "" {
				d.addWarning(cp, "redirect has an empty address")
			}
		case *SetFlag:
			d.flagCommand(cp, "setflag", v.Flags, sc)
		case *AddFlag:
			d.flagCommand(cp, "addflag", v.Flags, sc)
		case *RemoveFlag:
			d.flagCommand(cp, "removeflag", v.Flags, sc)
		case *If:
			d.walkTest(v.Test, cp+".test", sc)
			d.walk(v.Then, depth+1, cp+".then", sc)
			for j, b := range v.Elsif {
				ep := fmt.Sprintf("%s.elsif[%d]", cp, j)
				d.walkTest(b.Test, ep+".test", sc)
				d.walk(b.Then, depth+1, ep, sc)
			}
			d.walk(v.Else, depth+1, cp+".else", sc)
		case *RawCommand:
			sc.carriers[v.Name] = struct{}{}
			d.walk(v.Block, depth+1, cp, sc)
		}
	}
}

func (d *Diagnostics) flagCommand(path, name string, flags []string, sc *capScan) {
	sc.derived[capImap4Flags] = struct{}{}
	if len(flags) == 0 {
		d.addWarning(path, name+" has an empty flag list")
	}
}

func (d *Diagnostics) walkTest(t Test, path string, sc *capScan) {
	switch v := t.(type) {
	case *HeaderTest:
		d.derive(sc, comparatorCap(v.Comparator))
		d.checkComparison(path, v.Headers, v.Keys, v.MatchType)
	case *AddressTest:
		d.derive(sc, comparatorCap(v.Comparator))
		d.checkComparison(path, v.Headers, v.Keys, v.MatchType)
	case *EnvelopeTest:
		sc.derived[capEnvelope] = struct{}{}
		d.derive(sc, comparatorCap(v.Comparator))
		d.checkComparison(path, v.Parts, v.Keys, v.MatchType)
	case *BodyTest:
		sc.derived[capBody] = struct{}{}
		d.derive(sc, comparatorCap(v.Comparator))
		if len(v.Keys) == 0 {
			d.addWarning(path, "body test has an empty key list")
		}
		d.checkMatches(path, v.MatchType, v.Keys)
	case *ExistsTest:
		if len(v.Headers) == 0 {
			d.addWarning(path, "exists test has an empty header list")
		}
	case *AllOf:
		if len(v.Tests) == 0 {
			d.addWarning(path, "allof has no tests (always true)")
		}
		for k, sub := range v.Tests {
			d.walkTest(sub, fmt.Sprintf("%s.allof[%d]", path, k), sc)
		}
	case *AnyOf:
		if len(v.Tests) == 0 {
			d.addWarning(path, "anyof has no tests (always false)")
		}
		for k, sub := range v.Tests {
			d.walkTest(sub, fmt.Sprintf("%s.anyof[%d]", path, k), sc)
		}
	case *Not:
		d.walkTest(v.Test, path+".not", sc)
	case *RawTest:
		sc.carriers[v.Name] = struct{}{}
	}
}

func (d *Diagnostics) derive(sc *capScan, capName string) {
	if capName != "" {
		sc.derived[capName] = struct{}{}
	}
}

// checkComparison warns about empty header/key lists and dangling
// :matches escapes for the header/address/envelope-shaped tests.
func (d *Diagnostics) checkComparison(path string, names, keys []string, mt MatchType) {
	if len(names) == 0 {
		d.addWarning(path, "test has an empty header list")
	}
	if len(keys) == 0 {
		d.addWarning(path, "test has an empty key list")
	}
	d.checkMatches(path, mt, keys)
}

// checkMatches warns about :matches keys that end with a dangling
// backslash escape, which has no defined meaning (RFC 5228 §2.7.1).
func (d *Diagnostics) checkMatches(path string, mt MatchType, keys []string) {
	if mt != MatchMatches {
		return
	}
	for _, k := range keys {
		if hasDanglingEscape(k) {
			d.addWarning(path, fmt.Sprintf("the :matches pattern %q ends with a dangling backslash escape", k))
		}
	}
}

func hasDanglingEscape(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
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
