// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve_test

import (
	"fmt"

	"github.com/hstern/go-sieve"
)

// Build a script and encode it. The required capabilities are derived
// automatically and emitted as a single leading require.
func ExampleScript_Encode() {
	s := &sieve.Script{Commands: []sieve.Command{
		&sieve.FileInto{Mailbox: "Junk"},
		&sieve.Stop{},
	}}

	out, err := s.Encode()
	if err != nil {
		panic(err)
	}
	fmt.Print(string(out))
	// Output:
	// require "fileinto";
	// fileinto "Junk";
	// stop;
}

// Parse reads a script into the typed AST. Known commands become their
// concrete types; the AST can then be inspected or re-encoded.
func ExampleParse() {
	s, err := sieve.Parse([]byte(`require "fileinto";
if header :contains "subject" "hello" { fileinto "Greetings"; }`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d top-level commands\n", len(s.Commands))
	if _, ok := s.Commands[1].(*sieve.If); ok {
		fmt.Println("the second command is an if")
	}
	// Output:
	// 2 top-level commands
	// the second command is an if
}

// Validate reports the require-related MUSTs as fatal errors.
func ExampleScript_Validate() {
	s := &sieve.Script{Commands: []sieve.Command{
		&sieve.FileInto{Mailbox: "Junk"}, // uses fileinto without a require
	}}
	fmt.Println(s.Validate())
	// Output:
	// sieve: extension "fileinto" is used but not declared with require (RFC 5228 §3.2)
}

// Check separates fatal errors from non-fatal warnings — for example, a
// command this package does not model is preserved verbatim and reported
// as a warning rather than an error.
func ExampleScript_Check() {
	s, err := sieve.Parse([]byte(`require "vacation";
vacation "Away until Monday.";`))
	if err != nil {
		panic(err)
	}
	d := s.Check()
	fmt.Println("errors:", len(d.Errors))
	fmt.Println("warnings:", len(d.Warnings))
	// Output:
	// errors: 0
	// warnings: 1
}
