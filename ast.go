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
// Create adds the :create tag (RFC 5490 mailbox), creating the mailbox if
// it does not exist.
type FileInto struct {
	Mailbox string
	Copy    bool
	Create  bool
}

// Error aborts processing with a message (RFC 5463 ihave/error extension).
type Error struct {
	Message string
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

// Comment is a command-level comment, preserved only when a script is
// parsed with [KeepComments]. Text is the comment body without its
// delimiters; Bracket selects /* ... */ framing over the default
// # ... line comment. Comments that appear inside an expression (between
// a test's arguments, say) are not modelled and are dropped on parse.
type Comment struct {
	Text    string
	Bracket bool
}

func (*Comment) isCommand()    {}
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
func (*Error) isCommand()      {}
func (*RawCommand) isCommand() {}

// ---- Tests (RFC 5228 §5) ----

// HeaderTest matches header field values (§5.7). Index/IndexLast select a
// single occurrence among repeated headers (RFC 5260 index).
type HeaderTest struct {
	MatchType  MatchType
	Relational string // relational operator for :count/:value (RFC 5231)
	Comparator string // "" means the default i;ascii-casemap
	Index      int    // :index N, 0 = unset (require "index")
	IndexLast  bool   // :last
	Headers    []string
	Keys       []string
}

// AddressTest matches addresses in structured header fields (§5.1).
type AddressTest struct {
	MatchType   MatchType
	Relational  string // relational operator for :count/:value (RFC 5231)
	AddressPart AddressPart
	Comparator  string
	Index       int  // :index N, 0 = unset (require "index")
	IndexLast   bool // :last
	Headers     []string
	Keys        []string
}

// EnvelopeTest matches the SMTP envelope (§5.4; envelope extension).
type EnvelopeTest struct {
	MatchType   MatchType
	Relational  string // relational operator for :count/:value (RFC 5231)
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
	Relational   string // relational operator for :count/:value (RFC 5231)
	Comparator   string
	Keys         []string
}

// DateTest matches a date in a header field (RFC 5260 date). DatePart is
// e.g. "year", "month", "day", "date", "time", "hour", "weekday".
type DateTest struct {
	Index        int    // :index N, 0 = unset (require "index")
	IndexLast    bool   // :last
	Zone         string // :zone "+0100", "" = unset
	OriginalZone bool   // :originalzone
	MatchType    MatchType
	Relational   string
	Comparator   string
	Header       string
	DatePart     string
	Keys         []string
}

// CurrentDateTest matches the current date (RFC 5260 date).
type CurrentDateTest struct {
	Zone       string // :zone "+0100", "" = unset
	MatchType  MatchType
	Relational string
	Comparator string
	DatePart   string
	Keys       []string
}

// MailboxExistsTest is true if every named mailbox exists (RFC 5490
// mailbox).
type MailboxExistsTest struct {
	Mailboxes []string
}

// SpamTest matches the message's spam score against Value (RFC 5235
// spamtest). Percent adds the :percent tag (require "spamtestplus").
type SpamTest struct {
	Percent    bool
	MatchType  MatchType
	Relational string
	Comparator string
	Value      string
}

// VirusTest matches the message's virus-check score against Value
// (RFC 5235 virustest).
type VirusTest struct {
	MatchType  MatchType
	Relational string
	Comparator string
	Value      string
}

// EnvironmentTest matches a named environment item (RFC 5183 environment),
// e.g. "name", "host", "remote-host".
type EnvironmentTest struct {
	MatchType  MatchType
	Relational string
	Comparator string
	Name       string
	Keys       []string
}

// DuplicateTest is true when the message is a duplicate by tracked id
// (RFC 7352 duplicate). Header and UniqueID are mutually exclusive.
type DuplicateTest struct {
	Handle     string
	Header     string
	UniqueID   string
	Seconds    uint64
	HasSeconds bool
	Last       bool
}

// IHaveTest is true if the implementation supports all listed capabilities
// (RFC 5463 ihave), enabling graceful feature-testing.
type IHaveTest struct {
	Capabilities []string
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

func (*HeaderTest) isTest()        {}
func (*AddressTest) isTest()       {}
func (*EnvelopeTest) isTest()      {}
func (*ExistsTest) isTest()        {}
func (*SizeTest) isTest()          {}
func (*BodyTest) isTest()          {}
func (*DateTest) isTest()          {}
func (*CurrentDateTest) isTest()   {}
func (*MailboxExistsTest) isTest() {}
func (*SpamTest) isTest()          {}
func (*VirusTest) isTest()         {}
func (*EnvironmentTest) isTest()   {}
func (*DuplicateTest) isTest()     {}
func (*IHaveTest) isTest()         {}
func (*True) isTest()              {}
func (*False) isTest()             {}
func (*AllOf) isTest()             {}
func (*AnyOf) isTest()             {}
func (*Not) isTest()               {}
func (*RawTest) isTest()           {}

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

// Match-type tags (RFC 5228 §2.7.1, plus the relational and regex
// extensions). MatchCount and MatchValue carry a relational operator in
// the test's Relational field (RFC 5231); MatchRegex is the regex
// extension (draft-murchison-sieve-regex).
const (
	MatchIs       MatchType = iota // :is (default)
	MatchContains                  // :contains
	MatchMatches                   // :matches
	MatchCount                     // :count <relational> (require "relational")
	MatchValue                     // :value <relational> (require "relational")
	MatchRegex                     // :regex (require "regex")
)

func (m MatchType) tag() string {
	switch m {
	case MatchContains:
		return ":contains"
	case MatchMatches:
		return ":matches"
	case MatchCount:
		return ":count"
	case MatchValue:
		return ":value"
	case MatchRegex:
		return ":regex"
	default:
		return ":is"
	}
}

// relational reports whether the match-type takes a relational operator
// argument (the :count / :value forms of RFC 5231).
func (m MatchType) relational() bool { return m == MatchCount || m == MatchValue }

// AddressPart is a Sieve address-part (§2.7.2). The zero value
// AddressAll is the default and is omitted from canonical output.
type AddressPart int

// Address-part tags (RFC 5228 §2.7.2, plus the subaddress extension).
// AddressUser and AddressDetail are the subaddress extension (RFC 5233).
const (
	AddressAll       AddressPart = iota // :all (default)
	AddressLocalPart                    // :localpart
	AddressDomain                       // :domain
	AddressUser                         // :user (require "subaddress")
	AddressDetail                       // :detail (require "subaddress")
)

func (a AddressPart) tag() string {
	switch a {
	case AddressLocalPart:
		return ":localpart"
	case AddressDomain:
		return ":domain"
	case AddressUser:
		return ":user"
	case AddressDetail:
		return ":detail"
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
