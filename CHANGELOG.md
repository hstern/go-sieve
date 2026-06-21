# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Typed AST for RFC 5228 Sieve: control commands (`Require`, `If`/elsif/
  else, `Stop`), action commands (`Keep`, `Discard`, `FileInto`,
  `Redirect`), and the test tree.
- Byte-stable `Encode`/`String` with automatic `require` derivation.
- Tolerant `Parse` with `RawCommand`/`RawTest` carriers for unknown
  constructs.
- `Validate` for the spec MUSTs.
- Extensions: `fileinto`, `imap4flags` (RFC 5232), `copy` (RFC 3894),
  `body` (RFC 5173), `envelope`.

[Unreleased]: https://github.com/hstern/go-sieve/commits/main
