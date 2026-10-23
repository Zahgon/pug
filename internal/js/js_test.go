package js

import (
	"math"
	"testing"
)

func TestNumberToString(t *testing.T) {
	tenth, fifth := 0.1, 0.2
	for _, c := range []struct {
		in   float64
		want string
	}{
		{tenth + fifth, "0.30000000000000004"}, {1e21, "1e+21"}, {1e20, "100000000000000000000"}, {1.5e-7, "1.5e-7"},
		{0.000001, "0.000001"}, {1e-7, "1e-7"}, {math.Copysign(0, -1), "0"}, {-2147483648, "-2147483648"},
		{math.NaN(), "NaN"}, {math.Inf(-1), "-Infinity"}, {123456789012345680000, "123456789012345680000"},
	} {
		if got := NumberToString(c.in); got != c.want {
			t.Errorf("NumberToString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToFixed(t *testing.T) {
	for _, c := range []struct {
		x    float64
		d    int
		want string
	}{{1.005, 2, "1.00"}, {2.5, 0, "3"}, {1.5, 0, "2"}, {0.5, 0, "1"}, {1e21, 2, "1e+21"}, {-1.25, 1, "-1.3"}, {0, 2, "0.00"}} {
		if got := ToFixed(c.x, c.d); got != c.want {
			t.Errorf("ToFixed(%v, %d) = %q, want %q", c.x, c.d, got, c.want)
		}
	}
}

func TestStringToNumber(t *testing.T) {
	for in, want := range map[string]float64{"": 0, " 12 ": 12, "0x1f": 31, "0b101": 5, "1e3": 1000, ".5": 0.5, "Infinity": math.Inf(1)} {
		if got := StringToNumber(in); got != want {
			t.Errorf("StringToNumber(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"1_0", "-0x10", "abc", "1e", "0x"} {
		if got := StringToNumber(in); !math.IsNaN(got) {
			t.Errorf("StringToNumber(%q) = %v, want NaN", in, got)
		}
	}
}

func TestParseJSONErrors(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":1,}`:  `Expected double-quoted property name in JSON at position 7 (line 1 column 8)`,
		``:          `Unexpected end of JSON input`,
		`{a:1}`:     `Expected property name or '}' in JSON at position 1 (line 1 column 2)`,
		`[1,2`:      `Expected ',' or ']' after array element in JSON at position 4 (line 1 column 5)`,
		`"abc`:      `Unterminated string in JSON at position 4 (line 1 column 5)`,
		`tru`:       `Unexpected end of JSON input`,
		`{"a" 1}`:   `Expected ':' after property name in JSON at position 5 (line 1 column 6)`,
		`{"a":1} x`: `Unexpected non-whitespace character after JSON at position 8 (line 1 column 9)`,
		`é`:         `Unexpected token 'é', "é" is not valid JSON`,
		`01`:        `Unexpected number in JSON at position 1 (line 1 column 2)`,
		`-`:         `No number after minus sign in JSON at position 1 (line 1 column 2)`,
		`1.`:        `Unterminated fractional number in JSON at position 2 (line 1 column 3)`,
		`"\x"`:      `Bad escaped character in JSON at position 2 (line 1 column 3)`,
		`{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6, ]`: `Expected double-quoted property name in JSON at position 38 (line 1 column 39)`,
	} {
		_, err := ParseJSON(in)
		e, ok := err.(*Error)
		if !ok || e.Name != "SyntaxError" || e.Message != want {
			t.Errorf("ParseJSON(%q) error = %v, want SyntaxError: %s", in, err, want)
		}
	}
}

func TestObjectKeyOrder(t *testing.T) {
	v, err := ParseJSON(`{"b":1,"2":2,"a":3,"1":4,"01":5}`)
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := Stringify(v, Undefined)
	if err != nil || s != `{"1":4,"2":2,"b":1,"a":3,"01":5}` {
		t.Errorf("Stringify = %s, %v", s, err)
	}
}

func TestCaseMapping(t *testing.T) {
	if got := ToUpperCase("straße ŉ ﬁ"); got != "STRASSE ʼN FI" {
		t.Errorf("ToUpperCase = %q", got)
	}
	if got := ToLowerCase("ΣΊΣΥΦΟΣ ΣΑΣ İ"); got != "σίσυφος σας i̇" {
		t.Errorf("ToLowerCase = %q", got)
	}
}

func TestUTF16(t *testing.T) {
	s := "😀a"
	if Length(s) != 3 {
		t.Errorf("Length(%q) = %d", s, Length(s))
	}
	hi, lo := Slice(s, 0, 1), Slice(s, 1, 3)
	if Concat(hi, lo) != s {
		t.Errorf("surrogate halves do not rejoin")
	}
	if Output(hi) != "�" {
		t.Errorf("a lone surrogate must be written as U+FFFD")
	}
}

func TestCallSite(t *testing.T) {
	for src, want := range map[string]string{
		"a.b.c": "a.b.c", "a['b'].c": "a.b.c", "a.arr[0].trim": "a.arr[0].trim", "a.arr[a.i + 1].trim": "a.arr[(a.i + 1)].trim",
		"a.arr[1 + 1]": "a.arr[2]", "(a.s || a.n).x": "(a.s || a.n).x", "a.s.toUpperCase().x": "a.s.toUpperCase(...).x",
		"(5).trim": "5.trim", "'abc'.foo": `"abc".foo`, "{length: 2, 0: 'x'}": "{(intermediate value)(intermediate value)}",
		"(a.t ? a.n : a.s).trim": "(intermediate value)(intermediate value)(intermediate value).trim",
	} {
		n, err := ParseExpression(src)
		if err != nil {
			t.Fatal(err)
		}
		if got := CallSite(n); got != want {
			t.Errorf("CallSite(%s) = %q, want %q", src, got, want)
		}
	}
}

func TestEval(t *testing.T) {
	r := &testResolver{}
	sc := NewRoot(r)
	for src, want := range map[string]string{
		"[] + {}": "[object Object]", "[1, [2, 3], null] + ''": "1,2,3,", "'5' * '2'": "10", "null == undefined": "true",
		"'10' < '9'": "true", "typeof null": "object", "'ab'.replace('a', '[$&$$]')": "[a$]b", "' x '.trim().length": "1",
		"[3, 20, 100, undefined, 1].sort().join()": "1,100,20,3,", "Math.round(-2.5)": "-2", "parseInt('0x1f')": "31",
		"'a,b,,c'.split(',', 3).length": "3", "JSON.stringify({a: [1, null]}, null, 2)": "{\n  \"a\": [\n    1,\n    null\n  ]\n}",
	} {
		n, err := ParseExpression(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		v, err := sc.Eval(n)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := MustString(v); got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

type testResolver struct{}

func (testResolver) Free(name string) (*Binding, bool) {
	if v, ok := Global(name); ok {
		return &Binding{Value: v}, true
	}
	return &Binding{Value: Undefined}, true
}

func (testResolver) Define(string, Value) {}
