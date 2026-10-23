// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-runtime); MIT licence, see LICENSE.

package runtime

import (
	"testing"

	"github.com/pugjs/pug/internal/js"
)

func obj(kv ...any) *js.Object {
	o := js.NewObject()
	for i := 0; i < len(kv); i += 2 {
		o.Set(kv[i].(string), kv[i+1])
	}
	return o
}

func TestAttr(t *testing.T) {
	for _, c := range []struct {
		val            js.Value
		escaped, terse bool
		want           string
	}{
		{true, true, true, " key"}, {true, false, false, ` key="key"`}, {false, true, true, ""}, {js.Null, true, false, ""},
		{js.Undefined, false, false, ""}, {obj("foo", "bar"), true, true, ` key="{&quot;foo&quot;:&quot;bar&quot;}"`},
		{obj("foo", "bar"), false, true, ` key='{"foo":"bar"}'`}, {obj("foo", "don't"), false, true, ` key='{"foo":"don&#39;t"}'`},
		{500.0, true, true, ` key="500"`}, {"foo>bar", true, true, ` key="foo&gt;bar"`}, {"foo>bar", false, true, ` key="foo>bar"`},
	} {
		got, err := Attr("key", c.val, c.escaped, c.terse)
		if err != nil || got != c.want {
			t.Errorf("Attr(%v, %v, %v) = %q, %v; want %q", c.val, c.escaped, c.terse, got, err, c.want)
		}
	}
}

func TestAttrs(t *testing.T) {
	for _, c := range []struct {
		in   *js.Object
		want string
	}{
		{obj("foo", "bar", "hoo", "boo"), ` foo="bar" hoo="boo"`}, {obj("foo", ""), ` foo=""`}, {obj("class", ""), ""},
		{obj("foo", "bar", "class", js.NewArray("foo", obj("bar", true))), ` class="foo bar" foo="bar"`},
		{obj("style", obj("foo", "bar", "baz", "bash")), ` style="foo:bar;baz:bash;"`},
	} {
		got, err := Attrs(c.in, true)
		if err != nil || got != c.want {
			t.Errorf("Attrs = %q, %v; want %q", got, err, c.want)
		}
	}
}

func TestClassesAndEscape(t *testing.T) {
	v, _ := Classes(js.NewArray("<a>", js.NewArray("b", obj("c", true, "d", false)), js.Null), []bool{true})
	if v != "&lt;a&gt; b c" {
		t.Errorf("Classes = %v", v)
	}
	if e, _ := Escape(5.0); e != 5.0 {
		t.Errorf("Escape(5) must return the number unchanged, got %v", e)
	}
	if e, _ := Escape(js.NewArray("<")); e != "&lt;" {
		t.Errorf("Escape([\"<\"]) = %v", e)
	}
}

func TestMerge(t *testing.T) {
	merged, err := Merge([]js.Value{obj("class", "a", "style", "x:1"), obj("class", js.NewArray("b", "c"), "style", "y:2", "id", "i")})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := Attrs(merged, true)
	if got != ` class="a b c" style="x:1;y:2;" id="i"` {
		t.Errorf("merged attrs = %q", got)
	}
}
