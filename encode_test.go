// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import "testing"

func TestEncodeGolden(t *testing.T) {
	tests := []struct {
		name string
		in   *Script
		want string
	}{
		{
			name: "empty script",
			in:   &Script{},
			want: "",
		},
		{
			name: "keep and stop, no require",
			in: &Script{Commands: []Command{
				&Keep{},
				&Stop{},
			}},
			want: "keep;\nstop;\n",
		},
		{
			name: "fileinto derives require",
			in: &Script{Commands: []Command{
				&FileInto{Mailbox: "Junk"},
			}},
			want: "require \"fileinto\";\nfileinto \"Junk\";\n",
		},
		{
			name: "header contains, if/stop",
			in: &Script{Commands: []Command{
				&If{
					Test: &HeaderTest{
						MatchType: MatchContains,
						Headers:   []string{"subject"},
						Keys:      []string{"[SPAM]"},
					},
					Then: []Command{
						&FileInto{Mailbox: "Junk"},
						&Stop{},
					},
				},
			}},
			want: "require \"fileinto\";\n" +
				"if header :contains \"subject\" \"[SPAM]\" {\n" +
				"\tfileinto \"Junk\";\n" +
				"\tstop;\n" +
				"}\n",
		},
		{
			name: "multiple capabilities sorted and merged",
			in: &Script{Commands: []Command{
				&FileInto{Mailbox: "Archive", Copy: true},
				&SetFlag{Flags: []string{"\\Seen"}},
			}},
			want: "require [\"copy\", \"fileinto\", \"imap4flags\"];\n" +
				"fileinto :copy \"Archive\";\n" +
				"setflag \"\\\\Seen\";\n",
		},
		{
			name: "explicit require merged, body dropped",
			in: &Script{Commands: []Command{
				&Require{Capabilities: []string{"fileinto"}},
				&FileInto{Mailbox: "X"},
			}},
			want: "require \"fileinto\";\nfileinto \"X\";\n",
		},
		{
			name: "author require kept even if unused",
			in: &Script{Commands: []Command{
				&Require{Capabilities: []string{"vacation"}},
				&Keep{},
			}},
			want: "require \"vacation\";\nkeep;\n",
		},
		{
			name: "elsif and else",
			in: &Script{Commands: []Command{
				&If{
					Test: &SizeTest{Over: true, Limit: 1000000},
					Then: []Command{&Discard{}},
					Elsif: []Branch{{
						Test: &ExistsTest{Headers: []string{"x-spam"}},
						Then: []Command{&FileInto{Mailbox: "Junk"}},
					}},
					Else: []Command{&Keep{}},
				},
			}},
			want: "require \"fileinto\";\n" +
				"if size :over 1000000 {\n\tdiscard;\n} elsif exists \"x-spam\" {\n\tfileinto \"Junk\";\n} else {\n\tkeep;\n}\n",
		},
		{
			name: "anyof with address localpart",
			in: &Script{Commands: []Command{
				&If{
					Test: &AnyOf{Tests: []Test{
						&AddressTest{AddressPart: AddressLocalPart, MatchType: MatchIs, Headers: []string{"from"}, Keys: []string{"root"}},
						&Not{Test: &True{}},
					}},
					Then: []Command{&Discard{}},
				},
			}},
			want: "if anyof (address :localpart \"from\" \"root\", not true) {\n\tdiscard;\n}\n",
		},
		{
			name: "multiline body value",
			in: &Script{Commands: []Command{
				&Redirect{Address: "a@b.example"},
			}},
			want: "redirect \"a@b.example\";\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Encode mismatch\n got: %q\nwant: %q", got, tc.want)
			}
			// Byte-stability: encoding again yields identical bytes.
			again, _ := tc.in.Encode()
			if string(again) != string(got) {
				t.Errorf("Encode not deterministic:\n first: %q\nsecond: %q", got, again)
			}
		})
	}
}

func TestQuoteMultiline(t *testing.T) {
	got := quote("line1\n.hidden\nline3")
	want := "text:\nline1\n..hidden\nline3\n.\n"
	if got != want {
		t.Errorf("multiline quote\n got: %q\nwant: %q", got, want)
	}
}

func TestQuoteEscaping(t *testing.T) {
	got := quote(`a"b\c`)
	want := `"a\"b\\c"`
	if got != want {
		t.Errorf("quote escaping\n got: %q\nwant: %q", got, want)
	}
}
