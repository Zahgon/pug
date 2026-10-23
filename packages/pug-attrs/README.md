# pug-attrs

Compiles the attributes of tags and mixin calls.

Attribute compilation is part of the Go package `codegen` in `packages/pug-code-gen` (`compile.go`): class lists, styles and constant attribute values are folded at compile time as the original pug-attrs did, and the rest is rendered by `packages/pug-runtime`.

## License

  MIT
