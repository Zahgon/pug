// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-lexer); MIT licence, see LICENSE.

// Package lexer turns pug source into tokens. It follows the original pug-lexer step by step, including its error
// codes, messages and positions; columns count UTF-16 code units, as JavaScript string offsets do.
package lexer

import (
	"regexp"
	"strings"

	pugerror "github.com/pugjs/pug/packages/pug-error"

	"github.com/pugjs/pug/internal/charparser"
	"github.com/pugjs/pug/internal/js"
)

// Options configure a lexer.
type Options struct {
	Filename string
	// Interpolated is set for the lexer of a #[...] tag interpolation.
	Interpolated bool
	// StartingLine and StartingColumn default to 1.
	StartingLine   int
	StartingColumn int
}

// Lexer is the state of one lexing run.
type Lexer struct {
	input                string
	originalInput        string
	filename             string
	interpolated         bool
	lineno               int
	colno                int
	indentStack          []int
	indentRe             *regexp.Regexp
	interpolationAllowed bool
	tokens               []Token
	ended                bool
}

var (
	bomRe   = regexp.MustCompile(`^\x{FEFF}`)
	crlfRe  = regexp.MustCompile(`\r\n|\r`)
	tabsRe  = regexp.MustCompile(`^\n(\t*) *`)
	spaceRe = regexp.MustCompile(`^\n( *)`)
)

// Lex returns the tokens of src.
func Lex(src string, o Options) ([]Token, error) {
	l := New(src, o)
	return l.Tokens()
}

// New returns a lexer for src.
func New(src string, o Options) *Lexer {
	src = bomRe.ReplaceAllString(src, "")
	input := crlfRe.ReplaceAllString(src, "\n")
	l := &Lexer{input: input, originalInput: input, filename: o.Filename, interpolated: o.Interpolated,
		lineno: 1, colno: 1, indentStack: []int{0}, interpolationAllowed: true}
	if o.StartingLine != 0 {
		l.lineno = o.StartingLine
	}
	if o.StartingColumn != 0 {
		l.colno = o.StartingColumn
	}
	return l
}

// Tokens lexes the whole input.
func (l *Lexer) Tokens() ([]Token, error) {
	for !l.ended {
		if err := l.advance(); err != nil {
			return nil, err
		}
	}
	return l.tokens, nil
}

func (l *Lexer) error(code, message string) error {
	return pugerror.New(code, message, pugerror.Options{Line: l.lineno, Column: l.colno, Filename: l.filename,
		Src: l.originalInput, HasSrc: true})
}

// u16len is the length of s in UTF-16 code units.
func u16len(s string) int {
	return js.Length(s)
}

// assertExpression checks that exp is a JavaScript expression; with noThrow it only reports the result.
func (l *Lexer) assertExpression(exp string, noThrow bool) (bool, error) {
	if isExpression(exp) {
		return true, nil
	}
	if noThrow {
		return false, nil
	}
	return false, l.error("SYNTAX_ERROR", "Syntax Error: Unexpected token")
}

// isExpression reports whether exp is one expression without line comments, as is-expression decides.
func isExpression(exp string) bool {
	if strings.Contains(exp, "//") && hasLineComment(exp) {
		return false
	}
	return js.IsExpression(exp)
}

// hasLineComment reports whether exp holds a // comment outside strings.
func hasLineComment(exp string) bool {
	st := &charparser.State{}
	for _, c := range js.ToUTF16(exp) {
		before := st.IsComment()
		if err := st.ParseChar(c); err != nil {
			return false
		}
		if !before && st.IsComment() && c == '/' {
			return true
		}
	}
	return false
}

func (l *Lexer) assertNestingCorrect(exp string) error {
	st, err := charparser.Parse(exp)
	if err != nil {
		return parserError(err)
	}
	if st.IsNesting() {
		return l.error("INCORRECT_NESTING", "Nesting must match on expression `"+exp+"`")
	}
	return nil
}

// parserError converts an error of the character parser that pug does not catch into the JavaScript error that
// escapes the original.
func parserError(err error) error {
	if ce, ok := err.(*charparser.Error); ok && ce.Code == "CHARACTER_PARSER:MISMATCHED_BRACKET" {
		return &js.Error{Name: "SyntaxError", Message: ce.Message}
	}
	return &js.Error{Name: "Error", Message: err.Error()}
}

func (l *Lexer) tok(typ string) Token {
	return Token{Type: typ, Loc: Loc{Start: Pos{l.lineno, l.colno}, Filename: l.filename}}
}

func (l *Lexer) tokVal(typ, val string) Token {
	t := l.tok(typ)
	t.Val, t.HasVal = val, true
	return t
}

func (l *Lexer) tokEnd(t Token) Token {
	t.Loc.End = Pos{l.lineno, l.colno}
	return t
}

func (l *Lexer) push(t Token) {
	l.tokens = append(l.tokens, l.tokEnd(t))
}

func (l *Lexer) incrementLine(n int) {
	l.lineno += n
	if n != 0 {
		l.colno = 1
	}
}

func (l *Lexer) incrementColumn(n int) {
	l.colno += n
}

// consume drops n UTF-16 code units of input.
func (l *Lexer) consume(n int) {
	l.input = js.From(l.input, n)
}

// consumeText drops the prefix text (a match at the start of the input).
func (l *Lexer) consumeText(text string) {
	l.input = l.input[len(text):]
}

// scan matches re at the start of the input; the token's value is the first group, the column advances past the
// text before it.
func (l *Lexer) scan(re *regexp.Regexp, typ string) (Token, bool) {
	m := re.FindStringSubmatchIndex(l.input)
	if m == nil {
		return Token{}, false
	}
	whole := l.input[m[0]:m[1]]
	t := l.tok(typ)
	valLen := 0
	if len(m) > 2 && m[2] >= 0 {
		t.Val, t.HasVal = l.input[m[2]:m[3]], true
		valLen = u16len(t.Val)
	}
	l.consumeText(whole)
	l.incrementColumn(u16len(whole) - valLen)
	return t, true
}

var (
	leadingSpacesRe = regexp.MustCompile(`^([ ]+)([^ ]*)`)
	restOfLineRe    = regexp.MustCompile(`^[ \t]*(\n|$)`)
	spacesTabsRe    = regexp.MustCompile(`^[ \t]*`)
)

// scanEndOfLine matches re when the rest of the line after the match is blank or starts with a colon.
func (l *Lexer) scanEndOfLine(re *regexp.Regexp, typ string) (Token, bool) {
	m := re.FindStringSubmatchIndex(l.input)
	if m == nil {
		return Token{}, false
	}
	whole := l.input[m[0]:m[1]]
	val, hasVal := "", false
	if len(m) > 2 && m[2] >= 0 {
		val, hasVal = l.input[m[2]:m[3]], true
	}
	wsLen := 0
	if ws := leadingSpacesRe.FindStringSubmatch(whole); ws != nil {
		wsLen = u16len(ws[1])
		l.incrementColumn(wsLen)
	}
	newInput := l.input[len(whole):]
	mk := func() Token {
		t := l.tok(typ)
		t.Val, t.HasVal = val, hasVal
		l.incrementColumn(u16len(whole) - wsLen)
		return t
	}
	if strings.HasPrefix(newInput, ":") {
		l.input = newInput
		return mk(), true
	}
	if restOfLineRe.MatchString(newInput) {
		l.input = newInput[len(spacesTabsRe.FindString(newInput)):]
		return mk(), true
	}
	return Token{}, false
}

// bracketExpression finds the bracket that closes the one at offset skip of the input.
func (l *Lexer) bracketExpression(skip int) (charparser.Range, error) {
	start := js.UnitAt(l.input, skip)
	var end string
	switch start {
	case '(':
		end = ")"
	case '{':
		end = "}"
	case '[':
		end = "]"
	default:
		return charparser.Range{}, &js.Error{Name: "AssertionError [ERR_ASSERTION]",
			Message: `The start character should be "(", "{" or "["`}
	}
	r, err := charparser.ParseUntil(l.input, end, skip+1)
	if err == nil {
		return r, nil
	}
	ce := err.(*charparser.Error)
	idx := ce.Index
	tmp := js.IndexOf(js.From(l.input, skip), "\n", 0)
	nextNewline := tmp + skip
	ptr := 0
	for idx > nextNewline && tmp != -1 {
		l.incrementLine(1)
		idx -= nextNewline + 1
		ptr += nextNewline + 1
		tmp = js.IndexOf(js.From(l.input, ptr), "\n", 0)
		nextNewline = tmp
	}
	l.incrementColumn(idx)
	if ce.Code == "CHARACTER_PARSER:END_OF_STRING_REACHED" {
		return r, l.error("NO_END_BRACKET", "The end of the string reached with no closing bracket "+end+" found.")
	}
	return r, l.error("BRACKET_MISMATCH", ce.Message)
}

func (l *Lexer) scanIndentation() []string {
	if l.indentRe != nil {
		return l.indentRe.FindStringSubmatch(l.input)
	}
	re := tabsRe
	captures := re.FindStringSubmatch(l.input)
	if captures != nil && len(captures[1]) == 0 {
		re = spaceRe
		captures = re.FindStringSubmatch(l.input)
	}
	if captures != nil && len(captures[1]) > 0 {
		l.indentRe = re
	}
	return captures
}

func (l *Lexer) eos() (bool, error) {
	if l.input != "" {
		return false, nil
	}
	if l.interpolated {
		return false, l.error("NO_END_BRACKET", "End of line was reached with no closing bracket for interpolation.")
	}
	for i := 0; i < len(l.indentStack) && l.indentStack[i] != 0; i++ {
		l.push(l.tok("outdent"))
	}
	l.push(l.tok("eos"))
	l.ended = true
	return true, nil
}

var blankRe = regexp.MustCompile(`^\n[ \t]*\n`)

func (l *Lexer) blank() bool {
	if m := blankRe.FindString(l.input); m != "" {
		l.consumeText(m[:len(m)-1])
		l.incrementLine(1)
		return true
	}
	return false
}

var commentRe = regexp.MustCompile(`^//(-)?([^\n]*)`)

func (l *Lexer) comment() (bool, error) {
	m := commentRe.FindStringSubmatch(l.input)
	if m == nil {
		return false, nil
	}
	l.consumeText(m[0])
	t := l.tokVal("comment", m[2])
	t.Buffer = m[1] != "-"
	l.interpolationAllowed = t.Buffer
	l.incrementColumn(u16len(m[0]))
	l.tokens = append(l.tokens, l.tokEnd(t))
	_, err := l.pipelessText(0)
	return true, err
}

func (l *Lexer) interpolation() (bool, error) {
	if !strings.HasPrefix(l.input, "#{") {
		return false, nil
	}
	match, err := l.bracketExpression(1)
	if err != nil {
		return false, err
	}
	l.consume(match.End + 1)
	t := l.tokVal("interpolation", match.Src)
	l.incrementColumn(2)
	if _, err := l.assertExpression(match.Src, false); err != nil {
		return false, err
	}
	lines := strings.Split(match.Src, "\n")
	l.incrementLine(len(lines) - 1)
	l.incrementColumn(u16len(lines[len(lines)-1]) + 1)
	l.tokens = append(l.tokens, l.tokEnd(t))
	return true, nil
}

var tagRe = regexp.MustCompile(`^(\w(?:[-:\w]*\w)?)`)

func (l *Lexer) tag() bool {
	m := tagRe.FindString(l.input)
	if m == "" {
		return false
	}
	l.consumeText(m)
	t := l.tokVal("tag", m)
	l.incrementColumn(u16len(m))
	l.push(t)
	return true
}

var filterRe = regexp.MustCompile(`^:([\w\-]+)`)

func (l *Lexer) filter(inInclude bool) (bool, error) {
	t, ok := l.scan(filterRe, "filter")
	if !ok {
		return false, nil
	}
	l.incrementColumn(u16len(t.Val))
	l.push(t)
	if _, err := l.attrs(); err != nil {
		return false, err
	}
	if !inInclude {
		l.interpolationAllowed = false
		if _, err := l.pipelessText(0); err != nil {
			return false, err
		}
	}
	return true, nil
}

var doctypeRe = regexp.MustCompile(`^doctype *([^\n]*)`)

func (l *Lexer) doctype() bool {
	t, ok := l.scanEndOfLine(doctypeRe, "doctype")
	if ok {
		l.push(t)
	}
	return ok
}

var (
	idRe      = regexp.MustCompile(`^#([\w-]+)`)
	invalidRe = regexp.MustCompile(`[^\n\x{2028}\x{2029}][^ \t\(\#\.\:]*`)
)

// invalidName returns what the original reports as the invalid name after the first character of the input, or
// ok false where the original fails to find one (and crashes).
func (l *Lexer) invalidName() (string, error) {
	m := invalidRe.FindString(js.From(l.input, 1))
	if m == "" {
		return "", &js.Error{Name: "TypeError", Message: "Cannot read properties of null (reading '0')"}
	}
	return m, nil
}

func (l *Lexer) id() (bool, error) {
	if t, ok := l.scan(idRe, "id"); ok {
		l.incrementColumn(u16len(t.Val))
		l.push(t)
		return true, nil
	}
	if strings.HasPrefix(l.input, "#") {
		name, err := l.invalidName()
		if err != nil {
			return false, err
		}
		return false, l.error("INVALID_ID", `"`+name+`" is not a valid ID.`)
	}
	return false, nil
}

var (
	classRe        = regexp.MustCompile(`^\.([_a-zA-Z0-9\-]*[_a-zA-Z][_a-zA-Z0-9\-]*)`)
	classDigitsRe  = regexp.MustCompile(`^\.[_a-zA-Z0-9\-]+`)
	classMessage   = `" is not a valid class name.  Class names can only contain "_", "-", a-z and 0-9, and must contain at least one of "_", or a-z`
	noLetterMessag = "Class names must contain at least one letter or underscore."
)

func (l *Lexer) className() (bool, error) {
	if t, ok := l.scan(classRe, "class"); ok {
		l.incrementColumn(u16len(t.Val))
		l.push(t)
		return true, nil
	}
	if classDigitsRe.MatchString(l.input) {
		return false, l.error("INVALID_CLASS_NAME", noLetterMessag)
	}
	if strings.HasPrefix(l.input, ".") {
		name, err := l.invalidName()
		if err != nil {
			return false, err
		}
		return false, l.error("INVALID_CLASS_NAME", `"`+name+classMessage)
	}
	return false, nil
}

func (l *Lexer) endInterpolation() bool {
	if l.interpolated && strings.HasPrefix(l.input, "]") {
		l.input = l.input[1:]
		l.ended = true
		return true
	}
	return false
}

var stringInterpRe = regexp.MustCompile(`(\\)?([#!])\{((?:[^\r\x{2028}\x{2029}])*)$`)

// addText pushes text tokens for value, splitting out #[...] tag interpolations, #{...} and !{...} code
// interpolations and their escaped forms.
func (l *Lexer) addText(typ, value string) error {
	return l.addTextPrefix(typ, value, "", 0, false)
}

// addTextPrefix is addText continuing after already-processed text: prefix, from which escaped escape
// backslashes were removed. Without a prefix (hasPrefix false) even empty text makes a token, as the original's
// check against an undefined prefix never matches.
func (l *Lexer) addTextPrefix(typ, value, prefix string, escaped int, hasPrefix bool) error {
	if hasPrefix && value+prefix == "" {
		return nil
	}
	const inf = 1 << 30
	indexOfEnd, indexOfStart, indexOfEscaped := -1, -1, -1
	if l.interpolated {
		indexOfEnd = js.IndexOf(value, "]", 0)
	}
	if l.interpolationAllowed {
		indexOfStart = js.IndexOf(value, "#[", 0)
		indexOfEscaped = js.IndexOf(value, "\\#[", 0)
	}
	m := stringInterpRe.FindStringSubmatchIndex(value)
	indexOfStringInterp := inf
	if l.interpolationAllowed && m != nil {
		indexOfStringInterp = u16len(value[:m[0]])
	}
	if indexOfEnd == -1 {
		indexOfEnd = inf
	}
	if indexOfStart == -1 {
		indexOfStart = inf
	}
	if indexOfEscaped == -1 {
		indexOfEscaped = inf
	}

	if indexOfEscaped != inf && indexOfEscaped < indexOfEnd && indexOfEscaped < indexOfStart &&
		indexOfEscaped < indexOfStringInterp {
		prefix = prefix + js.Slice(value, 0, indexOfEscaped) + "#["
		return l.addTextPrefix(typ, js.From(value, indexOfEscaped+3), prefix, escaped+1, true)
	}
	if indexOfStart != inf && indexOfStart < indexOfEnd && indexOfStart < indexOfEscaped &&
		indexOfStart < indexOfStringInterp {
		t := l.tokVal(typ, prefix+js.Slice(value, 0, indexOfStart))
		l.incrementColumn(u16len(prefix) + indexOfStart + escaped)
		l.push(t)
		t = l.tok("start-pug-interpolation")
		l.incrementColumn(2)
		l.push(t)
		child := New(js.From(value, indexOfStart+2), Options{Filename: l.filename, Interpolated: true,
			StartingLine: l.lineno, StartingColumn: l.colno})
		interpolated, err := child.Tokens()
		if err != nil {
			if pe, ok := err.(*pugerror.Error); ok {
				l.colno = pe.Column
				return l.error(strings.TrimPrefix(pe.Code, "PUG:"), pe.Msg)
			}
			return err
		}
		l.colno = child.colno
		l.tokens = append(l.tokens, interpolated...)
		t = l.tok("end-pug-interpolation")
		l.incrementColumn(1)
		l.push(t)
		return l.addText(typ, child.input)
	}
	if indexOfEnd != inf && indexOfEnd < indexOfStart && indexOfEnd < indexOfEscaped && indexOfEnd < indexOfStringInterp {
		if prefix+js.Slice(value, 0, indexOfEnd) != "" {
			if err := l.addTextPrefix(typ, js.Slice(value, 0, indexOfEnd), prefix, 0, true); err != nil {
				return err
			}
		}
		l.ended = true
		l.input = js.From(value, js.IndexOf(value, "]", 0)+1) + l.input
		return nil
	}
	if indexOfStringInterp != inf {
		if m[2] >= 0 {
			prefix = prefix + js.Slice(value, 0, indexOfStringInterp) + value[m[4]:m[5]] + "{"
			return l.addTextPrefix(typ, js.From(value, indexOfStringInterp+3), prefix, escaped+1, true)
		}
		before := js.Slice(value, 0, indexOfStringInterp)
		if prefix != "" || before != "" {
			before = prefix + before
			t := l.tokVal(typ, before)
			l.incrementColumn(u16len(before) + escaped)
			l.push(t)
		}
		rest := value[m[6]:m[7]]
		t := l.tok("interpolated-code")
		l.incrementColumn(2)
		r, err := charparser.ParseUntil(rest, "}", 0)
		if err != nil {
			ce := err.(*charparser.Error)
			l.incrementColumn(ce.Index)
			if ce.Code == "CHARACTER_PARSER:END_OF_STRING_REACHED" {
				return l.error("NO_END_BRACKET", "End of line was reached with no closing bracket for interpolation.")
			}
			return l.error("BRACKET_MISMATCH", ce.Message)
		}
		t.MustEscape = value[m[4]:m[5]] == "#"
		t.Buffer = true
		t.Val, t.HasVal = r.Src, true
		if _, err := l.assertExpression(r.Src, false); err != nil {
			return err
		}
		if r.End+1 < u16len(rest) {
			rest = js.From(rest, r.End+1)
			l.incrementColumn(r.End + 1)
			l.push(t)
			return l.addText(typ, rest)
		}
		l.incrementColumn(u16len(rest))
		l.push(t)
		return nil
	}
	value = prefix + value
	t := l.tokVal(typ, value)
	l.incrementColumn(u16len(value) + escaped)
	l.push(t)
	return nil
}

var (
	textRe1  = regexp.MustCompile(`^(?:\| ?| )([^\n]+)`)
	textRe2  = regexp.MustCompile(`^( )`)
	textRe3  = regexp.MustCompile(`^\|( ?)`)
	htmlRe   = regexp.MustCompile(`^(<[^\n]*)`)
	dotRe    = regexp.MustCompile(`^\.`)
	extendRe = regexp.MustCompile(`^extends?`)
)

func (l *Lexer) text() (bool, error) {
	t, ok := l.scan(textRe1, "text")
	if !ok {
		t, ok = l.scan(textRe2, "text")
	}
	if !ok {
		t, ok = l.scan(textRe3, "text")
	}
	if !ok {
		return false, nil
	}
	return true, l.addText("text", t.Val)
}

func (l *Lexer) textHTML() (bool, error) {
	t, ok := l.scan(htmlRe, "text-html")
	if !ok {
		return false, nil
	}
	return true, l.addText("text-html", t.Val)
}

func (l *Lexer) dot() (bool, error) {
	t, ok := l.scanEndOfLine(dotRe, "dot")
	if !ok {
		return false, nil
	}
	l.push(t)
	_, err := l.pipelessText(0)
	return true, err
}

// keywordAt reports whether the input starts with word followed by one of the given lookahead conditions: a space,
// a colon, a newline or the end of the input.
func (l *Lexer) keywordAt(word string, colon bool) bool {
	if !strings.HasPrefix(l.input, word) {
		return false
	}
	rest := l.input[len(word):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\n' || (colon && rest[0] == ':')
}

// wordAt reports whether the input starts with word at a word boundary (JavaScript's \b).
func (l *Lexer) wordAt(word string) bool {
	if !strings.HasPrefix(l.input, word) {
		return false
	}
	rest := l.input[len(word):]
	return rest == "" || !isWordByte(rest[0])
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (l *Lexer) scanWord(word, typ string) Token {
	t := l.tok(typ)
	l.consumeText(word)
	l.incrementColumn(len(word))
	return t
}

func (l *Lexer) extends() (bool, error) {
	for _, w := range []string{"extends", "extend"} {
		if l.keywordAt(w, false) {
			l.push(l.scanWord(w, "extends"))
			ok, err := l.path()
			if err != nil {
				return false, err
			}
			if !ok {
				return false, l.error("NO_EXTENDS_PATH", "missing path for extends")
			}
			return true, nil
		}
	}
	for _, w := range []string{"extends", "extend"} {
		if l.wordAt(w) {
			l.scanWord(w, "")
			return false, l.error("MALFORMED_EXTENDS", "malformed extends")
		}
	}
	return false, nil
}

var (
	prependRe = regexp.MustCompile(`^(?:block +)?prepend +([^\n]+)`)
	appendRe  = regexp.MustCompile(`^(?:block +)?append +([^\n]+)`)
	blockRe   = regexp.MustCompile(`^block +([^\n]+)`)
)

func isWhitespaceUnit(c int) bool {
	return c == ' ' || c == '\n' || c == '\t'
}

// namedBlock lexes block, append and prepend lines; a trailing // comment stays in the input.
func (l *Lexer) namedBlock(re *regexp.Regexp, mode string) bool {
	m := re.FindStringSubmatch(l.input)
	if m == nil {
		return false
	}
	name := js.TrimSpace(m[1])
	comment := ""
	if strings.Contains(name, "//") {
		parts := strings.Split(name, "//")
		comment = "//" + strings.Join(parts[1:], "//")
		name = js.TrimSpace(parts[0])
	}
	if name == "" {
		return false
	}
	t := l.tokVal("block", name)
	length := u16len(m[0]) - u16len(comment)
	for isWhitespaceUnit(js.UnitAt(l.input, length-1)) {
		length--
	}
	l.incrementColumn(length)
	t.Mode = mode
	l.push(t)
	l.consume(u16len(m[0]) - u16len(comment))
	l.incrementColumn(u16len(m[0]) - u16len(comment) - length)
	return true
}

var (
	mixinBlockRe = regexp.MustCompile(`^block`)
	yieldRe      = regexp.MustCompile(`^yield`)
	defaultRe    = regexp.MustCompile(`^default`)
	blockCodeRe  = regexp.MustCompile(`^-`)
	pathRe       = regexp.MustCompile(`^ ([^\n]+)`)
	nonSpaceRe   = regexp.MustCompile(`^[^ \n]+`)
)

func (l *Lexer) endOfLineToken(re *regexp.Regexp, typ string) bool {
	t, ok := l.scanEndOfLine(re, typ)
	if ok {
		l.push(t)
	}
	return ok
}

func (l *Lexer) include() (bool, error) {
	if l.keywordAt("include", true) {
		l.push(l.scanWord("include", "include"))
		for {
			ok, err := l.filter(true)
			if err != nil {
				return false, err
			}
			if !ok {
				break
			}
		}
		ok, err := l.path()
		if err != nil {
			return false, err
		}
		if !ok {
			if nonSpaceRe.MatchString(l.input) {
				return false, l.fail()
			}
			return false, l.error("NO_INCLUDE_PATH", "missing path for include")
		}
		return true, nil
	}
	if l.wordAt("include") {
		l.scanWord("include", "")
		return false, l.error("MALFORMED_INCLUDE", "malformed include")
	}
	return false, nil
}

func (l *Lexer) path() (bool, error) {
	t, ok := l.scanEndOfLine(pathRe, "path")
	if !ok {
		return false, nil
	}
	t.Val = js.TrimSpace(t.Val)
	if t.Val == "" {
		return false, nil
	}
	l.push(t)
	return true, nil
}

var (
	caseRe = regexp.MustCompile(`^case +([^\n]+)`)
	whenRe = regexp.MustCompile(`^when +([^:\n]+)`)
	restRe = regexp.MustCompile(`:([^:\n]+)`)
)

func (l *Lexer) caseToken() (bool, error) {
	if t, ok := l.scanEndOfLine(caseRe, "case"); ok {
		l.incrementColumn(-u16len(t.Val))
		if _, err := l.assertExpression(t.Val, false); err != nil {
			return false, err
		}
		l.incrementColumn(u16len(t.Val))
		l.push(t)
		return true, nil
	}
	if l.wordAt("case") {
		l.scanWord("case", "")
		return false, l.error("NO_CASE_EXPRESSION", "missing expression for case")
	}
	return false, nil
}

func (l *Lexer) when() (bool, error) {
	if t, ok := l.scanEndOfLine(whenRe, "when"); ok {
		st, err := charparser.Parse(t.Val)
		if err != nil {
			return false, parserError(err)
		}
		for st.IsNesting() || st.IsString() {
			rest := restRe.FindString(l.input)
			if rest == "" {
				break
			}
			t.Val += rest
			l.consume(u16len(rest))
			l.incrementColumn(u16len(rest))
			if st, err = charparser.Parse(t.Val); err != nil {
				return false, parserError(err)
			}
		}
		l.incrementColumn(-u16len(t.Val))
		if _, err := l.assertExpression(t.Val, false); err != nil {
			return false, err
		}
		l.incrementColumn(u16len(t.Val))
		l.push(t)
		return true, nil
	}
	if l.wordAt("when") {
		l.scanWord("when", "")
		return false, l.error("NO_WHEN_EXPRESSION", "missing expression for when")
	}
	return false, nil
}

func (l *Lexer) defaultToken() (bool, error) {
	if l.endOfLineToken(defaultRe, "default") {
		return true, nil
	}
	if l.wordAt("default") {
		l.scanWord("default", "")
		return false, l.error("DEFAULT_WITH_EXPRESSION", "default should not have an expression")
	}
	return false, nil
}

const jsSpace = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

var (
	callRe     = regexp.MustCompile(`^\+([` + jsSpace + `]*)(([-\w]+)|(#\{))`)
	argsOpenRe = regexp.MustCompile(`^ *\(`)
	attrLikeRe = regexp.MustCompile(`^[` + jsSpace + `]*[-\w]+ *=`)
	mixinRe    = regexp.MustCompile(`^mixin +([-\w]+)(?: *\(([^\n\r\x{2028}\x{2029}]*)\))? *`)
)

func (l *Lexer) call() (bool, error) {
	m := callRe.FindStringSubmatch(l.input)
	if m == nil {
		return false, nil
	}
	var t Token
	var increment int
	if m[3] != "" {
		increment = u16len(m[0])
		l.consumeText(m[0])
		t = l.tokVal("call", m[3])
	} else {
		match, err := l.bracketExpression(2 + u16len(m[1]))
		if err != nil {
			return false, err
		}
		increment = match.End + 1
		l.consume(increment)
		if _, err := l.assertExpression(match.Src, false); err != nil {
			return false, err
		}
		t = l.tokVal("call", "#{"+match.Src+"}")
	}
	l.incrementColumn(increment)
	if open := argsOpenRe.FindString(l.input); open != "" {
		r, err := l.bracketExpression(u16len(open) - 1)
		if err != nil {
			return false, err
		}
		if !attrLikeRe.MatchString(r.Src) {
			l.incrementColumn(1)
			l.consume(r.End + 1)
			t.Args, t.HasArgs = r.Src, true
			if _, err := l.assertExpression("["+t.Args+"]", false); err != nil {
				return false, err
			}
			for _, c := range js.ToUTF16(t.Args) {
				if c == '\n' {
					l.incrementLine(1)
				} else {
					l.incrementColumn(1)
				}
			}
			l.incrementColumn(1)
		}
	}
	l.push(t)
	return true, nil
}

func (l *Lexer) mixin() bool {
	m := mixinRe.FindStringSubmatchIndex(l.input)
	if m == nil {
		return false
	}
	whole := l.input[m[0]:m[1]]
	t := l.tokVal("mixin", l.input[m[2]:m[3]])
	if m[4] >= 0 && m[5] > m[4] {
		t.Args, t.HasArgs = l.input[m[4]:m[5]], true
	}
	l.consumeText(whole)
	l.incrementColumn(u16len(whole))
	l.push(t)
	return true
}

var conditionalRe = regexp.MustCompile(`^(if|unless|else if|else)\b([^\n]*)`)

func (l *Lexer) conditional() (bool, error) {
	m := conditionalRe.FindStringSubmatch(l.input)
	if m == nil {
		return false, nil
	}
	l.consumeText(m[0])
	typ := strings.ReplaceAll(m[1], " ", "-")
	code := js.TrimSpace(m[2])
	t := l.tokVal(typ, code)
	l.incrementColumn(u16len(m[0]) - u16len(code))
	switch typ {
	case "if", "else-if":
		if _, err := l.assertExpression(code, false); err != nil {
			return false, err
		}
	case "unless":
		if _, err := l.assertExpression(code, false); err != nil {
			return false, err
		}
		t.Val = "!(" + code + ")"
		t.Type = "if"
	case "else":
		if code != "" {
			return false, l.error("ELSE_CONDITION", "`else` cannot have a condition, perhaps you meant `else if`")
		}
	}
	l.incrementColumn(u16len(code))
	l.push(t)
	return true, nil
}

var whileRe = regexp.MustCompile(`^while +([^\n]+)`)

func (l *Lexer) while() (bool, error) {
	if m := whileRe.FindStringSubmatch(l.input); m != nil {
		l.consumeText(m[0])
		if _, err := l.assertExpression(m[1], false); err != nil {
			return false, err
		}
		t := l.tokVal("while", m[1])
		l.incrementColumn(u16len(m[0]))
		l.push(t)
		return true, nil
	}
	if l.wordAt("while") {
		l.scanWord("while", "")
		return false, l.error("NO_WHILE_EXPRESSION", "missing expression for while")
	}
	return false, nil
}

var (
	eachRe       = regexp.MustCompile(`^(?:each|for) +([a-zA-Z_$][\w$]*)(?: *, *([a-zA-Z_$][\w$]*))? * in *([^\n]+)`)
	dashEachRe   = regexp.MustCompile(`^- *(?:each|for) +([a-zA-Z_$][\w$]*)(?: *, *([a-zA-Z_$][\w$]*))? +in +([^\n]+)`)
	eachOfRe     = regexp.MustCompile(`^(?:each|for) ([^\n\r\x{2028}\x{2029}]*?) of *([^\n]+)`)
	dashEachOfRe = regexp.MustCompile(`^- *(?:each|for) +([a-zA-Z_$][\w$]*)(?: *, *([a-zA-Z_$][\w$]*))? +of +([^\n]+)`)
	identRe      = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)
	pairRe       = regexp.MustCompile(`^\[ *[a-zA-Z_$][\w$]* *\, *[a-zA-Z_$][\w$]* *\]$`)
)

func (l *Lexer) each() (bool, error) {
	if m := eachRe.FindStringSubmatchIndex(l.input); m != nil {
		whole := l.input[m[0]:m[1]]
		code := l.input[m[6]:m[7]]
		t := l.tokVal("each", l.input[m[2]:m[3]])
		if m[4] >= 0 && m[5] > m[4] {
			t.Key, t.HasKey = l.input[m[4]:m[5]], true
		}
		l.consumeText(whole)
		t.Loc.Start = Pos{l.lineno, l.colno}
		l.incrementColumn(u16len(whole) - u16len(code))
		if _, err := l.assertExpression(code, false); err != nil {
			return false, err
		}
		t.Code = code
		l.incrementColumn(u16len(code))
		l.push(t)
		return true, nil
	}
	name := "for"
	if l.wordAt("each") {
		name = "each"
	}
	for _, w := range []string{"each", "for"} {
		if l.wordAt(w) {
			l.scanWord(w, "")
			return false, l.error("MALFORMED_EACH", "This `"+name+"` has a syntax error. `"+name+
				"` statements should be of the form: `"+name+" VARIABLE_NAME of JS_EXPRESSION`")
		}
	}
	if dashEachRe.MatchString(l.input) {
		return false, l.error("MALFORMED_EACH",
			`Pug each and for should no longer be prefixed with a dash ("-"). They are pug keywords and not part of JavaScript.`)
	}
	return false, nil
}

func (l *Lexer) eachOf() (bool, error) {
	if m := eachOfRe.FindStringSubmatch(l.input); m != nil {
		l.consumeText(m[0])
		t := l.tokVal("eachOf", m[1])
		t.Value = m[1]
		l.incrementColumn(u16len(m[0]) - u16len(m[2]))
		if _, err := l.assertExpression(m[2], false); err != nil {
			return false, err
		}
		t.Code = m[2]
		l.incrementColumn(u16len(m[2]))
		l.push(t)
		v := js.TrimSpace(t.Value)
		if !identRe.MatchString(v) && !pairRe.MatchString(v) {
			return false, l.error("MALFORMED_EACH_OF_LVAL", "The value variable for each must either be a valid "+
				"identifier (e.g. `item`) or a pair of identifiers in square brackets (e.g. `[key, value]`).")
		}
		return true, nil
	}
	if dashEachOfRe.MatchString(l.input) {
		return false, l.error("MALFORMED_EACH",
			`Pug each and for should not be prefixed with a dash ("-"). They are pug keywords and not part of JavaScript.`)
	}
	return false, nil
}

var codeRe = regexp.MustCompile(`^(!?=|-)[ \t]*([^\n]+)`)

func (l *Lexer) code() (bool, error) {
	m := codeRe.FindStringSubmatch(l.input)
	if m == nil {
		return false, nil
	}
	flags, code := m[1], m[2]
	shortened := 0
	if l.interpolated {
		r, err := charparser.ParseUntil(code, "]", 0)
		if err != nil {
			ce := err.(*charparser.Error)
			l.incrementColumn(u16len(m[0]) - u16len(code) + ce.Index)
			if ce.Code == "CHARACTER_PARSER:END_OF_STRING_REACHED" {
				return false, l.error("NO_END_BRACKET", "End of line was reached with no closing bracket for interpolation.")
			}
			return false, l.error("BRACKET_MISMATCH", ce.Message)
		}
		shortened = u16len(code) - r.End
		code = r.Src
	}
	l.consume(u16len(m[0]) - shortened)
	t := l.tokVal("code", code)
	t.MustEscape = flags[0] == '='
	t.Buffer = flags[0] == '=' || (len(flags) > 1 && flags[1] == '=')
	l.incrementColumn(u16len(m[0]) - u16len(m[2]))
	if t.Buffer {
		if _, err := l.assertExpression(code, false); err != nil {
			return false, err
		}
	}
	l.tokens = append(l.tokens, t)
	l.incrementColumn(u16len(code))
	l.tokens[len(l.tokens)-1] = l.tokEnd(t)
	return true, nil
}

func (l *Lexer) blockCode() (bool, error) {
	t, ok := l.scanEndOfLine(blockCodeRe, "blockcode")
	if !ok {
		return false, nil
	}
	l.push(t)
	l.interpolationAllowed = false
	_, err := l.pipelessText(0)
	return true, err
}
