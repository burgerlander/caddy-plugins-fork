# mediocre-caddy-plugins

TODO proper introduction

## Build

TODO

## Plugins

The following plugins are implemented in this module.

### http.handlers.templates.functions.gemtext

This extension to `templates` allows for rendering a [gemtext][gemtext] string
as a roughly equivalent set of HTML tags. It is similar to the [markdown template
function][mdfunc] in its usage. It can be enabled by being included in the
`templates.extensions` set.

```text
templates {
    extensions {
        gemtext
    }
}
```

See the `template.localhost` virtual host in `example/Caddyfile` for an example
of using the `gemtext` template to render a gemtext file within an HTML file.

[gemtext]: https://geminiprotocol.net/docs/gemtext.gmi
[mdfunc]: https://caddyserver.com/docs/modules/http.handlers.templates#markdown

## Development

A nix-based development environment is provided with the correct versions of all
development dependencies. It can be activated by doing:

```bash
nix-shell -A shell
```

The `./cmd/mediocre-caddy` binary package can be used to run a Caddy instance
with all plugins provided by this package pre-installed.

The Caddyfile `./example/Caddyfile` can be used to spin up a Caddy instance with
various virtual-hosts predefined with usefule configurations for testing. See
that file for a description of the available virtual hosts.

```bash
go run ./cmd/mediocre-caddy run --config ./example/Caddyfile
```
