# pug-code-gen

Compiles a linked pug syntax tree into a template and renders it.

Go package `codegen`: `Compile(ast, Options)` returns a `*Template` whose `Render` method produces the HTML. It follows the original code generator: pretty printing, doctypes and self-closing tags, constant folding of attributes, and the JavaScript scoping of locals, `var` declarations, loops and mixins.

## License

  MIT
