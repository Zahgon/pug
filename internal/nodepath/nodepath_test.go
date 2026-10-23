package nodepath

import "testing"

func TestJoinDirnameExtname(t *testing.T) {
	for _, c := range [][3]string{{"a", "b/../c.pug", "a/c.pug"}, {".", "x.pug", "x.pug"}, {"/base", "/inc/a.pug", "/base/inc/a.pug"}, {"a/", "b/", "a/b/"}, {"", "", "."}} {
		if got := Join(c[0], c[1]); got != c[2] {
			t.Errorf("Join(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
	for in, want := range map[string]string{"a/b.pug": "a", "b.pug": ".", "/a": "/", "a/b/": "a", "//a": "//"} {
		if got := Dirname(in); got != want {
			t.Errorf("Dirname(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"a.pug": ".pug", "a": "", ".hidden": "", "a.b/c": "", "a.": ".", "x.tar.gz": ".gz"} {
		if got := Extname(in); got != want {
			t.Errorf("Extname(%q) = %q, want %q", in, got, want)
		}
	}
}
