package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func runIn(t *testing.T, files map[string]string, args ...string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRender(t *testing.T) {
	code, out, errOut := runIn(t, map[string]string{
		"page.pug":    "doctype html\nhtml\n  body\n    p.a(data-x={a: 1}) Hello #{name}\n    input(checked)\n",
		"locals.json": `{"name": "Wörld <&>"}`,
	}, "-O", "locals.json", "page.pug")
	want := `<!DOCTYPE html><html><body><p class="a" data-x="{&quot;a&quot;:1}">Hello Wörld &lt;&amp;&gt;</p><input checked></body></html>`
	if code != 0 || out != want || errOut != "" {
		t.Errorf("got %d %q %q", code, out, errOut)
	}
}

func TestPretty(t *testing.T) {
	code, out, _ := runIn(t, map[string]string{"page.pug": "div\n  p a\n  span b\n"}, "--indent", "\t", "page.pug")
	if code != 0 || out != "\n<div>\n\t<p>a</p><span>b</span>\n</div>" {
		t.Errorf("got %d %q", code, out)
	}
}

func TestPugError(t *testing.T) {
	code, out, errOut := runIn(t, map[string]string{"dup.pug": "div\n  p(foo=\"a\" foo=\"b\")\n"}, "dup.pug")
	want := "PUG:DUPLICATE_ATTRIBUTE\nError: dup.pug:2:13\n    1| div\n  > 2|   p(foo=\"a\" foo=\"b\")\n-------------------^\n    3| \n\nDuplicate attribute \"foo\" is not allowed.\n"
	if code != 1 || out != "" || errOut != want {
		t.Errorf("got %d %q %q", code, out, errOut)
	}
}

func TestRuntimeError(t *testing.T) {
	files := map[string]string{"rt.pug": "div\n  p hello\n  p= user.name.first\n"}
	code, _, errOut := runIn(t, files, "rt.pug")
	want := "TypeError: rt.pug:3\n    1| div\n    2|   p hello\n  > 3|   p= user.name.first\n    4| \n\nCannot read properties of undefined (reading 'name')\n"
	if code != 1 || errOut != want {
		t.Errorf("got %d %q", code, errOut)
	}
	code, _, errOut = runIn(t, files, "--no-debug", "rt.pug")
	if code != 1 || errOut != "TypeError: Cannot read properties of undefined (reading 'name')\n" {
		t.Errorf("--no-debug: got %d %q", code, errOut)
	}
}

func TestUsageErrors(t *testing.T) {
	files := map[string]string{"page.pug": "p= a\n", "bad.json": `{"a": 1,}`, "arr.json": "[1]"}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--nope", "page.pug"}, "pug: unknown flag --nope\n"},
		{[]string{"-O"}, "pug: flag -O needs a value\n"},
		{[]string{}, "pug: expected exactly one FILE argument\n"},
		{[]string{"-O", "missing.json", "page.pug"}, "pug: cannot read locals file missing.json\n"},
		{[]string{"-O", "bad.json", "page.pug"}, "pug: locals file bad.json is not valid JSON: Expected double-quoted property name in JSON at position 8 (line 1 column 9)\n"},
		{[]string{"-O", "arr.json", "page.pug"}, "pug: locals file arr.json must hold a JSON object\n"},
	} {
		code, out, errOut := runIn(t, files, c.args...)
		if code != 2 || out != "" || errOut != c.want {
			t.Errorf("%v: got %d %q %q", c.args, code, out, errOut)
		}
	}
}

func TestMissingTemplate(t *testing.T) {
	code, _, errOut := runIn(t, nil, "nope.pug")
	if code != 1 || errOut != "Error: ENOENT: no such file or directory, open 'nope.pug'\n" {
		t.Errorf("got %d %q", code, errOut)
	}
}

func TestFilters(t *testing.T) {
	code, out, _ := runIn(t, map[string]string{"page.pug": ":upper:wrap(tag=\"em\") straße\n:length\n  😀a\n"}, "page.pug")
	if code != 0 || out != "<EM>STRASSE</EM>3" {
		t.Errorf("got %d %q", code, out)
	}
}
