# go-sieve

[![CI](https://github.com/hstern/go-sieve/actions/workflows/ci.yml/badge.svg)](https://github.com/hstern/go-sieve/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/hstern/go-sieve.svg)](https://pkg.go.dev/github.com/hstern/go-sieve)

A typed Go AST, byte-stable emitter, and tolerant parser for
**[RFC 5228 — Sieve: An Email Filtering Language](https://www.rfc-editor.org/rfc/rfc5228.html)**.

`go-sieve` lets you **build, serialise, and read back** Sieve filter
scripts without string-bashing: construct a script as typed values,
`Encode` it to canonical script text (with the required `require`
derived automatically), and `Parse` text back into the AST.

> **Not an executor.** [`github.com/foxcpp/go-sieve`](https://github.com/foxcpp/go-sieve)
> solves the *opposite* problem — it **evaluates** Sieve against a
> message. `go-sieve` **builds and serialises** Sieve (and parses it
> back). Different module path, complementary scope.

Standard library only. `go 1.26+`. Apache-2.0.

## Install

```sh
go get github.com/hstern/go-sieve
```

## Quickstart — build and encode

```go
package main

import (
	"fmt"

	"github.com/hstern/go-sieve"
)

func main() {
	s := &sieve.Script{Commands: []sieve.Command{
		&sieve.If{
			Test: &sieve.HeaderTest{
				MatchType: sieve.MatchContains,
				Headers:   []string{"subject"},
				Keys:      []string{"[SPAM]"},
			},
			Then: []sieve.Command{
				&sieve.FileInto{Mailbox: "Junk"},
				&sieve.Stop{},
			},
		},
	}}

	out, _ := s.Encode()
	fmt.Print(string(out))
}
```

Output — note the auto-derived `require`:

```sieve
require "fileinto";
if header :contains "subject" "[SPAM]" {
	fileinto "Junk";
	stop;
}
```

## Quickstart — parse and inspect

```go
s, err := sieve.Parse([]byte(script))
if err != nil {
	// *sieve.ParseError carries a line/column
}
for _, c := range s.Commands {
	// type-switch over the typed AST; unknown commands arrive as *sieve.RawCommand
}
```

## What it does

- **Typed AST** (RFC 5228 §2–§5): control (`Require`, `If`/elsif/else,
  `Stop`), actions (`Keep`, `Discard`, `FileInto`, `Redirect`), and the
  test tree (`HeaderTest`, `AddressTest`, `EnvelopeTest`, `ExistsTest`,
  `SizeTest`, `BodyTest`, `AllOf`/`AnyOf`/`Not`, `True`/`False`).
- **Extensions**: `fileinto`, `imap4flags` (RFC 5232), `copy`
  (RFC 3894), `body` (RFC 5173), `envelope`.
- **Byte-stable `Encode`**: one canonical form, deterministic output,
  a single leading `require` derived from the commands used.
- **Tolerant `Parse`**: comments and whitespace tolerated; unknown
  commands/tests — and unmodelled tags on known ones — preserved verbatim
  in `RawCommand`/`RawTest` so hand-edited scripts round-trip. Pass
  `Parse(b, KeepComments())` to retain command-level comments.
- **Opt-in `Validate`**: checks the obvious MUSTs (every used extension
  is `require`d, `require` precedes use, tag legality).

## Round-trip guarantee

For a canonical script `s`, `Encode(Parse(s)) == s`. For any AST `a`,
`Parse(Encode(a))` re-encodes identically. Sieve whitespace is
insignificant, so `Encode` picks exactly one canonical layout.

## Status

Pre-1.0 (`v0.x`). The API may shift before `v1.0.0`; the round-trip and
byte-stability guarantees will not.

## License

Apache-2.0 — see [LICENSE](LICENSE).
