# mediocre-caddy-plugins

Plugins to the Caddy webserver which I've developed for myself.

## Build

A Caddy binary with these plugins included can be built using the
[xcaddy][xcaddy] tool:

```bash
xcaddy build --with dev.mediocregopher.com/mediocre-caddy-plugins.git
```

If you want just a specific plugin you can choose it using its module path:

```bash
xcaddy build \
    --with dev.mediocregopher.com/mediocre-caddy-plugins.git/http/handlers/templates/functions
```

It's also possible to build Caddy manually using a custom `main.go` file, see
[the example from the caddy repo][caddymain].

[xcaddy]: https://github.com/caddyserver/xcaddy
[caddymain]: https://github.com/caddyserver/caddy/blob/master/cmd/caddy/main.go

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
        gemtext {
            # All parameters are optional
            gateway_url "https://some.gateway/x/"
        }
    }
}
```

See the `template.localhost` virtual host in `./example/Caddyfile`, and the
associated `./example/tpl/render_gemtext.html` template file, for an example of
how to use this directive.

[gemtext]: https://geminiprotocol.net/docs/gemtext.gmi
[mdfunc]: https://caddyserver.com/docs/modules/http.handlers.templates#markdown

#### Parameters

Optional parameters to the gemtext extension include:

**gateway_url**

If given then any `gemini://` URLs encountered as links within
the document will be appended to this URL, having their `gemini://` scheme
stripped off first.

e.g. if `gateway_url` is `https://some.gateway/x/` then the following line:

```text
=> gemini://geminiprotocol.net Check it out!
```

becomes

```html
<a href="https://some.gateway/x/geminiprotocol.net">Check it out!</a>
```

#### Template function

Within a template being rendered the `gemtext` function will be available and
can be passed any string. The function will return a struct with the following
fields:

* `Body`: The result of converting each line of the input string into an
  equivalent line of HTML. This will not include any wrapping HTML tags like
  `<div>` or `<body>`.

* `Title`: A suggested title, based on the first `# Header` line found in the
  gemtext input.

## Development

A nix-based development environment is provided with the correct versions of all
development dependencies. It can be activated by doing:

```bash
nix-shell
```

The `./cmd/mediocre-caddy` binary package can be used to run a Caddy instance
with all plugins provided by this package pre-installed.

The Caddyfile `./example/Caddyfile` can be used to spin up a Caddy instance with
various virtual-hosts predefined with useful configurations for testing. See
that file for a description of the available virtual hosts.

```bash
go run ./cmd/mediocre-caddy run --config ./example/Caddyfile
```
