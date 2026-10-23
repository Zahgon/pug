// Package charparser tracks the bracket, string and comment state of JavaScript source one UTF-16 code unit at a
// time, as the character-parser package that pug's lexer uses does, so that pug can find where an expression
// embedded in a template ends.
package charparser

import "github.com/pugjs/pug/internal/js"

type kind int

const (
	lineComment kind = iota
	blockComment
	singleQuote
	doubleQuote
	templateQuote
	regexp
	roundBracket
	curlyBracket
	squareBracket
)

// State is the parse state after a sequence of code units.
type State struct {
	stack       []kind
	regexpStart bool
	escaped     bool
	hasDollar   bool
	history     []uint16 // code units seen outside comments, most recent last
	lastChar    uint16
}

// Error is a parse failure at a code unit offset.
type Error struct {
	Code    string
	Message string
	Index   int
}

// Error returns the message.
func (e *Error) Error() string {
	return e.Message
}

func (s *State) current() (kind, bool) {
	if len(s.stack) == 0 {
		return 0, false
	}
	return s.stack[len(s.stack)-1], true
}

func (s *State) pop() {
	s.stack = s.stack[:len(s.stack)-1]
}

// IsString reports whether the state is inside a string or template literal.
func (s *State) IsString() bool {
	c, ok := s.current()
	return ok && (c == singleQuote || c == doubleQuote || c == templateQuote)
}

// IsComment reports whether the state is inside a comment.
func (s *State) IsComment() bool {
	c, ok := s.current()
	return ok && (c == lineComment || c == blockComment)
}

// IsNesting reports whether anything is open: a bracket, a string, a comment or a regular expression.
func (s *State) IsNesting() bool {
	return len(s.stack) > 0
}

// Parse runs the parser over all of src from a fresh state.
func Parse(src string) (*State, error) {
	st := &State{}
	for i, c := range js.ToUTF16(src) {
		if err := st.ParseChar(c); err != nil {
			err.(*Error).Index = i
			return nil, err
		}
	}
	return st, nil
}

// Range is the part of a source before a delimiter, in code unit offsets.
type Range struct {
	Start, End int
	Src        string
}

// ParseUntil scans src from start until delimiter occurs outside any nesting.
func ParseUntil(src, delimiter string, start int) (Range, error) {
	u := js.ToUTF16(src)
	d := js.ToUTF16(delimiter)
	st := &State{}
	i := start
	for ; i < len(u); i++ {
		if !st.IsNesting() && hasPrefix(u[i:], d) {
			return Range{Start: start, End: i, Src: js.FromUTF16(u[start:i])}, nil
		}
		if err := st.ParseChar(u[i]); err != nil {
			err.(*Error).Index = i
			return Range{}, err
		}
	}
	return Range{}, &Error{Code: "CHARACTER_PARSER:END_OF_STRING_REACHED",
		Message: "The end of the string was reached with no closing bracket found.", Index: i}
}

func hasPrefix(u, p []uint16) bool {
	if len(p) > len(u) {
		return false
	}
	for i := range p {
		if u[i] != p[i] {
			return false
		}
	}
	return true
}

// ParseChar advances the state by one code unit.
func (s *State) ParseChar(c uint16) error {
	wasComment := s.IsComment()
	var lastChar uint16
	hasLast := len(s.history) > 0
	if hasLast {
		lastChar = s.history[len(s.history)-1]
	}
	if s.regexpStart {
		if c == '/' || c == '*' {
			s.pop()
		}
		s.regexpStart = false
	}
	cur, open := s.current()
	switch {
	case open && cur == lineComment:
		if c == '\n' {
			s.pop()
		}
	case open && cur == blockComment:
		if s.lastChar == '*' && c == '/' {
			s.pop()
		}
	case open && (cur == singleQuote || cur == doubleQuote):
		quote := uint16('\'')
		if cur == doubleQuote {
			quote = '"'
		}
		switch {
		case c == quote && !s.escaped:
			s.pop()
		case c == '\\' && !s.escaped:
			s.escaped = true
		default:
			s.escaped = false
		}
	case open && cur == templateQuote:
		switch {
		case c == '`' && !s.escaped:
			s.pop()
			s.hasDollar = false
		case c == '\\' && !s.escaped:
			s.escaped = true
			s.hasDollar = false
		case c == '$' && !s.escaped:
			s.hasDollar = true
		case c == '{' && s.hasDollar:
			s.stack = append(s.stack, curlyBracket)
		default:
			s.escaped = false
			s.hasDollar = false
		}
	case open && cur == regexp:
		switch {
		case c == '/' && !s.escaped:
			s.pop()
		case c == '\\' && !s.escaped:
			s.escaped = true
		default:
			s.escaped = false
		}
	default:
		switch {
		case c == '(':
			s.stack = append(s.stack, roundBracket)
		case c == '{':
			s.stack = append(s.stack, curlyBracket)
		case c == '[':
			s.stack = append(s.stack, squareBracket)
		case c == ')' || c == '}' || c == ']':
			want := map[uint16]kind{')': roundBracket, '}': curlyBracket, ']': squareBracket}[c]
			if !open || cur != want {
				return &Error{Code: "CHARACTER_PARSER:MISMATCHED_BRACKET", Message: "Mismatched Bracket: " + string(rune(c))}
			}
			s.pop()
		case hasLast && lastChar == '/' && c == '/':
			s.history = s.history[:len(s.history)-1]
			s.stack = append(s.stack, lineComment)
		case hasLast && lastChar == '/' && c == '*':
			s.history = s.history[:len(s.history)-1]
			s.stack = append(s.stack, blockComment)
		case c == '/' && isRegexp(s.history):
			s.stack = append(s.stack, regexp)
			s.regexpStart = true
		case c == '\'':
			s.stack = append(s.stack, singleQuote)
		case c == '"':
			s.stack = append(s.stack, doubleQuote)
		case c == '`':
			s.stack = append(s.stack, templateQuote)
		}
	}
	if !s.IsComment() && !wasComment {
		s.history = append(s.history, c)
	}
	s.lastChar = c
	return nil
}

// IsPunctuator reports whether c is one of the characters character-parser treats as JavaScript punctuation; the
// absent character (c < 0, the start of a string) counts as punctuation.
func IsPunctuator(c int) bool {
	if c < 0 {
		return true
	}
	switch c {
	case '.', '(', ')', ';', ',', '{', '}', '[', ']', ':', '?', '~', '%', '&', '*', '+', '-', '/', '<', '>', '^', '|',
		'!', '=':
		return true
	}
	return false
}

var keywords = map[string]bool{}

func init() {
	for _, k := range []string{"if", "in", "do", "var", "for", "new", "try", "let", "this", "else", "case", "void",
		"with", "enum", "while", "break", "catch", "throw", "const", "yield", "class", "super", "return", "typeof",
		"delete", "switch", "export", "import", "default", "finally", "extends", "function", "continue", "debugger",
		"package", "private", "interface", "instanceof", "implements", "protected", "public", "static"} {
		keywords[k] = true
	}
}

func isJSSpace(c uint16) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return c >= 0x2000 && c <= 0x200A
}

func isWordChar(c uint16) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isRegexp decides whether a slash after history starts a regular expression rather than a division.
func isRegexp(history []uint16) bool {
	i := len(history) - 1
	for i >= 0 && isJSSpace(history[i]) {
		i--
	}
	if i < 0 {
		// The start of the source counts as punctuation.
		return true
	}
	switch c := history[i]; {
	case c == ')':
		return false
	case c == '}':
		return true
	case IsPunctuator(int(c)):
		return true
	}
	// The word just before the slash, read backwards: a keyword (typeof /re/) means a regular expression.
	j := i
	for j >= 0 && isWordChar(history[j]) {
		j--
	}
	if j == i {
		return false
	}
	// \b after the word: the next character (further back) must not be a word character, which holds by
	// construction.
	word := make([]rune, 0, i-j)
	for k := j + 1; k <= i; k++ {
		word = append(word, rune(history[k]))
	}
	return keywords[string(word)]
}
