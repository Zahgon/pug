<a href="https://pugjs.org"><img src="https://cdn.rawgit.com/pugjs/pug-logo/eec436cee8fd9d1726d7839cbe99d1f694692c0c/SVG/pug-final-logo-_-colour-128.svg" height="200" align="right"></a>

# Pug

Full documentation of the template language is at [pugjs.org](https://pugjs.org/)

Pug is a high-performance template engine heavily influenced by [Haml](http://haml.info/). This repository
implements it in Go: a module with the compiler packages and a `pug` command that renders template files with JSON
locals. For bug reports, feature requests and questions, [open an issue](https://github.com/pugjs/pug/issues/new).

## Packages

The module `github.com/pugjs/pug` keeps the structure of the original packages; each directory keeps its licence.

Directory | Go package | Role
----------|------------|-----
`packages/pug` | `pug` | compiles templates: `Compile`, `CompileFile`, `Template.Render`
`packages/pug-lexer` | `lexer` | tokens, with line and column positions
`packages/pug-parser` | `parser` | the syntax tree
`packages/pug-load` | `load` | reads included and extended files
`packages/pug-strip-comments` | `stripcomments` | drops unbuffered comments
`packages/pug-filters` | `filters` | applies filters at compile time
`packages/pug-linker` | `linker` | inheritance, blocks and includes
`packages/pug-code-gen` | `codegen` | compiles and renders, including attribute compilation (formerly pug-attrs)
`packages/pug-runtime` | `runtime` | escaping, attributes, classes, styles, merging and error context
`packages/pug-walk` | `walk` | tree traversal
`packages/pug-error` | `pugerror` | errors with source context
`internal/js` | `js` | the JavaScript that templates evaluate: values, operators, built-in methods and JSON
`cmd/pug` | `main` | the `pug` command

## Rename from "Jade"

This project was formerly known as "Jade". However, it was revealed to us that "Jade" is a registered trademark; as a result, a rename was needed. After some discussion among the maintainers, **"Pug"** was chosen as the new name for this project. As of version 2, "pug" is the official package name.

If your package or app currently uses `jade`, don't worry: we have secured permissions to continue to occupy that package name, although all new versions will be released under `pug`.

Before the renaming, work had already begun on “Jade 2.0.0”. Therefore, the rename to Pug coincided with the major version bump. As a result, upgrading from Jade to Pug will be the same process as upgrading any other package with a major version bump.

The syntax of Pug has several differences, deprecations, and removals compared to its predecessor. These differences are documented in [#2305](https://github.com/pugjs/pug/issues/2305).

The website and documentation for Pug are still being updated. But if you are new to Pug, you should get started with the new syntax.

## Installation

Pug needs Go 1.23 or later and nothing beyond the standard library:

    go build ./cmd/pug

## Command Line

    pug [flags] FILE

Flag | Meaning
-----|--------
`-O, --obj FILE` | read the locals from a JSON file holding an object
`-P, --pretty` | indent the output with two spaces
`--indent STRING` | indent the output with STRING (whitespace only)
`-D, --doctype NAME` | the default doctype
`-b, --basedir DIR` | the root of absolute `include` and `extends` paths
`--no-debug` | compile without debug information: runtime errors carry no template context
`--self` | read the locals through the variable `self`

The HTML goes to standard output. Template errors are written to standard error with their `PUG:` code, the
file, line and column and the surrounding source lines (exit status 1); usage errors exit with status 2. The
filters `cdata`, `upper`, `lower`, `wrap` and `length` are available to templates.

JavaScript in templates (code, interpolation, attribute values, conditions, loops and mixin arguments) is evaluated
by the `js` package, which implements the expressions, statements and built-in methods that templates use, with the
coercions, number formatting and error messages of JavaScript.

## Syntax

Pug is a clean, whitespace sensitive syntax for writing HTML. Here is a simple example:

```pug
doctype html
html(lang="en")
  head
    title= pageTitle
    script(type='text/javascript').
      if (foo) bar(1 + 5);
  body
    h1 Pug - node template engine
    #container.col
      if youAreUsingPug
        p You are amazing
      else
        p Get on it!
      p.
        Pug is a terse and simple templating language with a
        strong focus on performance and powerful features.
```

Pug transforms the above to:

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <title>Pug</title>
    <script type="text/javascript">
      if (foo) bar(1 + 5);
    </script>
  </head>
  <body>
    <h1>Pug - node template engine</h1>
    <div id="container" class="col">
      <p>You are amazing</p>
      <p>
        Pug is a terse and simple templating language with a strong focus on
        performance and powerful features.
      </p>
    </div>
  </body>
</html>
```

## API

```go
tmpl, err := pug.CompileFile("views/page.pug", pug.Options{Pretty: "  ", PrettySet: true, Basedir: "views"})
if err != nil {
	return err
}
html, err := tmpl.RenderJSON(`{"name": "Timothy"}`)
```

## Development

    go vet ./...
    go test ./...

The tests render the fixtures under `packages/pug/test` (cases, anti-cases and regressions) and compare the lexer
with the recorded token snapshots of `packages/pug-lexer/test`.

## Tutorials

- cssdeck interactive [Pug syntax tutorial](http://cssdeck.com/labs/learning-the-jade-templating-engine-syntax)
- cssdeck interactive [Pug logic tutorial](http://cssdeck.com/labs/jade-templating-tutorial-codecast-part-2)
- [Pug について。](https://gist.github.com/japboy/5402844) (A Japanese Tutorial)

## Implementations in other languages

### Ports in other languages

Ports to other languages, with very close syntax:

- [PHP](https://github.com/pug-php/pug)
- [Java](https://github.com/neuland/jade4j)
- [Python](https://github.com/kakulukia/pypugjs)
- [Ruby](https://github.com/yivo/pug-ruby)
- [C# (ASP.NET Core)](https://github.com/AspNetMonsters/pugzor)
- [RPG/ILE](https://github.com/WorksOfLiam/apug)

### Equivalents in other languages

Templates engines for other languages with a different syntax, but a similar philosophy:

- [Scaml for Scala](https://scalate.github.io/scalate/documentation/scaml-reference.html)
- [Slim for Ruby](https://github.com/slim-template/slim) (should not be confused with Slim PHP framework)
- [HAML for Ruby](http://haml.info)

### Framework implementations/adapters

Embedded view engines for frameworks:

- [Laravel](https://github.com/BKWLD/laravel-pug)
- [Symfony](https://github.com/pug-php/pug-symfony)
- [Phalcon](https://github.com/pug-php/pug-phalcon)
- [CodeIgniter](https://github.com/pug-php/ci-pug-engine)
- [Yii 2](https://github.com/pug-php/pug-yii2)
- [Slim 3](https://github.com/pug-php/pug-slim)
- [Silex (implementation example)](https://gist.github.com/kylekatarnls/ba13e4361ab14f4ff5d2a5775eb0cc10)
- [Lumen](https://github.com/BKWLD/laravel-pug#use-in-lumen)
- [Rails](https://github.com/yivo/pug-rails)

### CMS plugins

- [WordPress](https://github.com/welaika/wordless)

## Additional Resources

- [Emacs Mode](https://github.com/brianc/jade-mode)
- [Vim Syntax](https://github.com/digitaltoad/vim-pug)
- [TextMate Bundle](http://github.com/miksago/jade-tmbundle)
- [Coda/SubEtha syntax Mode](https://github.com/aaronmccall/jade.mode)
- [html2pug](https://github.com/donpark/html2jade) converter
- [pug2php](https://github.com/SE7ENSKY/jade2php) converter
- [Pug Server](https://github.com/ctrlaltdev/pug-server) Ideal for building local prototypes apart from any application
- [cache-pug-templates](https://github.com/ladjs/cache-pug-templates) Cache Pug templates for [Lad](https://github.com/ladjs/lad)/[Koa](https://github.com/koajs/koa)/[Express](https://github.com/expressjs/express)/[Connect](https://github.com/senchalabs/connect) with [Redis](https://redis.io)
- [Prettier Plugin](https://github.com/prettier/plugin-pug)

## Backers

Support us with a monthly donation and help us continue our activities. [[Become a backer](https://opencollective.com/pug#backer)]

<a href="https://opencollective.com/pug/backer/0/website" target="_blank"><img src="https://opencollective.com/pug/backer/0/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/1/website" target="_blank"><img src="https://opencollective.com/pug/backer/1/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/2/website" target="_blank"><img src="https://opencollective.com/pug/backer/2/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/3/website" target="_blank"><img src="https://opencollective.com/pug/backer/3/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/4/website" target="_blank"><img src="https://opencollective.com/pug/backer/4/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/5/website" target="_blank"><img src="https://opencollective.com/pug/backer/5/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/6/website" target="_blank"><img src="https://opencollective.com/pug/backer/6/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/7/website" target="_blank"><img src="https://opencollective.com/pug/backer/7/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/8/website" target="_blank"><img src="https://opencollective.com/pug/backer/8/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/9/website" target="_blank"><img src="https://opencollective.com/pug/backer/9/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/10/website" target="_blank"><img src="https://opencollective.com/pug/backer/10/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/11/website" target="_blank"><img src="https://opencollective.com/pug/backer/11/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/12/website" target="_blank"><img src="https://opencollective.com/pug/backer/12/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/13/website" target="_blank"><img src="https://opencollective.com/pug/backer/13/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/14/website" target="_blank"><img src="https://opencollective.com/pug/backer/14/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/15/website" target="_blank"><img src="https://opencollective.com/pug/backer/15/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/16/website" target="_blank"><img src="https://opencollective.com/pug/backer/16/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/17/website" target="_blank"><img src="https://opencollective.com/pug/backer/17/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/18/website" target="_blank"><img src="https://opencollective.com/pug/backer/18/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/19/website" target="_blank"><img src="https://opencollective.com/pug/backer/19/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/20/website" target="_blank"><img src="https://opencollective.com/pug/backer/20/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/21/website" target="_blank"><img src="https://opencollective.com/pug/backer/21/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/22/website" target="_blank"><img src="https://opencollective.com/pug/backer/22/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/23/website" target="_blank"><img src="https://opencollective.com/pug/backer/23/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/24/website" target="_blank"><img src="https://opencollective.com/pug/backer/24/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/25/website" target="_blank"><img src="https://opencollective.com/pug/backer/25/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/26/website" target="_blank"><img src="https://opencollective.com/pug/backer/26/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/27/website" target="_blank"><img src="https://opencollective.com/pug/backer/27/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/28/website" target="_blank"><img src="https://opencollective.com/pug/backer/28/avatar.svg"></a>
<a href="https://opencollective.com/pug/backer/29/website" target="_blank"><img src="https://opencollective.com/pug/backer/29/avatar.svg"></a>

## Sponsors

Become a sponsor and get your logo on our README on GitHub with a link to your site. [[Become a sponsor](https://opencollective.com/pug#sponsor)]

<a href="https://opencollective.com/pug/sponsor/0/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/0/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/1/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/1/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/2/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/2/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/3/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/3/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/4/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/4/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/5/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/5/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/6/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/6/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/7/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/7/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/8/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/8/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/9/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/9/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/10/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/10/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/11/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/11/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/12/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/12/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/13/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/13/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/14/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/14/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/15/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/15/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/16/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/16/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/17/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/17/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/18/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/18/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/19/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/19/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/20/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/20/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/21/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/21/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/22/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/22/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/23/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/23/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/24/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/24/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/25/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/25/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/26/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/26/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/27/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/27/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/28/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/28/avatar.svg"></a>
<a href="https://opencollective.com/pug/sponsor/29/website" target="_blank"><img src="https://opencollective.com/pug/sponsor/29/avatar.svg"></a>

## License

MIT
