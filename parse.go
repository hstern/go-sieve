// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// errUnknownTag is an internal sentinel: a known command or test hit a tag
// this package does not model. The dispatcher catches it, rewinds, and
// re-parses the whole construct as a carrier so it round-trips verbatim.
var errUnknownTag = errors.New("sieve: unknown tag")

// ParseError reports a syntax error in a Sieve script, with the 1-based
// line and column where it was detected. A ParseError is fatal: the
// script could not be read into an AST.
type ParseError struct {
	Line int
	Col  int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("sieve: line %d:%d: %s", e.Line, e.Col, e.Msg)
}

// ParseOption configures [Parse].
type ParseOption func(*parseConfig)

type parseConfig struct {
	keepComments bool
}

// KeepComments makes [Parse] preserve comments as [Comment] nodes
// (re-emitted by the encoder) instead of discarding them. No comment
// content is lost: a comment that appeared mid-expression is retained but
// repositioned to the nearest command boundary on its own line — exact
// inline placement is not preserved, since the AST is canonical rather
// than a lossless syntax tree. Comments that lead the script are emitted
// before the automatically derived require.
func KeepComments() ParseOption {
	return func(c *parseConfig) { c.keepComments = true }
}

// Parse reads canonical or hand-written Sieve text into a [Script].
// Comments and insignificant whitespace are tolerated. Commands and
// tests this package does not model are preserved verbatim in
// [RawCommand] / [RawTest] carrier nodes, so a script using an
// unmodelled extension still round-trips. Parse does not enforce the
// require-before-use rule or other structural MUSTs — call
// [Script.Validate] for that.
//
// By default comments are discarded; pass [KeepComments] to preserve them.
func Parse(b []byte, opts ...ParseOption) (*Script, error) {
	var cfg parseConfig
	for _, o := range opts {
		o(&cfg)
	}
	toks, err := lex(string(b), cfg.keepComments)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	cmds, err := p.parseCommands(false)
	if err != nil {
		return nil, err
	}
	return &Script{Commands: cmds}, nil
}

// ---- Parser ----

type parser struct {
	toks []token
	pos  int
	// pendingComments holds comment tokens skipped mid-expression (only in
	// KeepComments mode); the command loop drains them into Comment nodes so
	// no comment content is lost, repositioned to the nearest command
	// boundary.
	pendingComments []token
}

// peek and next transparently skip comment tokens (present only when
// parsing with KeepComments); the command loop drains them separately into
// Comment nodes via the raw token slice. Skipping them here means a comment
// inside an expression is simply ignored.
func (p *parser) peek() token {
	i := p.pos
	for i < len(p.toks) && p.toks[i].kind == tComment {
		i++
	}
	if i < len(p.toks) {
		return p.toks[i]
	}
	return token{kind: tEOF}
}

func (p *parser) next() token {
	for p.pos < len(p.toks) && p.toks[p.pos].kind == tComment {
		// A comment reached here is mid-expression; keep it so the command
		// loop can re-emit it rather than dropping it.
		p.pendingComments = append(p.pendingComments, p.toks[p.pos])
		p.pos++
	}
	if p.pos < len(p.toks) {
		t := p.toks[p.pos]
		p.pos++
		return t
	}
	return token{kind: tEOF}
}

func (p *parser) errAt(t token, msg string) error {
	return &ParseError{Line: t.line, Col: t.col, Msg: msg}
}

func (p *parser) parseCommands(inBlock bool) ([]Command, error) {
	var cmds []Command
	for {
		// Drain any pending command-level comments into Comment nodes (only
		// present when parsing with KeepComments).
		for p.pos < len(p.toks) && p.toks[p.pos].kind == tComment {
			ct := p.toks[p.pos]
			p.pos++
			cmds = append(cmds, &Comment{Text: ct.text, Bracket: ct.num == 1})
		}
		t := p.peek()
		switch t.kind {
		case tEOF:
			if inBlock {
				return nil, p.errAt(t, "unexpected end of input, expected }")
			}
			return cmds, nil
		case tRBrace:
			if !inBlock {
				return nil, p.errAt(t, "unexpected }")
			}
			return cmds, nil
		}
		c, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		// Re-emit any comments seen mid-expression before this command so
		// their content survives (their exact inline position does not).
		for _, ct := range p.pendingComments {
			cmds = append(cmds, &Comment{Text: ct.text, Bracket: ct.num == 1})
		}
		p.pendingComments = p.pendingComments[:0]
		cmds = append(cmds, c)
	}
}

func (p *parser) parseCommand() (Command, error) {
	t := p.next()
	if t.kind != tIdent {
		return nil, p.errAt(t, "expected command name")
	}
	switch t.text {
	case "require":
		list, err := p.parseStringList()
		if err != nil {
			return nil, err
		}
		if err := p.expectSemicolon(); err != nil {
			return nil, err
		}
		return &Require{Capabilities: list}, nil
	case "stop":
		return &Stop{}, p.expectSemicolon()
	case "keep":
		return &Keep{}, p.expectSemicolon()
	case "discard":
		return &Discard{}, p.expectSemicolon()
	case "if":
		return p.parseIf()
	case "elsif":
		return nil, p.errAt(t, "elsif without matching if")
	case "else":
		return nil, p.errAt(t, "else without matching if")
	case "fileinto":
		return p.knownCommandOrRaw("fileinto", p.parseFileInto)
	case "redirect":
		return p.knownCommandOrRaw("redirect", p.parseRedirect)
	case "setflag", "addflag", "removeflag":
		// The no-variable flag commands take only a string-list; any tag
		// means an unmodelled form, so preserve the whole command.
		if p.peek().kind == tTag {
			return p.parseRawCommand(t.text)
		}
		return p.parseFlagCommand(t.text)
	default:
		return p.parseRawCommand(t.text)
	}
}

// knownCommandOrRaw runs a modelled-command parser, but if it reports an
// unmodelled tag, rewinds to just after the command name and re-parses the
// whole command as a carrier so it round-trips verbatim.
func (p *parser) knownCommandOrRaw(name string, parse func() (Command, error)) (Command, error) {
	start := p.pos
	c, err := parse()
	if errors.Is(err, errUnknownTag) {
		p.pos = start
		return p.parseRawCommand(name)
	}
	return c, err
}

func (p *parser) parseIf() (Command, error) {
	test, err := p.parseTest()
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	n := &If{Test: test, Then: then}
	for {
		t := p.peek()
		if t.kind != tIdent || t.text != "elsif" {
			break
		}
		p.next()
		et, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		eb, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		n.Elsif = append(n.Elsif, Branch{Test: et, Then: eb})
	}
	if t := p.peek(); t.kind == tIdent && t.text == "else" {
		p.next()
		eb, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		n.Else = eb
	}
	return n, nil
}

func (p *parser) parseBlock() ([]Command, error) {
	if t := p.next(); t.kind != tLBrace {
		return nil, p.errAt(t, "expected {")
	}
	cmds, err := p.parseCommands(true)
	if err != nil {
		return nil, err
	}
	if t := p.next(); t.kind != tRBrace {
		return nil, p.errAt(t, "expected }")
	}
	return cmds, nil
}

func (p *parser) parseFileInto() (Command, error) {
	f := &FileInto{}
	for p.peek().kind == tTag {
		if p.peek().text != "copy" {
			return nil, errUnknownTag
		}
		p.next()
		f.Copy = true
	}
	mbox, err := p.parseSingleString()
	if err != nil {
		return nil, err
	}
	f.Mailbox = mbox
	return f, p.expectSemicolon()
}

func (p *parser) parseRedirect() (Command, error) {
	r := &Redirect{}
	for p.peek().kind == tTag {
		if p.peek().text != "copy" {
			return nil, errUnknownTag
		}
		p.next()
		r.Copy = true
	}
	addr, err := p.parseSingleString()
	if err != nil {
		return nil, err
	}
	r.Address = addr
	return r, p.expectSemicolon()
}

func (p *parser) parseFlagCommand(name string) (Command, error) {
	flags, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	if err := p.expectSemicolon(); err != nil {
		return nil, err
	}
	switch name {
	case "setflag":
		return &SetFlag{Flags: flags}, nil
	case "addflag":
		return &AddFlag{Flags: flags}, nil
	default:
		return &RemoveFlag{Flags: flags}, nil
	}
}

func (p *parser) parseRawCommand(name string) (Command, error) {
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	rc := &RawCommand{Name: name, Args: args}
	// An unmodelled control command may take a test argument before its
	// block (e.g. a custom if-like command).
	if p.peek().kind == tIdent {
		test, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		rc.Test = test
		rc.Block = block
		rc.HasBlock = true
		return rc, nil
	}
	t := p.next()
	switch t.kind {
	case tSemicolon:
		return rc, nil
	case tLBrace:
		cmds, err := p.parseCommands(true)
		if err != nil {
			return nil, err
		}
		if b := p.next(); b.kind != tRBrace {
			return nil, p.errAt(b, "expected }")
		}
		rc.HasBlock = true
		rc.Block = cmds
		return rc, nil
	default:
		return nil, p.errAt(t, "expected ; or { after "+name)
	}
}

// ---- Tests ----

func (p *parser) parseTest() (Test, error) {
	t := p.next()
	if t.kind != tIdent {
		return nil, p.errAt(t, "expected test name")
	}
	switch t.text {
	case "true":
		return &True{}, nil
	case "false":
		return &False{}, nil
	case "not":
		inner, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		return &Not{Test: inner}, nil
	case "allof":
		tests, err := p.parseTestList()
		if err != nil {
			return nil, err
		}
		return &AllOf{Tests: tests}, nil
	case "anyof":
		tests, err := p.parseTestList()
		if err != nil {
			return nil, err
		}
		return &AnyOf{Tests: tests}, nil
	case "exists":
		list, err := p.parseStringList()
		if err != nil {
			return nil, err
		}
		return &ExistsTest{Headers: list}, nil
	case "size":
		return p.parseSizeTest()
	case "header":
		return p.knownTestOrRaw("header", p.parseHeaderTest)
	case "address":
		return p.knownTestOrRaw("address", p.parseAddressTest)
	case "envelope":
		return p.knownTestOrRaw("envelope", p.parseEnvelopeTest)
	case "body":
		return p.knownTestOrRaw("body", p.parseBodyTest)
	default:
		return p.parseRawTest(t.text)
	}
}

// knownTestOrRaw is the test-side counterpart of knownCommandOrRaw: on an
// unmodelled tag it rewinds and preserves the whole test as a carrier.
func (p *parser) knownTestOrRaw(name string, parse func() (Test, error)) (Test, error) {
	start := p.pos
	t, err := parse()
	if errors.Is(err, errUnknownTag) {
		p.pos = start
		return p.parseRawTest(name)
	}
	return t, err
}

func (p *parser) parseTestList() ([]Test, error) {
	if t := p.next(); t.kind != tLParen {
		return nil, p.errAt(t, "expected (")
	}
	var tests []Test
	if p.peek().kind == tRParen {
		p.next()
		return tests, nil
	}
	for {
		t, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		tests = append(tests, t)
		n := p.next()
		switch n.kind {
		case tComma:
			continue
		case tRParen:
			return tests, nil
		default:
			return nil, p.errAt(n, "expected , or )")
		}
	}
}

func (p *parser) parseSizeTest() (Test, error) {
	tag := p.next()
	if tag.kind != tTag || (tag.text != "over" && tag.text != "under") {
		return nil, p.errAt(tag, "size requires :over or :under")
	}
	num := p.next()
	if num.kind != tNumber {
		return nil, p.errAt(num, "size requires a number")
	}
	return &SizeTest{Over: tag.text == "over", Limit: num.num}, nil
}

// applyMatchTag handles the match-type and comparator tags common to the
// comparison tests, including the relational (:count/:value, RFC 5231) and
// regex (:regex) extensions. It reports whether the tag was recognised.
func (p *parser) applyMatchTag(tag token, mt *MatchType, rel, comp *string) (bool, error) {
	switch tag.text {
	case "is":
		*mt = MatchIs
	case "contains":
		*mt = MatchContains
	case "matches":
		*mt = MatchMatches
	case "regex":
		*mt = MatchRegex
	case "count", "value":
		if tag.text == "count" {
			*mt = MatchCount
		} else {
			*mt = MatchValue
		}
		s, err := p.parseSingleString()
		if err != nil {
			return false, err
		}
		*rel = s
	case "comparator":
		s, err := p.parseSingleString()
		if err != nil {
			return false, err
		}
		*comp = s
	default:
		return false, nil
	}
	return true, nil
}

func applyAddressPartTag(tag token, ap *AddressPart) bool {
	switch tag.text {
	case "all":
		*ap = AddressAll
	case "localpart":
		*ap = AddressLocalPart
	case "domain":
		*ap = AddressDomain
	case "user":
		*ap = AddressUser
	case "detail":
		*ap = AddressDetail
	default:
		return false
	}
	return true
}

func (p *parser) parseHeaderTest() (Test, error) {
	h := &HeaderTest{}
	for p.peek().kind == tTag {
		tag := p.next()
		ok, err := p.applyMatchTag(tag, &h.MatchType, &h.Relational, &h.Comparator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errUnknownTag
		}
	}
	headers, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	h.Headers, h.Keys = headers, keys
	return h, nil
}

func (p *parser) parseAddressTest() (Test, error) {
	a := &AddressTest{}
	for p.peek().kind == tTag {
		tag := p.next()
		if applyAddressPartTag(tag, &a.AddressPart) {
			continue
		}
		ok, err := p.applyMatchTag(tag, &a.MatchType, &a.Relational, &a.Comparator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errUnknownTag
		}
	}
	headers, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	a.Headers, a.Keys = headers, keys
	return a, nil
}

func (p *parser) parseEnvelopeTest() (Test, error) {
	e := &EnvelopeTest{}
	for p.peek().kind == tTag {
		tag := p.next()
		if applyAddressPartTag(tag, &e.AddressPart) {
			continue
		}
		ok, err := p.applyMatchTag(tag, &e.MatchType, &e.Relational, &e.Comparator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errUnknownTag
		}
	}
	parts, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	e.Parts, e.Keys = parts, keys
	return e, nil
}

func (p *parser) parseBodyTest() (Test, error) {
	b := &BodyTest{}
	for p.peek().kind == tTag {
		tag := p.next()
		switch tag.text {
		case "raw":
			b.Transform = BodyRaw
			continue
		case "text":
			b.Transform = BodyText
			continue
		case "content":
			b.Transform = BodyContent
			ct, err := p.parseStringList()
			if err != nil {
				return nil, err
			}
			b.ContentTypes = ct
			continue
		}
		ok, err := p.applyMatchTag(tag, &b.MatchType, &b.Relational, &b.Comparator)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errUnknownTag
		}
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	b.Keys = keys
	return b, nil
}

func (p *parser) parseRawTest(name string) (Test, error) {
	// A combinator-style unknown test taking a parenthesised test list is
	// not modelled; unknown tests take plain arguments.
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	return &RawTest{Name: name, Args: args}, nil
}

// ---- Shared argument parsing ----

func (p *parser) parseArgs() ([]Argument, error) {
	var args []Argument
	for {
		switch p.peek().kind {
		case tTag:
			args = append(args, TagArg{Name: p.next().text})
		case tNumber:
			args = append(args, NumberArg{Value: p.next().num})
		case tString, tLBracket:
			list, err := p.parseStringList()
			if err != nil {
				return nil, err
			}
			args = append(args, StringArg{Values: list})
		default:
			return args, nil
		}
	}
}

func (p *parser) parseStringList() ([]string, error) {
	t := p.peek()
	switch t.kind {
	case tString:
		p.next()
		return []string{t.text}, nil
	case tLBracket:
		p.next()
		var out []string
		if p.peek().kind == tRBracket {
			p.next()
			return out, nil
		}
		for {
			s := p.next()
			if s.kind != tString {
				return nil, p.errAt(s, "expected string in list")
			}
			out = append(out, s.text)
			n := p.next()
			switch n.kind {
			case tComma:
				continue
			case tRBracket:
				return out, nil
			default:
				return nil, p.errAt(n, "expected , or ]")
			}
		}
	default:
		return nil, p.errAt(t, "expected string or string list")
	}
}

func (p *parser) parseSingleString() (string, error) {
	t := p.next()
	if t.kind != tString {
		return "", p.errAt(t, "expected string")
	}
	return t.text, nil
}

func (p *parser) expectSemicolon() error {
	if t := p.next(); t.kind != tSemicolon {
		return p.errAt(t, "expected ;")
	}
	return nil
}

// ---- Lexer ----

type tokenKind int

const (
	tEOF tokenKind = iota
	tIdent
	tTag
	tNumber
	tString
	tLBracket
	tRBracket
	tLBrace
	tRBrace
	tLParen
	tRParen
	tComma
	tSemicolon
	tComment // emitted only when keepComments is set
)

type token struct {
	kind tokenKind
	text string
	num  uint64
	line int
	col  int
}

type lexer struct {
	src          string
	pos          int
	line         int
	col          int
	keepComments bool
}

func lex(src string, keepComments bool) ([]token, error) {
	l := &lexer{src: src, line: 1, col: 1, keepComments: keepComments}
	var toks []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, t)
		if t.kind == tEOF {
			return toks, nil
		}
	}
}

func (l *lexer) eof() bool { return l.pos >= len(l.src) }
func (l *lexer) cur() byte { return l.src[l.pos] }

func (l *lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *lexer) next() (token, error) {
	for {
		l.skipSpace()
		if l.eof() {
			return token{kind: tEOF, line: l.line, col: l.col}, nil
		}
		if !l.atComment() {
			break
		}
		tok, err := l.lexComment()
		if err != nil {
			return token{}, err
		}
		if l.keepComments {
			return tok, nil
		}
		// otherwise loop and skip it
	}
	line, col := l.line, l.col
	if strings.HasPrefix(l.src[l.pos:], "text:") {
		val, err := l.lexMultiline()
		if err != nil {
			return token{}, err
		}
		return token{kind: tString, text: val, line: line, col: col}, nil
	}
	c := l.cur()
	switch {
	case c == '"':
		val, err := l.lexQuoted()
		if err != nil {
			return token{}, err
		}
		return token{kind: tString, text: val, line: line, col: col}, nil
	case c == ':':
		l.advance()
		name := l.lexIdentChars()
		if name == "" {
			return token{}, &ParseError{Line: line, Col: col, Msg: "empty tag"}
		}
		return token{kind: tTag, text: name, line: line, col: col}, nil
	case c >= '0' && c <= '9':
		return l.lexNumber(line, col)
	case isIdentStart(c):
		return token{kind: tIdent, text: l.lexIdentChars(), line: line, col: col}, nil
	}
	l.advance()
	kind, ok := singleCharToken(c)
	if !ok {
		return token{}, &ParseError{Line: line, Col: col, Msg: fmt.Sprintf("unexpected character %q", c)}
	}
	return token{kind: kind, line: line, col: col}, nil
}

func singleCharToken(c byte) (tokenKind, bool) {
	switch c {
	case '[':
		return tLBracket, true
	case ']':
		return tRBracket, true
	case '{':
		return tLBrace, true
	case '}':
		return tRBrace, true
	case '(':
		return tLParen, true
	case ')':
		return tRParen, true
	case ',':
		return tComma, true
	case ';':
		return tSemicolon, true
	default:
		return tEOF, false
	}
}

func (l *lexer) skipSpace() {
	for !l.eof() {
		switch l.cur() {
		case ' ', '\t', '\r', '\n':
			l.advance()
		default:
			return
		}
	}
}

func (l *lexer) atComment() bool {
	if l.eof() {
		return false
	}
	c := l.cur()
	return c == '#' || (c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*')
}

// lexComment consumes a hash (#) or bracket (/* */) comment and returns it
// as a tComment token. The token text is the body without delimiters; num
// is 1 for a bracket comment, 0 for a hash comment.
func (l *lexer) lexComment() (token, error) {
	line, col := l.line, l.col
	if l.cur() == '#' {
		l.advance() // consume '#'
		start := l.pos
		for !l.eof() && l.cur() != '\n' {
			l.advance()
		}
		text := strings.TrimSuffix(l.src[start:l.pos], "\r")
		return token{kind: tComment, text: text, num: 0, line: line, col: col}, nil
	}
	l.advance() // consume '/'
	l.advance() // consume '*'
	start := l.pos
	for !l.eof() {
		if l.cur() == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			text := l.src[start:l.pos]
			l.advance() // consume '*'
			l.advance() // consume '/'
			return token{kind: tComment, text: text, num: 1, line: line, col: col}, nil
		}
		l.advance()
	}
	return token{}, &ParseError{Line: line, Col: col, Msg: "unterminated /* */ comment"}
}

func (l *lexer) lexQuoted() (string, error) {
	line, col := l.line, l.col
	l.advance() // opening quote
	var b strings.Builder
	for {
		if l.eof() {
			return "", &ParseError{Line: line, Col: col, Msg: "unterminated string"}
		}
		c := l.advance()
		switch c {
		case '\\':
			if l.eof() {
				return "", &ParseError{Line: line, Col: col, Msg: "unterminated string"}
			}
			b.WriteByte(l.advance()) // backslash drops; the next byte is literal
		case '"':
			return b.String(), nil
		default:
			b.WriteByte(c)
		}
	}
}

func (l *lexer) lexMultiline() (string, error) {
	line, col := l.line, l.col
	for range len("text:") {
		l.advance()
	}
	// Ignore the remainder of the "text:" line (optional whitespace/comment).
	for !l.eof() && l.cur() != '\n' {
		l.advance()
	}
	if !l.eof() {
		l.advance() // consume the newline
	}
	var lines []string
	for {
		if l.eof() {
			return "", &ParseError{Line: line, Col: col, Msg: "unterminated text: block"}
		}
		start := l.pos
		for !l.eof() && l.cur() != '\n' {
			l.advance()
		}
		raw := l.src[start:l.pos]
		if !l.eof() {
			l.advance() // consume the newline
		}
		raw = strings.TrimSuffix(raw, "\r")
		if raw == "." {
			return strings.Join(lines, "\n"), nil
		}
		raw = strings.TrimPrefix(raw, ".")
		lines = append(lines, raw)
	}
}

func (l *lexer) lexNumber(line, col int) (token, error) {
	start := l.pos
	for !l.eof() && l.cur() >= '0' && l.cur() <= '9' {
		l.advance()
	}
	digits := l.src[start:l.pos]
	var mult uint64 = 1
	if !l.eof() {
		switch l.cur() {
		case 'K', 'k':
			mult = 1 << 10
			l.advance()
		case 'M', 'm':
			mult = 1 << 20
			l.advance()
		case 'G', 'g':
			mult = 1 << 30
			l.advance()
		}
	}
	n, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return token{}, &ParseError{Line: line, Col: col, Msg: "number out of range"}
	}
	return token{kind: tNumber, num: n * mult, line: line, col: col}, nil
}

func (l *lexer) lexIdentChars() string {
	start := l.pos
	for !l.eof() && isIdentChar(l.cur()) {
		l.advance()
	}
	return l.src[start:l.pos]
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}
