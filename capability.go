// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import "sort"

// Capability strings declared via require (RFC 5228 §2.10.5 and the
// extension RFCs).
const (
	capFileInto   = "fileinto"
	capCopy       = "copy"
	capImap4Flags = "imap4flags"
	capEnvelope   = "envelope"
	capBody       = "body"
)

// Capabilities returns the sorted, de-duplicated set of extension
// capabilities the script requires: those derived from the commands and
// tests it uses, unioned with any capabilities named by explicit Require
// commands (which are preserved even when unused). This is the single
// source of truth that both the encoder's auto-require and Validate read.
func (s *Script) Capabilities() []string {
	set := map[string]struct{}{}
	collectCommandCaps(s.Commands, set)
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func addCap(set map[string]struct{}, caps ...string) {
	for _, c := range caps {
		if c != "" {
			set[c] = struct{}{}
		}
	}
}

func collectCommandCaps(cmds []Command, set map[string]struct{}) {
	for _, c := range cmds {
		switch v := c.(type) {
		case *Require:
			addCap(set, v.Capabilities...)
		case *FileInto:
			addCap(set, capFileInto)
			if v.Copy {
				addCap(set, capCopy)
			}
		case *Redirect:
			if v.Copy {
				addCap(set, capCopy)
			}
		case *SetFlag, *AddFlag, *RemoveFlag:
			addCap(set, capImap4Flags)
		case *If:
			collectTestCaps(v.Test, set)
			collectCommandCaps(v.Then, set)
			for _, b := range v.Elsif {
				collectTestCaps(b.Test, set)
				collectCommandCaps(b.Then, set)
			}
			collectCommandCaps(v.Else, set)
		}
		// *Keep, *Discard, *Stop, *RawCommand contribute nothing
		// automatically; a RawCommand's capability is carried by an
		// explicit Require.
	}
}

func collectTestCaps(t Test, set map[string]struct{}) {
	switch v := t.(type) {
	case *HeaderTest:
		addCap(set, comparatorCap(v.Comparator))
	case *AddressTest:
		addCap(set, comparatorCap(v.Comparator))
	case *EnvelopeTest:
		addCap(set, capEnvelope, comparatorCap(v.Comparator))
	case *BodyTest:
		addCap(set, capBody, comparatorCap(v.Comparator))
	case *AllOf:
		for _, sub := range v.Tests {
			collectTestCaps(sub, set)
		}
	case *AnyOf:
		for _, sub := range v.Tests {
			collectTestCaps(sub, set)
		}
	case *Not:
		collectTestCaps(v.Test, set)
	}
}

// comparatorCap returns the capability a comparator requires, or "" for
// the two built-in comparators that need no require (RFC 5228 §2.7.3:
// i;ascii-casemap and i;octet are mandatory-to-implement). Any other
// comparator must be declared with require "comparator-<name>"
// (RFC 5228 §2.7.3, RFC 4790 §3.1).
func comparatorCap(comp string) string {
	switch comp {
	case "", defaultComparator, "i;octet":
		return ""
	default:
		return "comparator-" + comp
	}
}
