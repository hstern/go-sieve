# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Script.Check` now returns located diagnostics (`Diagnostic.Path`) and
  warns about degenerate shapes (empty header/key lists, empty
  `allof`/`anyof`, empty targets).
- Comparator legality: a non-built-in comparator derives a
  `comparator-<name>` capability, enforced by both auto-`require` and
  `Validate` (RFC 5228 §2.7.3 / RFC 4790).
- `:matches` patterns ending in a dangling backslash escape now warn.
- `Parse(b, KeepComments())` preserves comments as `Comment` nodes; the
  default remains comment-free. No comment content is dropped — including
  mid-expression comments (repositioned to the nearest command boundary) —
  and leading comments are emitted before the auto-derived `require`.
- Example showing how a ManageSieve (RFC 5804) `CheckScript` backend maps
  onto `Parse`/`Check` without coupling the libraries.

### Changed

- Parser is now fully Postel-tolerant: an unmodelled tag on a known
  command/test, and an unmodelled control command taking a test argument,
  are preserved as carriers instead of being rejected.

## [0.1.0] - 2026-06-21

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

[Unreleased]: https://github.com/hstern/go-sieve/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hstern/go-sieve/releases/tag/v0.1.0
