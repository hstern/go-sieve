// Copyright 2026 The go-sieve Authors
// SPDX-License-Identifier: Apache-2.0

package sieve

// Script is an ordered list of Sieve commands — the root of the AST
// (RFC 5228 §2.8). The zero value is an empty script that encodes to
// nothing.
type Script struct {
	Commands []Command
}

// Command is a Sieve command: a control command (Require, If, Stop), an
// action command (Keep, Discard, FileInto, Redirect, and the imap4flags
// actions), or a RawCommand carrier for an unmodelled command. The set
// of concrete types is closed (the marker method is unexported);
// unknown commands from the parser arrive as *RawCommand.
type Command interface {
	isCommand()
}

// Test is a Sieve test: a leaf test (HeaderTest, AddressTest,
// EnvelopeTest, ExistsTest, SizeTest, BodyTest, True, False), a
// combinator (AllOf, AnyOf, Not), or a RawTest carrier for an
// unmodelled test.
type Test interface {
	isTest()
}

// ---- Control commands (RFC 5228 §3) ----

// Require declares the extension capabilities a script uses (§3.2). The
// encoder derives the required capabilities automatically and emits a
// single leading require; an explicit Require is merged into it (its
// capabilities are never dropped, even if unused).
type Require struct {
	Capabilities []string
}

// Stop ends all processing (§3.3).
type Stop struct{}

// If is a conditional with optional elsif branches and an optional else
// block (§3.1).
type If struct {
	Test  Test
	Then  []Command
	Elsif []Branch
	Else  []Command // nil means no else block
}

// Branch is a single elsif clause of an If.
type Branch struct {
	Test Test
	Then []Command
}

// ---- Action commands (RFC 5228 §4) ----

// Keep files the message into the default mailbox (§4.1).
type Keep struct{}

// Discard silently throws the message away (§4.2).
type Discard struct{}

// FileInto files the message into Mailbox (RFC 5228 fileinto extension).
// Copy adds the :copy tag (RFC 3894), leaving the implicit keep intact.
type FileInto struct {
	Mailbox string
	Copy    bool
}

// Redirect forwards the message to Address (§4.4). Copy adds the :copy
// tag (RFC 3894).
type Redirect struct {
	Address string
	Copy    bool
}

// SetFlag replaces the set of IMAP flags (RFC 5232 imap4flags).
type SetFlag struct {
	Flags []string
}

// AddFlag adds IMAP flags (RFC 5232 imap4flags).
type AddFlag struct {
	Flags []string
}

// RemoveFlag removes IMAP flags (RFC 5232 imap4flags).
type RemoveFlag struct {
	Flags []string
}

// RawCommand carries a command this package does not model so that a
// hand-edited script round-trips. It re-emits verbatim (canonically
// re-quoted). It also captures a known command (or test) that carried a
// tag this package does not model — the whole construct is preserved as a
// RawCommand rather than rejected.
//
// Test is non-nil when the command was an unmodelled control structure
// that took a test argument before its block (e.g. a custom if-like
// command). HasBlock is true when the command was followed by a { ... }
// block rather than terminated by ';'.
type RawCommand struct {
	Name     string
	Args     []Argument
	Test     Test
	Block    []Command
	HasBlock bool
}

func (*Require) isCommand()    {}
func (*Stop) isCommand()       {}
func (*If) isCommand()         {}
func (*Keep) isCommand()       {}
func (*Discard) isCommand()    {}
func (*FileInto) isCommand()   {}
func (*Redirect) isCommand()   {}
func (*SetFlag) isCommand()    {}
func (*AddFlag) isCommand()    {}
func (*RemoveFlag) isCommand() {}
func (*RawCommand) isCommand() {}

// ---- Tests (RFC 5228 §5) ----

// HeaderTest matches header field values (§5.7).
type HeaderTest struct {
	MatchType  MatchType
	Comparator string // "" means the default i;ascii-casemap
	Headers    []string
	Keys       []string
}

// AddressTest matches addresses in structured header fields (§5.1).
type AddressTest struct {
	MatchType   MatchType
	AddressPart AddressPart
	Comparator  string
	Headers     []string
	Keys        []string
}

// EnvelopeTest matches the SMTP envelope (§5.4; envelope extension).
type EnvelopeTest struct {
	MatchType   MatchType
	AddressPart AddressPart
	Comparator  string
	Parts       []string // envelope-part, e.g. "from", "to"
	Keys        []string
}

// ExistsTest is true if every named header exists (§5.5).
type ExistsTest struct {
	Headers []string
}

// SizeTest compares the message size (§5.9). Over selects :over (else
// :under).
type SizeTest struct {
	Over  bool
	Limit uint64
}

// BodyTest matches the message body (RFC 5173 body extension).
type BodyTest struct {
	Transform    BodyTransform
	ContentTypes []string // only meaningful when Transform is BodyContent
	MatchType    MatchType
	Comparator   string
	Keys         []string
}

// True always matches (§5.10).
type True struct{}

// False never matches (§5.3).
type False struct{}

// AllOf is the logical AND of its tests (§5.2).
type AllOf struct {
	Tests []Test
}

// AnyOf is the logical OR of its tests (§5.6, anyof).
type AnyOf struct {
	Tests []Test
}

// Not inverts a test (§5.8).
type Not struct {
	Test Test
}

// RawTest carries a test this package does not model so that a
// hand-edited script round-trips.
type RawTest struct {
	Name string
	Args []Argument
}

func (*HeaderTest) isTest()   {}
func (*AddressTest) isTest()  {}
func (*EnvelopeTest) isTest() {}
func (*ExistsTest) isTest()   {}
func (*SizeTest) isTest()     {}
func (*BodyTest) isTest()     {}
func (*True) isTest()         {}
func (*False) isTest()        {}
func (*AllOf) isTest()        {}
func (*AnyOf) isTest()        {}
func (*Not) isTest()          {}
func (*RawTest) isTest()      {}

// ---- Arguments (for carrier nodes) ----

// Argument is one argument of a RawCommand or RawTest: a string list, a
// number, or a tag.
type Argument interface {
	isArgument()
}

// StringArg is a Sieve string-list argument. A single element encodes as
// a bare string; multiple elements encode as a bracketed list.
type StringArg struct {
	Values []string
}

// NumberArg is a numeric argument. Quantifier suffixes (K/M/G) are
// folded into Value on parse; the encoder emits the plain integer.
type NumberArg struct {
	Value uint64
}

// TagArg is a tagged argument such as :copy. Name omits the leading
// colon.
type TagArg struct {
	Name string
}

func (StringArg) isArgument() {}
func (NumberArg) isArgument() {}
func (TagArg) isArgument()    {}

// ---- Tagged-argument enums ----

// MatchType is a Sieve match-type (§2.7.1). The zero value MatchIs is
// the default and is omitted from canonical output.
type MatchType int

// Match-type tags (RFC 5228 §2.7.1).
const (
	MatchIs       MatchType = iota // :is (default)
	MatchContains                  // :contains
	MatchMatches                   // :matches
)

func (m MatchType) tag() string {
	switch m {
	case MatchContains:
		return ":contains"
	case MatchMatches:
		return ":matches"
	default:
		return ":is"
	}
}

// AddressPart is a Sieve address-part (§2.7.2). The zero value
// AddressAll is the default and is omitted from canonical output.
type AddressPart int

// Address-part tags (RFC 5228 §2.7.2).
const (
	AddressAll       AddressPart = iota // :all (default)
	AddressLocalPart                    // :localpart
	AddressDomain                       // :domain
)

func (a AddressPart) tag() string {
	switch a {
	case AddressLocalPart:
		return ":localpart"
	case AddressDomain:
		return ":domain"
	default:
		return ":all"
	}
}

// BodyTransform is the transform of a BodyTest (RFC 5173). The zero
// value BodyText is the default and is omitted from canonical output.
type BodyTransform int

// Body-transform tags (RFC 5173).
const (
	BodyText    BodyTransform = iota // :text (default)
	BodyRaw                          // :raw
	BodyContent                      // :content <content-type-list>
)

// defaultComparator is the comparator assumed when none is given
// (§2.7.3). It is omitted from canonical output.
const defaultComparator = "i;ascii-casemap"
