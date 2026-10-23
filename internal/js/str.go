package js

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// JavaScript strings are sequences of UTF-16 code units. This package holds them in Go strings encoded as WTF-8:
// UTF-8, except that a lone surrogate code unit is stored as the three-byte form of its code point. Valid UTF-8 text
// is therefore its own representation, and every JavaScript string, including one with unpaired surrogates, has
// exactly one encoding.

// ToUTF16 returns the UTF-16 code units of the JavaScript string s.
func ToUTF16(s string) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		if u, ok := surrogateAt(s, i); ok {
			out = append(out, u)
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r >= 0x10000 {
			a, b := utf16.EncodeRune(r)
			out = append(out, uint16(a), uint16(b))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// surrogateAt reports whether s holds the WTF-8 encoding of a lone surrogate at byte offset i.
func surrogateAt(s string, i int) (uint16, bool) {
	if i+2 < len(s) && s[i] == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2] >= 0x80 && s[i+2] <= 0xBF {
		return 0xD000 | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F), true
	}
	return 0, false
}

// FromUTF16 returns the JavaScript string made of the code units u.
func FromUTF16(u []uint16) string {
	var b strings.Builder
	b.Grow(len(u))
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c < 0x80:
			b.WriteByte(byte(c))
		case c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF:
			b.WriteRune(utf16.DecodeRune(rune(c), rune(u[i+1])))
			i++
		case c >= 0xD800 && c <= 0xDFFF:
			b.WriteByte(0xED)
			b.WriteByte(byte(0x80 | (c>>6)&0x3F))
			b.WriteByte(byte(0x80 | c&0x3F))
		default:
			b.WriteRune(rune(c))
		}
	}
	return b.String()
}

// Concat joins JavaScript strings; a lone high surrogate at the end of one and a lone low surrogate at the start of
// the next become one supplementary character.
func Concat(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		cur := b.String()
		if n := len(cur); n >= 3 && len(p) >= 3 {
			if hi, ok := surrogateAt(cur, n-3); ok && hi <= 0xDBFF {
				if lo, ok := surrogateAt(p, 0); ok && lo >= 0xDC00 {
					joined := cur[:n-3] + string(utf16.DecodeRune(rune(hi), rune(lo))) + p[3:]
					b.Reset()
					b.WriteString(joined)
					continue
				}
			}
		}
		b.WriteString(p)
	}
	return b.String()
}

// Length returns the number of UTF-16 code units of s.
func Length(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if _, ok := surrogateAt(s, i); ok {
			n++
			i += 3
			continue
		}
		if s[i] < 0x80 {
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// IsASCII reports whether s holds only ASCII characters, so that code unit offsets equal byte offsets.
func IsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Output returns s as UTF-8 bytes the way Node writes a string: each lone surrogate becomes U+FFFD.
func Output(s string) string {
	if !strings.Contains(s, "\xED") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if _, ok := surrogateAt(s, i); ok {
			b.WriteString("�")
			i += 3
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// DecodeUTF8 converts bytes read from a file to a JavaScript string the way Node's utf8 decoder does: each maximal
// invalid subsequence becomes U+FFFD.
func DecodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	var b strings.Builder
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r != utf8.RuneError || size > 1 {
			b.WriteRune(r)
			i += size
			continue
		}
		b.WriteString("�")
		i += invalidLength(data[i:])
	}
	return b.String()
}

// invalidLength returns the length of the maximal subpart of an ill-formed UTF-8 sequence at the start of p (at
// least one byte), as the WHATWG decoder replaces it.
func invalidLength(p []byte) int {
	c := p[0]
	var need int
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		need, lo = 2, 0xA0
	case c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF:
		need = 2
	case c == 0xED:
		need, hi = 2, 0x9F
	case c == 0xF0:
		need, lo = 3, 0x90
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	case c == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for k := 0; k < need && n < len(p); k++ {
		if p[n] < lo || p[n] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
		n++
	}
	return n
}

// isJSWhitespace reports whether the code unit c is WhiteSpace or a LineTerminator in ECMAScript.
func isJSWhitespace(c uint16) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return c >= 0x2000 && c <= 0x200A
}

// trimUnits trims ECMAScript whitespace from the start and/or the end of u.
func trimUnits(u []uint16, start, end bool) []uint16 {
	i, j := 0, len(u)
	if start {
		for i < j && isJSWhitespace(u[i]) {
			i++
		}
	}
	if end {
		for j > i && isJSWhitespace(u[j-1]) {
			j--
		}
	}
	return u[i:j]
}

// TrimSpace trims ECMAScript whitespace from both ends of s.
func TrimSpace(s string) string {
	return FromUTF16(trimUnits(ToUTF16(s), true, true))
}

// unitsIndex returns the index of sub in u at or after from, or -1.
func unitsIndex(u, sub []uint16, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i+len(sub) <= len(u); i++ {
		if unitsHasPrefix(u[i:], sub) {
			return i
		}
	}
	return -1
}

func unitsHasPrefix(u, prefix []uint16) bool {
	if len(prefix) > len(u) {
		return false
	}
	for i, c := range prefix {
		if u[i] != c {
			return false
		}
	}
	return true
}

// CompareStrings orders two JavaScript strings by their UTF-16 code units.
func CompareStrings(a, b string) int {
	if IsASCII(a) && IsASCII(b) {
		return strings.Compare(a, b)
	}
	x, y := ToUTF16(a), ToUTF16(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(x) < len(y):
		return -1
	case len(x) > len(y):
		return 1
	}
	return 0
}

// Slice returns the code units from..to of s (offsets in UTF-16 code units, clamped like String.prototype.substring
// with from <= to).
func Slice(s string, from, to int) string {
	if IsASCII(s) {
		from, to = clampRange(from, to, len(s))
		return s[from:to]
	}
	u := ToUTF16(s)
	from, to = clampRange(from, to, len(u))
	return FromUTF16(u[from:to])
}

func clampRange(from, to, n int) (int, int) {
	if from < 0 {
		from = 0
	}
	if to > n {
		to = n
	}
	if from > to {
		from = to
	}
	return from, to
}

// From returns s without its first n code units (String.prototype.substr(n)).
func From(s string, n int) string {
	return Slice(s, n, 1<<30)
}

// UnitAt returns the code unit at offset i of s, or -1 past its end.
func UnitAt(s string, i int) int {
	if IsASCII(s) {
		if i < 0 || i >= len(s) {
			return -1
		}
		return int(s[i])
	}
	u := ToUTF16(s)
	if i < 0 || i >= len(u) {
		return -1
	}
	return int(u[i])
}

// IndexOf returns the offset in code units of the first sub in s at or after from, or -1.
func IndexOf(s, sub string, from int) int {
	if IsASCII(s) && IsASCII(sub) {
		if from > len(s) {
			from = len(s)
		}
		if from < 0 {
			from = 0
		}
		i := strings.Index(s[from:], sub)
		if i < 0 {
			return -1
		}
		return i + from
	}
	return unitsIndex(ToUTF16(s), ToUTF16(sub), from)
}
