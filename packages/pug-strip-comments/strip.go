// Copyright (c) 2021 Forbes Lindesay (pugjs/pug, pug-strip-comments); MIT licence, see LICENSE.

// Package stripcomments removes the tokens of unbuffered comments from a token list, as pug-strip-comments does
// with its default options.
package stripcomments

import (
	pugerror "github.com/pugjs/pug/packages/pug-error"
	lexer "github.com/pugjs/pug/packages/pug-lexer"
)

func unexpected(typ, occasion, filename string, line int) error {
	return pugerror.New("UNEXPECTED_TOKEN", "`"+typ+"` encountered when "+occasion,
		pugerror.Options{Filename: filename, Line: line})
}

// Strip drops unbuffered comments (//-) and their text blocks.
func Strip(tokens []lexer.Token, filename string) ([]lexer.Token, error) {
	var out []lexer.Token
	inComment, inPipelessText := false, false
	for _, t := range tokens {
		keep := false
		switch t.Type {
		case "comment":
			if inComment {
				return nil, unexpected("comment", "already in a comment", filename, 0)
			}
			inComment = !t.Buffer
			keep = !inComment
		case "start-pipeless-text":
			if !inComment {
				keep = true
				break
			}
			if inPipelessText {
				return nil, unexpected("start-pipeless-text", "already in pipeless text mode", filename, 0)
			}
			inPipelessText = true
		case "end-pipeless-text":
			if !inComment {
				keep = true
				break
			}
			if !inPipelessText {
				return nil, unexpected("end-pipeless-text", "not in pipeless text mode", filename, 0)
			}
			inPipelessText = false
			inComment = false
		case "text":
			keep = !inComment
		default:
			if inPipelessText {
				break
			}
			inComment = false
			keep = true
		}
		if keep {
			out = append(out, t)
		}
	}
	return out, nil
}
