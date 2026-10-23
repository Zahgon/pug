# pug-filters

Applies filters at compile time.

Go package `filters`: `Handle(ast, filters)` replaces `:filter` blocks, nested filters and `include:filter` files by the output of the given filter functions; any other filter name is a `PUG:UNKNOWN_FILTER` error.

## License

  MIT
