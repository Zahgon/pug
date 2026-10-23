// Package nodepath implements the POSIX functions of Node's path module that pug uses (join, dirname, extname)
// and the messages of the file system errors Node reports, so that resolved file names and error texts match.
package nodepath

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"

	"github.com/pugjs/pug/internal/js"
)

// Normalize implements path.normalize: it resolves . and .. segments and repeated slashes, keeps a leading slash
// and a trailing slash, and gives "." for an empty result.
func Normalize(p string) string {
	if p == "" {
		return "."
	}
	absolute := strings.HasPrefix(p, "/")
	trailing := strings.HasSuffix(p, "/")
	var out []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
		case "..":
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
			} else if !absolute {
				out = append(out, "..")
			}
		default:
			out = append(out, seg)
		}
	}
	res := strings.Join(out, "/")
	if res == "" && !absolute {
		res = "."
	}
	if res != "" && trailing {
		res += "/"
	}
	if absolute {
		return "/" + res
	}
	return res
}

// Join implements path.join.
func Join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return "."
	}
	return Normalize(strings.Join(kept, "/"))
}

// Dirname implements path.dirname.
func Dirname(p string) string {
	if p == "" {
		return "."
	}
	hasRoot := p[0] == '/'
	end := -1
	matchedSlash := true
	for i := len(p) - 1; i >= 1; i-- {
		if p[i] == '/' {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return p[:end]
}

// Extname implements path.extname.
func Extname(p string) string {
	startDot, startPart, end := -1, 0, -1
	matchedSlash := true
	preDotState := 0
	for i := len(p) - 1; i >= 0; i-- {
		c := p[i]
		if c == '/' {
			if !matchedSlash {
				startPart = i + 1
				break
			}
			continue
		}
		if end == -1 {
			matchedSlash = false
			end = i + 1
		}
		if c == '.' {
			if startDot == -1 {
				startDot = i
			} else if preDotState != 1 {
				preDotState = 1
			}
		} else if startDot != -1 {
			preDotState = -1
		}
	}
	if startDot == -1 || end == -1 || preDotState == 0 ||
		(preDotState == 1 && startDot == end-1 && startDot == startPart+1) {
		return ""
	}
	return p[startDot:end]
}

// ReadFile reads a file as fs.readFileSync does; a failure is a JavaScript Error with Node's message.
func ReadFile(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err == nil {
		return data, nil
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return nil, &js.Error{Name: "Error", Message: "ENOENT: no such file or directory, open '" + name + "'"}
		case syscall.EISDIR:
			return nil, &js.Error{Name: "Error", Message: "EISDIR: illegal operation on a directory, read"}
		case syscall.ENOTDIR:
			return nil, &js.Error{Name: "Error", Message: "ENOTDIR: not a directory, open '" + name + "'"}
		case syscall.EACCES:
			return nil, &js.Error{Name: "Error", Message: "EACCES: permission denied, open '" + name + "'"}
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &js.Error{Name: "Error", Message: "ENOENT: no such file or directory, open '" + name + "'"}
	}
	return nil, &js.Error{Name: "Error", Message: err.Error()}
}
