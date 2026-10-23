package js

import (
	"strings"
	"unicode"
)

// ToUpperCase implements String.prototype.toUpperCase: the full Unicode case mapping without a locale.
func ToUpperCase(s string) string {
	if IsASCII(s) {
		return strings.ToUpper(s)
	}
	return mapCase(s, true)
}

// ToLowerCase implements String.prototype.toLowerCase, including the final form of capital sigma.
func ToLowerCase(s string) string {
	if IsASCII(s) {
		return strings.ToLower(s)
	}
	return mapCase(s, false)
}

func mapCase(s string, upper bool) string {
	u := ToUTF16(s)
	var out []uint16
	for i := 0; i < len(u); {
		r, n := decodeUnit(u, i)
		if r < 0 {
			// A lone surrogate maps to itself.
			out = append(out, u[i])
			i++
			continue
		}
		var mapped []rune
		switch {
		case upper:
			if m, ok := upperSpecial[r]; ok {
				mapped = m
			} else {
				mapped = []rune{unicode.ToUpper(r)}
			}
		case r == 0x03A3:
			if finalSigma(u, i, i+n) {
				mapped = []rune{0x03C2}
			} else {
				mapped = []rune{0x03C3}
			}
		default:
			if m, ok := lowerSpecial[r]; ok {
				mapped = m
			} else {
				mapped = []rune{unicode.ToLower(r)}
			}
		}
		for _, m := range mapped {
			out = appendRune(out, m)
		}
		i += n
	}
	return FromUTF16(out)
}

// decodeUnit decodes the code point at u[i] and its length in code units; a lone surrogate gives -1.
func decodeUnit(u []uint16, i int) (rune, int) {
	c := u[i]
	if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF {
		return (rune(c)-0xD800)<<10 + (rune(u[i+1]) - 0xDC00) + 0x10000, 2
	}
	if c >= 0xD800 && c <= 0xDFFF {
		return -1, 1
	}
	return rune(c), 1
}

// decodeUnitBefore decodes the code point that ends just before u[i] and its length.
func decodeUnitBefore(u []uint16, i int) (rune, int) {
	c := u[i-1]
	if c >= 0xDC00 && c <= 0xDFFF && i >= 2 && u[i-2] >= 0xD800 && u[i-2] <= 0xDBFF {
		return (rune(u[i-2])-0xD800)<<10 + (rune(c) - 0xDC00) + 0x10000, 2
	}
	if c >= 0xD800 && c <= 0xDFFF {
		return -1, 1
	}
	return rune(c), 1
}

func appendRune(out []uint16, r rune) []uint16 {
	if r >= 0x10000 {
		r -= 0x10000
		return append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
	}
	return append(out, uint16(r))
}

// isCased reports the Unicode Cased property.
func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r)
}

// isCaseIgnorable reports the Unicode Case_Ignorable property.
func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0x00A8, 0x00AD, 0x00AF, 0x00B4, 0x00B7, 0x00B8, 0x0387, 0x055F, 0x05F4, 0x2018,
		0x2019, 0x2024, 0x2027, 0xFE13, 0xFE52, 0xFE55, 0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// finalSigma reports whether the capital sigma at u[start:end] is in the Final_Sigma context: preceded by a cased
// letter and not followed by one, ignoring case-ignorable characters in between.
func finalSigma(u []uint16, start, end int) bool {
	before := false
	for i := start; i > 0; {
		r, n := decodeUnitBefore(u, i)
		i -= n
		if r >= 0 && isCaseIgnorable(r) {
			continue
		}
		before = r >= 0 && isCased(r)
		break
	}
	if !before {
		return false
	}
	for i := end; i < len(u); {
		r, n := decodeUnit(u, i)
		i += n
		if r >= 0 && isCaseIgnorable(r) {
			continue
		}
		return !(r >= 0 && isCased(r))
	}
	return true
}
