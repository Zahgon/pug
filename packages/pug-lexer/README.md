# pug-lexer

The pug lexer.

Go package `lexer`: `Lex(src, Options)` returns the tokens of a template with their positions (columns count UTF-16 code units, as in JavaScript). `go test` compares the tokens of `test/cases` and the errors of `test/errors` with the recorded snapshots.

## License

  MIT
