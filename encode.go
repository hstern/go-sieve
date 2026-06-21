// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

import (
	"fmt"
	"strconv"
	"strings"
)

// Encode serialises the script to canonical Sieve text. The output is
// byte-stable (deterministic across runs) and self-contained: a single
// require listing the derived capabilities is emitted first, so callers
// never hand-maintain it. Explicit Require commands in the script are
// merged into that leading require and otherwise omitted.
//
// Encode only returns an error if the AST contains a command or test
// type this package does not recognise, which cannot happen for values
// built from the exported types (the Command and Test interfaces are
// sealed).
func (s *Script) Encode() ([]byte, error) {
	var e encoder
	if err := e.encodeScript(s); err != nil {
		return nil, err
	}
	return []byte(e.b.String()), nil
}

// String returns the canonical Sieve text for the script, or the empty
// string if the script cannot be encoded. Use [Script.Encode] when the
// error matters.
func (s *Script) String() string {
	out, _ := s.Encode()
	return string(out)
}

type encoder struct {
	b      strings.Builder
	indent int
}

func (e *encoder) encodeScript(s *Script) error {
	// Emit leading comments before the derived require so a comment at the
	// top of the script stays at the top (KeepComments mode).
	i := 0
	for i < len(s.Commands) {
		c, ok := s.Commands[i].(*Comment)
		if !ok {
			break
		}
		if err := e.encodeCommand(c); err != nil {
			return err
		}
		i++
	}
	if caps := s.Capabilities(); len(caps) > 0 {
		e.b.WriteString("require ")
		e.writeStringList(caps)
		e.b.WriteString(";\n")
	}
	for _, c := range s.Commands[i:] {
		// Explicit Require commands are folded into the single leading
		// require emitted above; never emit them again in the body.
		if _, ok := c.(*Require); ok {
			continue
		}
		if err := e.encodeCommand(c); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) encodeCommands(cmds []Command) error {
	for _, c := range cmds {
		if err := e.encodeCommand(c); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) writeIndent() {
	for i := 0; i < e.indent; i++ {
		e.b.WriteByte('\t')
	}
}

func (e *encoder) encodeCommand(c Command) error {
	e.writeIndent()
	switch v := c.(type) {
	case *Comment:
		if v.Bracket {
			e.b.WriteString("/*")
			e.b.WriteString(v.Text)
			e.b.WriteString("*/\n")
		} else {
			e.b.WriteByte('#')
			e.b.WriteString(v.Text)
			e.b.WriteByte('\n')
		}
	case *Require:
		e.b.WriteString("require ")
		e.writeStringList(v.Capabilities)
		e.b.WriteString(";\n")
	case *Stop:
		e.b.WriteString("stop;\n")
	case *Keep:
		e.b.WriteString("keep;\n")
	case *Discard:
		e.b.WriteString("discard;\n")
	case *FileInto:
		e.b.WriteString("fileinto")
		if v.Copy {
			e.b.WriteString(" :copy")
		}
		if v.Create {
			e.b.WriteString(" :create")
		}
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Mailbox))
		e.b.WriteString(";\n")
	case *Error:
		e.b.WriteString("error ")
		e.b.WriteString(quote(v.Message))
		e.b.WriteString(";\n")
	case *Redirect:
		e.b.WriteString("redirect")
		if v.Copy {
			e.b.WriteString(" :copy")
		}
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Address))
		e.b.WriteString(";\n")
	case *SetFlag:
		e.writeFlagCommand("setflag", v.Flags)
	case *AddFlag:
		e.writeFlagCommand("addflag", v.Flags)
	case *RemoveFlag:
		e.writeFlagCommand("removeflag", v.Flags)
	case *If:
		return e.encodeIf(v)
	case *RawCommand:
		return e.encodeRawCommand(v)
	default:
		return fmt.Errorf("sieve: cannot encode command of type %T", c)
	}
	return nil
}

func (e *encoder) writeFlagCommand(name string, flags []string) {
	e.b.WriteString(name)
	e.b.WriteByte(' ')
	e.writeStringList(flags)
	e.b.WriteString(";\n")
}

func (e *encoder) encodeIf(v *If) error {
	e.b.WriteString("if ")
	if err := e.encodeBlock(v.Test, v.Then); err != nil {
		return err
	}
	for _, br := range v.Elsif {
		e.b.WriteString(" elsif ")
		if err := e.encodeBlock(br.Test, br.Then); err != nil {
			return err
		}
	}
	if len(v.Else) > 0 {
		e.b.WriteString(" else {\n")
		e.indent++
		if err := e.encodeCommands(v.Else); err != nil {
			return err
		}
		e.indent--
		e.writeIndent()
		e.b.WriteByte('}')
	}
	e.b.WriteByte('\n')
	return nil
}

// encodeBlock writes "<test> {\n <body> }" with no trailing newline, so
// the caller can append elsif/else or the final newline.
func (e *encoder) encodeBlock(t Test, body []Command) error {
	if err := e.encodeTest(t); err != nil {
		return err
	}
	e.b.WriteString(" {\n")
	e.indent++
	if err := e.encodeCommands(body); err != nil {
		return err
	}
	e.indent--
	e.writeIndent()
	e.b.WriteByte('}')
	return nil
}

func (e *encoder) encodeRawCommand(v *RawCommand) error {
	e.b.WriteString(v.Name)
	for _, a := range v.Args {
		e.b.WriteByte(' ')
		e.encodeArg(a)
	}
	if v.Test != nil {
		e.b.WriteByte(' ')
		if err := e.encodeTest(v.Test); err != nil {
			return err
		}
	}
	if v.HasBlock {
		e.b.WriteString(" {\n")
		e.indent++
		if err := e.encodeCommands(v.Block); err != nil {
			return err
		}
		e.indent--
		e.writeIndent()
		e.b.WriteString("}\n")
		return nil
	}
	e.b.WriteString(";\n")
	return nil
}

func (e *encoder) encodeTest(t Test) error {
	switch v := t.(type) {
	case *True:
		e.b.WriteString("true")
	case *False:
		e.b.WriteString("false")
	case *HeaderTest:
		e.b.WriteString("header")
		e.writeIndex(v.Index, v.IndexLast)
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.writeStringList(v.Headers)
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *AddressTest:
		e.b.WriteString("address")
		e.writeIndex(v.Index, v.IndexLast)
		e.writeComparator(v.Comparator)
		e.writeAddressPart(v.AddressPart)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.writeStringList(v.Headers)
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *DateTest:
		e.b.WriteString("date")
		e.writeIndex(v.Index, v.IndexLast)
		e.writeZone(v.Zone, v.OriginalZone)
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Header))
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.DatePart))
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *CurrentDateTest:
		e.b.WriteString("currentdate")
		e.writeZone(v.Zone, false)
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.DatePart))
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *EnvelopeTest:
		e.b.WriteString("envelope")
		e.writeComparator(v.Comparator)
		e.writeAddressPart(v.AddressPart)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.writeStringList(v.Parts)
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *ExistsTest:
		e.b.WriteString("exists ")
		e.writeStringList(v.Headers)
	case *SizeTest:
		e.b.WriteString("size ")
		if v.Over {
			e.b.WriteString(":over ")
		} else {
			e.b.WriteString(":under ")
		}
		e.b.WriteString(strconv.FormatUint(v.Limit, 10))
	case *BodyTest:
		e.b.WriteString("body")
		e.writeComparator(v.Comparator)
		e.writeBodyTransform(v)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *MailboxExistsTest:
		e.b.WriteString("mailboxexists ")
		e.writeStringList(v.Mailboxes)
	case *SpamTest:
		e.b.WriteString("spamtest")
		if v.Percent {
			e.b.WriteString(" :percent")
		}
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Value))
	case *VirusTest:
		e.b.WriteString("virustest")
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Value))
	case *EnvironmentTest:
		e.b.WriteString("environment")
		e.writeComparator(v.Comparator)
		e.writeMatch(v.MatchType, v.Relational)
		e.b.WriteByte(' ')
		e.b.WriteString(quote(v.Name))
		e.b.WriteByte(' ')
		e.writeStringList(v.Keys)
	case *DuplicateTest:
		e.b.WriteString("duplicate")
		if v.Handle != "" {
			e.b.WriteString(" :handle ")
			e.b.WriteString(quote(v.Handle))
		}
		if v.Header != "" {
			e.b.WriteString(" :header ")
			e.b.WriteString(quote(v.Header))
		}
		if v.UniqueID != "" {
			e.b.WriteString(" :uniqueid ")
			e.b.WriteString(quote(v.UniqueID))
		}
		if v.HasSeconds {
			e.b.WriteString(" :seconds ")
			e.b.WriteString(strconv.FormatUint(v.Seconds, 10))
		}
		if v.Last {
			e.b.WriteString(" :last")
		}
	case *IHaveTest:
		e.b.WriteString("ihave ")
		e.writeStringList(v.Capabilities)
	case *AllOf:
		e.b.WriteString("allof ")
		return e.encodeTestList(v.Tests)
	case *AnyOf:
		e.b.WriteString("anyof ")
		return e.encodeTestList(v.Tests)
	case *Not:
		e.b.WriteString("not ")
		return e.encodeTest(v.Test)
	case *RawTest:
		e.b.WriteString(v.Name)
		for _, a := range v.Args {
			e.b.WriteByte(' ')
			e.encodeArg(a)
		}
	default:
		return fmt.Errorf("sieve: cannot encode test of type %T", t)
	}
	return nil
}

func (e *encoder) encodeTestList(tests []Test) error {
	e.b.WriteByte('(')
	for i, t := range tests {
		if i > 0 {
			e.b.WriteString(", ")
		}
		if err := e.encodeTest(t); err != nil {
			return err
		}
	}
	e.b.WriteByte(')')
	return nil
}

func (e *encoder) encodeArg(a Argument) {
	switch v := a.(type) {
	case StringArg:
		e.writeStringList(v.Values)
	case NumberArg:
		e.b.WriteString(strconv.FormatUint(v.Value, 10))
	case TagArg:
		e.b.WriteByte(':')
		e.b.WriteString(v.Name)
	}
}

func (e *encoder) writeIndex(index int, last bool) {
	if index > 0 {
		e.b.WriteString(" :index ")
		e.b.WriteString(strconv.Itoa(index))
		if last {
			e.b.WriteString(" :last")
		}
	}
}

func (e *encoder) writeZone(zone string, original bool) {
	if zone != "" {
		e.b.WriteString(" :zone ")
		e.b.WriteString(quote(zone))
	} else if original {
		e.b.WriteString(" :originalzone")
	}
}

func (e *encoder) writeMatch(m MatchType, relational string) {
	if m == MatchIs {
		return
	}
	e.b.WriteByte(' ')
	e.b.WriteString(m.tag())
	if m.relational() {
		e.b.WriteByte(' ')
		e.b.WriteString(quote(relational))
	}
}

func (e *encoder) writeAddressPart(a AddressPart) {
	if a != AddressAll {
		e.b.WriteByte(' ')
		e.b.WriteString(a.tag())
	}
}

func (e *encoder) writeComparator(c string) {
	if c != "" && c != defaultComparator {
		e.b.WriteString(" :comparator ")
		e.b.WriteString(quote(c))
	}
}

func (e *encoder) writeBodyTransform(v *BodyTest) {
	switch v.Transform {
	case BodyRaw:
		e.b.WriteString(" :raw")
	case BodyContent:
		e.b.WriteString(" :content ")
		e.writeStringList(v.ContentTypes)
	}
	// BodyText is the default and is omitted.
}

// writeStringList emits a Sieve string-list: a single element as a bare
// string, otherwise a bracketed, comma-separated list.
func (e *encoder) writeStringList(vals []string) {
	if len(vals) == 1 {
		e.b.WriteString(quote(vals[0]))
		return
	}
	e.b.WriteByte('[')
	for i, v := range vals {
		if i > 0 {
			e.b.WriteString(", ")
		}
		e.b.WriteString(quote(v))
	}
	e.b.WriteByte(']')
}

// quote renders a Sieve string: a quoted-string when it has no newline,
// otherwise a multi-line text: block (RFC 5228 §2.4.2). Canonical output
// uses LF line endings; a value with an embedded LF forces the multiline
// form.
func quote(s string) string {
	if strings.Contains(s, "\n") {
		return multiline(s)
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '\\' || c == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String()
}

// multiline renders a value as a text: block, dot-stuffing any line that
// begins with '.' and terminating with a line containing only '.'
// (RFC 5228 §2.4.2.2).
func multiline(s string) string {
	var b strings.Builder
	b.WriteString("text:\n")
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(line, ".") {
			b.WriteByte('.')
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(".\n")
	return b.String()
}
