// Copyright (c) 2009-2014 TJ Holowaychuk (pugjs/pug); MIT licence, see LICENSE.

package pug

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/pugjs/pug/internal/js"
)

var snapRe = regexp.MustCompile("(?s)exports\\[`([^`]+)`\\] = `\\n?\"(.*?)\"\\n?`;")

// snapshot returns the recorded output named name in a jest snapshot file.
func snapshot(t *testing.T, file, name string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range snapRe.FindAllStringSubmatch(string(data), -1) {
		if m[1] == name {
			return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(m[2])
		}
	}
	t.Fatalf("snapshot %q not found in %s", name, file)
	return ""
}

func render(t *testing.T, file string, o Options, locals *js.Object) string {
	t.Helper()
	tmpl, err := CompileFile(file, o)
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	out, err := tmpl.Render(locals)
	if err != nil {
		t.Fatalf("render %s: %v", file, err)
	}
	return out
}

func compileError(t *testing.T, file, want string) {
	t.Helper()
	_, err := CompileFile(file, Options{})
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: error %v, want one containing %q", file, err, want)
	}
}

func TestExtendsNotTopLevel(t *testing.T) {
	const want = `Declaration of template inheritance ("extends") should be the first thing in the file. There can only be one extends statement per file.`
	compileError(t, "test/extends-not-top-level/index.pug", want)
	compileError(t, "test/extends-not-top-level/duplicate.pug", want)
}

func TestEachOf(t *testing.T) {
	const want = "The value variable for each must either be a valid identifier (e.g. `item`) or a pair of identifiers in square brackets (e.g. `[key, value]`)."
	for _, f := range []string{"left-side", "right-side", "one-val", "no-brackets"} {
		compileError(t, "test/eachOf/error/"+f+".pug", want)
	}
	// The upstream suite iterates a Map; an array of pairs iterates the same way.
	users := js.NewObject()
	users.Set("users", js.NewArray(js.NewArray("a", "b"), js.NewArray("foo", "bar")))
	snap := "test/eachOf/__snapshots__/index.test.js.snap"
	if got, want := render(t, "test/eachOf/passing/brackets.pug", Options{}, users), snapshot(t, snap, "Proper Usage Brackets 1"); got != want {
		t.Errorf("brackets: got %q, want %q", got, want)
	}
	if got, want := render(t, "test/eachOf/passing/no-brackets.pug", Options{}, users), snapshot(t, snap, "Proper Usage No Brackets 1"); got != want {
		t.Errorf("no brackets: got %q, want %q", got, want)
	}
}

func ajax(on bool) *js.Object {
	o := js.NewObject()
	o.Set("ajax", on)
	return o
}

func TestDuplicateAndShadowedBlocks(t *testing.T) {
	for _, dir := range []string{"duplicate-block", "shadowed-block"} {
		snap := "test/" + dir + "/__snapshots__/index.test.js.snap"
		name := "layout with duplicate block"
		if dir == "shadowed-block" {
			name = "layout with shadowed block"
		}
		for i, on := range []bool{true, false} {
			got := render(t, "test/"+dir+"/index.pug", Options{}, ajax(on))
			if want := snapshot(t, snap, name+" "+string(rune('1'+i))); got != want {
				t.Errorf("%s ajax=%v: got %q, want %q", dir, on, got, want)
			}
		}
	}
}

func TestRegression2436(t *testing.T) {
	snap := "test/regression-2436/__snapshots__/index.test.js.snap"
	for file, name := range map[string]string{
		"issue1.pug": "#2436 - block with a same name extends from the same layout in nesting 1",
		"issue2.pug": "#2436 - block with a same name extends from different layout in nesting 1",
	} {
		got := render(t, "test/regression-2436/"+file, Options{Pretty: "  ", PrettySet: true}, js.NewObject())
		if want := snapshot(t, snap, name); got != want {
			t.Errorf("%s: got %q, want %q", file, got, want)
		}
	}
}
