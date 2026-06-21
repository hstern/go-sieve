// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

// Package sieve builds, serialises, and parses Sieve email-filter
// scripts as defined by RFC 5228 — Sieve: An Email Filtering Language.
//
// It provides a typed abstract syntax tree (the [Script] / [Command] /
// [Test] types), a byte-stable emitter ([Script.Encode] / [Script.String])
// that derives the required capabilities and emits a single leading
// require automatically, and a tolerant parser ([Parse]) that reads a
// script back into the AST. Unknown commands and tests are preserved
// verbatim in carrier nodes ([RawCommand] / [RawTest]) so a hand-edited
// script round-trips even when it uses an extension this package does
// not model.
//
// This package does not execute Sieve against a message — evaluating a
// script is a different problem solved by github.com/foxcpp/go-sieve.
// This package is the complementary half: it constructs and serialises
// scripts (and parses them back).
//
// The supported extensions at this version are fileinto (RFC 5228),
// imap4flags (RFC 5232), copy (RFC 3894), body (RFC 5173), and envelope.
package sieve

// SpecVersion is the RFC 5228 — Sieve: An Email Filtering Language
// version this build implements.
const SpecVersion = "RFC 5228"
