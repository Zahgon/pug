package js

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// NumberToString implements Number::toString for radix 10: the shortest digits that round-trip, in fixed notation
// for exponents from -7 to 20 and in exponential notation otherwise.
func NumberToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case x == 0:
		return "0"
	case x < 0:
		return "-" + NumberToString(-x)
	case math.IsInf(x, 1):
		return "Infinity"
	}
	digits, n := shortestDigits(x)
	k := len(digits)
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	e := n - 1
	sign := "+"
	if e < 0 {
		sign = "-"
		e = -e
	}
	if k == 1 {
		return digits + "e" + sign + strconv.Itoa(e)
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + strconv.Itoa(e)
}

// shortestDigits returns the shortest round-trip decimal digits of the positive finite x and the position n of the
// decimal point: x = 0.digits × 10^n.
func shortestDigits(x float64) (string, int) {
	s := strconv.FormatFloat(x, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(exp)
	return strings.Replace(mant, ".", "", 1), e + 1
}

// StringToNumber implements ToNumber applied to a string.
func StringToNumber(s string) float64 {
	t := TrimSpace(s)
	if t == "" {
		return 0
	}
	switch t {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(t) > 2 && t[0] == '0' {
		base := 0
		switch t[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			return parseDigits(t[2:], base, true)
		}
	}
	if !isDecimalLiteral(t) {
		return math.NaN()
	}
	f, _ := strconv.ParseFloat(t, 64)
	return f
}

// isDecimalLiteral reports whether s is a StrDecimalLiteral without Infinity: an optional sign, digits with an
// optional fraction or a fraction alone, and an optional exponent.
func isDecimalLiteral(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	intDigits := 0
	for i < len(s) && isDigit(s[i]) {
		i++
		intDigits++
	}
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
			fracDigits++
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(s) && isDigit(s[i]) {
			i++
			expDigits++
		}
		if expDigits == 0 {
			return false
		}
	}
	return i == len(s)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// digitValue returns the value of c as a digit of base 36, or 36.
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 36
}

// parseDigits converts digits of base to a number, rounding to nearest. With whole set, every character must be a
// digit (else NaN); otherwise the longest prefix of digits counts and no digit at all is NaN.
func parseDigits(s string, base int, whole bool) float64 {
	n := 0
	for n < len(s) && digitValue(s[n]) < base {
		n++
	}
	if n == 0 || (whole && n != len(s)) {
		return math.NaN()
	}
	s = s[:n]
	if base == 10 {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	v := new(big.Int)
	b := big.NewInt(int64(base))
	for i := 0; i < len(s); i++ {
		v.Mul(v, b)
		v.Add(v, big.NewInt(int64(digitValue(s[i]))))
	}
	f, _ := new(big.Float).SetInt(v).Float64()
	return f
}

// ParseInt implements the global parseInt.
func ParseInt(s string, radix Value) float64 {
	u := trimUnits(ToUTF16(s), true, false)
	t := FromUTF16(u)
	sign := 1.0
	if t != "" && (t[0] == '+' || t[0] == '-') {
		if t[0] == '-' {
			sign = -1
		}
		t = t[1:]
	}
	r := ToInt32(ToNumber(radix))
	strip := true
	if r != 0 {
		if r < 2 || r > 36 {
			return math.NaN()
		}
		if r != 16 {
			strip = false
		}
	} else {
		r = 10
	}
	if strip && len(t) >= 2 && t[0] == '0' && (t[1] == 'x' || t[1] == 'X') {
		t = t[2:]
		r = 16
	}
	return sign * parseDigits(t, int(r), false)
}

// ParseFloat implements the global parseFloat.
func ParseFloat(s string) float64 {
	t := FromUTF16(trimUnits(ToUTF16(s), true, false))
	rest := t
	sign := ""
	if rest != "" && (rest[0] == '+' || rest[0] == '-') {
		sign = rest[:1]
		rest = rest[1:]
	}
	if strings.HasPrefix(rest, "Infinity") {
		if sign == "-" {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	end := 0
	for end < len(rest) && isDigit(rest[end]) {
		end++
	}
	intDigits := end
	fracDigits := 0
	if end < len(rest) && rest[end] == '.' {
		k := end + 1
		for k < len(rest) && isDigit(rest[k]) {
			k++
			fracDigits++
		}
		if intDigits > 0 || fracDigits > 0 {
			end = k
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return math.NaN()
	}
	if end < len(rest) && (rest[end] == 'e' || rest[end] == 'E') {
		k := end + 1
		if k < len(rest) && (rest[k] == '+' || rest[k] == '-') {
			k++
		}
		d := k
		for k < len(rest) && isDigit(rest[k]) {
			k++
		}
		if k > d {
			end = k
		}
	}
	f, _ := strconv.ParseFloat(sign+rest[:end], 64)
	return f
}

// ToIntegerOrInfinity implements the abstract operation of that name.
func ToIntegerOrInfinity(v Value) float64 {
	n := ToNumber(v)
	if math.IsNaN(n) || n == 0 {
		return 0
	}
	return math.Trunc(n)
}

// ToInt32 implements the abstract operation of that name on a number.
func ToInt32(n float64) int32 {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	n = math.Trunc(n)
	m := math.Mod(n, 4294967296)
	if m < 0 {
		m += 4294967296
	}
	return int32(uint32(m))
}

// ToFixed implements Number.prototype.toFixed for a digits count already checked to be in 0..100.
func ToFixed(x float64, digits int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.Abs(x) >= 1e21 || math.IsInf(x, 0) {
		return NumberToString(x)
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	// n = round(x × 10^digits), ties to the larger n, computed on the exact binary value of x.
	r := new(big.Rat).SetFloat64(x)
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)))
	q, m := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	m.Mul(m, big.NewInt(2))
	if m.Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	s := q.String()
	if digits == 0 {
		return sign + s
	}
	if len(s) <= digits {
		s = strings.Repeat("0", digits-len(s)+1) + s
	}
	return sign + s[:len(s)-digits] + "." + s[len(s)-digits:]
}
