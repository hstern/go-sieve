// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import "testing"

// canonicalScripts are byte-exact canonical outputs of the encoder. Each
// must satisfy Encode(Parse(s)) == s.
var canonicalScripts = []string{
	"keep;\n",
	"stop;\n",
	"discard;\n",
	"require \"fileinto\";\nfileinto \"Junk\";\n",
	"require [\"copy\", \"fileinto\"];\nfileinto :copy \"Archive\";\n",
	"require \"imap4flags\";\nsetflag \"\\\\Seen\";\n",
	"require \"imap4flags\";\naddflag [\"\\\\Seen\", \"\\\\Flagged\"];\n",
	"redirect \"a@b.example\";\n",
	"require \"copy\";\nredirect :copy \"a@b.example\";\n",
	"require \"fileinto\";\nif header :contains \"subject\" \"[SPAM]\" {\n\tfileinto \"Junk\";\n\tstop;\n}\n",
	"if size :over 1048576 {\n\tdiscard;\n}\n",
	"if exists [\"to\", \"cc\"] {\n\tkeep;\n}\n",
	"if address :domain \"from\" \"example.com\" {\n\tkeep;\n}\n",
	"require \"envelope\";\nif envelope :localpart \"from\" \"postmaster\" {\n\tdiscard;\n}\n",
	"require \"body\";\nif body :raw :contains \"viagra\" {\n\tdiscard;\n}\n",
	"require \"body\";\nif body :content [\"text/plain\", \"text/html\"] :contains \"x\" {\n\tkeep;\n}\n",
	"if anyof (address :localpart \"from\" \"root\", not true) {\n\tdiscard;\n}\n",
	"if allof (header :contains \"subject\" \"hi\", header \"from\" \"a@b\") {\n\tkeep;\n}\n",
	"if header :comparator \"i;octet\" \"x-flag\" \"YES\" {\n\tkeep;\n}\n",
	"require \"fileinto\";\nif header :contains \"subject\" \"a\" {\n\tfileinto \"A\";\n} elsif header :contains \"subject\" \"b\" {\n\tfileinto \"B\";\n} else {\n\tkeep;\n}\n",
	// Unmodelled command preserved as a carrier (reject is not modelled).
	"require \"reject\";\nreject \"not accepted\";\n",
	// Unmodelled control command taking a test argument before its block.
	"mycontrol true {\n\tkeep;\n}\n",
	// mailbox (RFC 5490): :create and mailboxexists.
	"require [\"fileinto\", \"mailbox\"];\nfileinto :create \"Archive\";\n",
	"require [\"fileinto\", \"mailbox\"];\nif mailboxexists \"Archive\" {\n\tfileinto \"Archive\";\n}\n",
	// spamtest/virustest (RFC 5235).
	"require [\"relational\", \"spamtest\"];\nif spamtest :value \"ge\" \"5\" {\n\tdiscard;\n}\n",
	"require \"spamtestplus\";\nif spamtest :percent \"50\" {\n\tdiscard;\n}\n",
	"require \"virustest\";\nif virustest \"4\" {\n\tdiscard;\n}\n",
	// environment (RFC 5183).
	"require \"environment\";\nif environment :contains \"remote-host\" \"example.com\" {\n\tkeep;\n}\n",
	// duplicate (RFC 7352).
	"require \"duplicate\";\nif duplicate :handle \"notify\" :seconds 3600 {\n\tdiscard;\n}\n",
	// ihave / error (RFC 5463).
	"require \"ihave\";\nif not ihave \"vacation\" {\n\terror \"vacation not supported\";\n}\n",
	// vacation (RFC 5230).
	"require \"vacation\";\nvacation :days 7 :subject \"Away\" \"I am out of office.\";\n",
	"require \"vacation\";\nvacation \"out of office\";\n",
	// notify / enotify (RFC 5435).
	"require \"enotify\";\nnotify :importance \"1\" \"mailto:admin@example.com\";\n",
	"require \"enotify\";\nif valid_notify_method \"mailto:x@example.com\" {\n\tstop;\n}\n",
	// editheader (RFC 5293).
	"require \"editheader\";\naddheader :last \"X-Filtered\" \"yes\";\n",
	"require \"editheader\";\ndeleteheader :index 2 :contains \"X-Spam-Flag\" \"YES\";\n",
	// date / index (RFC 5260).
	"require \"date\";\nif date \"received\" \"weekday\" \"1\" {\n\tkeep;\n}\n",
	"require [\"date\", \"relational\"];\nif currentdate :value \"ge\" \"date\" \"2026-06-21\" {\n\tdiscard;\n}\n",
	"require [\"fileinto\", \"index\"];\nif header :index 2 \"received\" \"x\" {\n\tfileinto \"X\";\n}\n",
	"require \"index\";\nif address :index 1 :last \"from\" \"x\" {\n\tkeep;\n}\n",
	"require [\"date\", \"index\"];\nif date :index 1 :zone \"+0100\" \"received\" \"hour\" \"09\" {\n\tkeep;\n}\n",
	// relational (RFC 5231): :count / :value derive "relational".
	"require \"relational\";\nif header :count \"ge\" \"received\" \"3\" {\n\tdiscard;\n}\n",
	"require [\"body\", \"relational\"];\nif body :value \"gt\" \"5\" {\n\tkeep;\n}\n",
	// subaddress (RFC 5233): :user / :detail derive "subaddress".
	"require [\"fileinto\", \"subaddress\"];\nif address :user \"to\" \"sales\" {\n\tfileinto \"Sales\";\n}\n",
	// regex (draft): :regex derives "regex".
	"require [\"fileinto\", \"regex\"];\nif header :regex \"subject\" \"^\\\\[ticket-[0-9]+\\\\]\" {\n\tfileinto \"Tickets\";\n}\n",
	// Multi-line text: block with dot-stuffing.
	"redirect text:\nline one\n..dotted\nline three\n.\n;\n",
}

func TestRoundTripCanonical(t *testing.T) {
	for _, src := range canonicalScripts {
		t.Run(src, func(t *testing.T) {
			s, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}
			out, err := s.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(out) != src {
				t.Errorf("round trip mismatch\n got: %q\nwant: %q", out, src)
			}
		})
	}
}

func TestParseEncodeIsFixedPoint(t *testing.T) {
	// Encoding any AST and parsing it back must re-encode identically.
	scripts := []*Script{
		{Commands: []Command{&Keep{}}},
		{Commands: []Command{&FileInto{Mailbox: "Inbox/Lists", Copy: true}}},
		{Commands: []Command{&If{
			Test: &AllOf{Tests: []Test{
				&HeaderTest{MatchType: MatchMatches, Headers: []string{"from"}, Keys: []string{"*@spam.example"}},
				&SizeTest{Over: false, Limit: 500},
			}},
			Then: []Command{&Discard{}},
		}}},
	}
	for i, s := range scripts {
		first, err := s.Encode()
		if err != nil {
			t.Fatalf("script %d Encode: %v", i, err)
		}
		reparsed, err := Parse(first)
		if err != nil {
			t.Fatalf("script %d Parse(%q): %v", i, first, err)
		}
		second, err := reparsed.Encode()
		if err != nil {
			t.Fatalf("script %d re-Encode: %v", i, err)
		}
		if string(first) != string(second) {
			t.Errorf("script %d not a fixed point\n first: %q\nsecond: %q", i, first, second)
		}
	}
}

func TestParseToleratesCommentsAndWhitespace(t *testing.T) {
	src := `# leading comment
require   "fileinto" ;   /* inline */
if header :contains "subject" "hi" {
    fileinto "X" ;  # trailing
}
`
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "require \"fileinto\";\nif header :contains \"subject\" \"hi\" {\n\tfileinto \"X\";\n}\n"
	out, _ := s.Encode()
	if string(out) != want {
		t.Errorf("canonicalized form\n got: %q\nwant: %q", out, want)
	}
}

func TestCarrierFallbacks(t *testing.T) {
	// A genuinely unknown tag on a known command -> *RawCommand.
	s, err := Parse([]byte(`fileinto :weird "Archive";`))
	if err != nil {
		t.Fatal(err)
	}
	if rc, ok := s.Commands[0].(*RawCommand); !ok {
		t.Errorf("fileinto :weird => %T, want *RawCommand", s.Commands[0])
	} else if rc.Name != "fileinto" {
		t.Errorf("carrier name = %q, want fileinto", rc.Name)
	}

	// A genuinely unknown tag on a known test -> *RawTest.
	s, err = Parse([]byte(`if header :novel "received" "3" { discard; }`))
	if err != nil {
		t.Fatal(err)
	}
	iff := s.Commands[0].(*If)
	if _, ok := iff.Test.(*RawTest); !ok {
		t.Errorf("header :novel => %T, want *RawTest", iff.Test)
	}

	// :count is now a modelled relational match-type, not a carrier.
	s, err = Parse([]byte(`if header :count "ge" "received" "3" { discard; }`))
	if err != nil {
		t.Fatal(err)
	}
	ht, ok := s.Commands[0].(*If).Test.(*HeaderTest)
	if !ok {
		t.Fatalf("header :count => %T, want *HeaderTest", s.Commands[0].(*If).Test)
	}
	if ht.MatchType != MatchCount || ht.Relational != "ge" {
		t.Errorf("header :count parsed as MatchType=%d Relational=%q", ht.MatchType, ht.Relational)
	}

	// Unmodelled control command with a test argument.
	s, err = Parse([]byte("mycontrol true {\n\tkeep;\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := s.Commands[0].(*RawCommand)
	if !ok {
		t.Fatalf("mycontrol => %T, want *RawCommand", s.Commands[0])
	}
	if _, ok := rc.Test.(*True); !ok {
		t.Errorf("carrier test = %T, want *True", rc.Test)
	}
	if !rc.HasBlock || len(rc.Block) != 1 {
		t.Errorf("carrier block = %v (HasBlock=%v), want one command", rc.Block, rc.HasBlock)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"keep",             // missing ;
		"if {",             // missing test
		"fileinto;",        // missing mailbox
		`require "x"`,      // missing ;
		"}",                // stray brace
		"if true { keep; ", // unterminated block
	}
	for _, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%q) = nil error, want a ParseError", src)
		} else if _, ok := err.(*ParseError); !ok {
			t.Errorf("Parse(%q) error = %T, want *ParseError", src, err)
		}
	}
}
