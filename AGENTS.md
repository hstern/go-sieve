# Contributor guide

`go-sieve` is a standard-library-only implementation of RFC 5228 Sieve.
These are the conventions for changes to this repository.

## Ground rules

- **Standard library only at runtime.** No third-party runtime
  dependencies. Build-time tooling (linters, release tooling) is
  unconstrained but never lands in consumers' `go.sum`.
- **Round-trip fidelity is the core invariant.** The emitter and parser
  are exact inverses. For a canonical script `s`, `Encode(Parse(s)) == s`;
  for any AST `a`, `Parse(Encode(a))` re-encodes identically. Any change
  must keep both invariants.
- **Byte-stable output.** No map iteration on any `Encode` path.
  Capability lists and tags are emitted in a fixed, sorted order.
- **Lenient parse, strict opt-in `Validate`.** `Parse` never rejects an
  unknown command/test (it produces a carrier node); structural MUSTs are
  enforced only by `Validate`.

## Per-file header

Every `.go` file (including tests) starts with exactly:

```go
// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0
```

## Before opening a PR

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
golangci-lint run # if installed locally; CI runs it regardless
```

CI runs three required checks — `static`, `test`, `lint` — on
`ubuntu-latest`. All three must be green before merge.

## Commit style

Imperative mood, concise subject (`Add parser carrier for unknown
tests`). One logical change per commit.
