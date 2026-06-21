// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import "sort"

// Capability strings declared via require (RFC 5228 §2.10.5 and the
// extension RFCs).
const (
	capFileInto     = "fileinto"
	capCopy         = "copy"
	capImap4Flags   = "imap4flags"
	capEnvelope     = "envelope"
	capBody         = "body"
	capMailbox      = "mailbox"
	capIhave        = "ihave"
	capSpamTest     = "spamtest"
	capSpamTestPlus = "spamtestplus"
	capVirusTest    = "virustest"
	capEnvironment  = "environment"
	capDuplicate    = "duplicate"
	capDate         = "date"
	capIndex        = "index"
	capVacation     = "vacation"
	capEnotify      = "enotify"
	capEditHeader   = "editheader"
	capVariables    = "variables"
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
			if v.Create {
				addCap(set, capMailbox)
			}
		case *Redirect:
			if v.Copy {
				addCap(set, capCopy)
			}
		case *SetFlag:
			addCap(set, capImap4Flags)
			if v.Variable != "" {
				addCap(set, capVariables)
			}
		case *AddFlag:
			addCap(set, capImap4Flags)
			if v.Variable != "" {
				addCap(set, capVariables)
			}
		case *RemoveFlag:
			addCap(set, capImap4Flags)
			if v.Variable != "" {
				addCap(set, capVariables)
			}
		case *Set:
			addCap(set, capVariables)
		case *Error:
			addCap(set, capIhave)
		case *Vacation:
			addCap(set, capVacation)
		case *Notify:
			addCap(set, capEnotify)
		case *AddHeader:
			addCap(set, capEditHeader)
		case *DeleteHeader:
			// deleteheader's own :index/:last are part of editheader (RFC 5293),
			// not the index extension, so no "index" capability here.
			addCap(set, capEditHeader, comparatorCap(v.Comparator), matchCap(v.MatchType))
		case *If:
			collectTestCaps(v.Test, set)
			collectCommandCaps(v.Then, set)
			for _, b := range v.Elsif {
				collectTestCaps(b.Test, set)
				collectCommandCaps(b.Then, set)
			}
			collectCommandCaps(v.Else, set)
		case *RawCommand:
			// The carrier command contributes nothing itself (its own
			// capability rides on an explicit Require), but a modelled test
			// or block it carries still does.
			if v.Test != nil {
				collectTestCaps(v.Test, set)
			}
			collectCommandCaps(v.Block, set)
		}
		// *Keep, *Discard, *Stop contribute nothing.
	}
}

func collectTestCaps(t Test, set map[string]struct{}) {
	switch v := t.(type) {
	case *HeaderTest:
		addCap(set, comparatorCap(v.Comparator), matchCap(v.MatchType), indexCap(v.Index))
	case *AddressTest:
		addCap(set, comparatorCap(v.Comparator), matchCap(v.MatchType), addressPartCap(v.AddressPart), indexCap(v.Index))
	case *EnvelopeTest:
		addCap(set, capEnvelope, comparatorCap(v.Comparator), matchCap(v.MatchType), addressPartCap(v.AddressPart))
	case *BodyTest:
		addCap(set, capBody, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *DateTest:
		addCap(set, capDate, comparatorCap(v.Comparator), matchCap(v.MatchType), indexCap(v.Index))
	case *CurrentDateTest:
		addCap(set, capDate, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *StringTest:
		addCap(set, capVariables, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *HasFlagTest:
		addCap(set, capImap4Flags, comparatorCap(v.Comparator), matchCap(v.MatchType))
		if len(v.Variables) > 0 {
			addCap(set, capVariables)
		}
	case *ValidNotifyMethodTest:
		addCap(set, capEnotify)
	case *NotifyMethodCapabilityTest:
		addCap(set, capEnotify, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *MailboxExistsTest:
		addCap(set, capMailbox)
	case *SpamTest:
		// spamtestplus is a superset of spamtest; :percent needs it alone.
		if v.Percent {
			addCap(set, capSpamTestPlus)
		} else {
			addCap(set, capSpamTest)
		}
		addCap(set, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *VirusTest:
		addCap(set, capVirusTest, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *EnvironmentTest:
		addCap(set, capEnvironment, comparatorCap(v.Comparator), matchCap(v.MatchType))
	case *DuplicateTest:
		addCap(set, capDuplicate)
	case *IHaveTest:
		addCap(set, capIhave)
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

// matchCap returns the capability a match-type requires, or "" for the
// built-in ones: :count/:value need "relational" (RFC 5231) and :regex
// needs "regex" (draft-murchison-sieve-regex).
func matchCap(m MatchType) string {
	switch m {
	case MatchCount, MatchValue:
		return "relational"
	case MatchRegex:
		return "regex"
	default:
		return ""
	}
}

// indexCap returns "index" when a :index N argument is set (RFC 5260
// index), or "" otherwise.
func indexCap(index int) string {
	if index > 0 {
		return capIndex
	}
	return ""
}

// addressPartCap returns the capability an address-part requires, or "" for
// the built-in ones: :user/:detail need "subaddress" (RFC 5233).
func addressPartCap(a AddressPart) string {
	switch a {
	case AddressUser, AddressDetail:
		return "subaddress"
	default:
		return ""
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
