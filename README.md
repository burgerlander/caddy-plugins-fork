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

### http.handlers.gemtext

This HTTP handler will translate [gemtext][gemtext] documents into HTML
documents. It requires at least one argument, `template`, to which is passed an
HTML template file that gemtext documents will be rendered into.

Only responses with a `Content-Type` of `text/gemini` will be modified by this
module.

Example usage:

```text
http://gemtext.localhost {
	root example/static
	gemtext {
		root example/tpl
		template render_gemtext.html
	}
	file_server
}
```

#### Parameters

**template**

Path to the template which will be used to render the HTML page, relative to the
`root`.

The template will be rendered with these extra data fields:

* `.Title`: The Title of the gemini document, determined based on the first
  primary header (single `#` prefix) found. This will be an empty string if no
  primary header is found.

* `.Body`: A string containing all rendered HTML DOM elements.

**heading_template**

Path to a template which will be used for rendering headings. If not given then
headings will be rendered with appropriate HTML header tags.

The template will be rendered with these extra data fields:

* `.Level`: Which level of heading is being rendered, 1, 2, or 3.

* `.Text`: The text of the heading.

**link_template**

Path to a template which will be used for rendering links. If not given then
links will be rendered using an anchor tag wrapped in a paragraph tag.

The template will be rendered with these extra data fields:

* `.URL`: The URL the link points to.
* `.Label`: The label attached to the link. If the original link had no label
  then this will be equivalent to `.URL`.

**root**

The root path from which to load template files. Default is `{http.vars.root}`
if set, or current working directory otherwise.

**delimiters**

The template action delimiters. Defaults to:

```text
delimiters "{{" "}}"
```

### http.handlers.gemlog_to_feed

This module will convert a gemtext response document into an RSS, Atom, or JSON
feed, by first intepreting the gemtext document as a [gemlog][gemlog] and making
an appropriate conversion from there.

Example usage:

```text
handle_path /gmisub.xml {
	# Rewrite the request path to point to the gemlog file
	rewrite /gmisub.gmi

	# Convert the response from file_server, which is the gemlog file itself,
	# into an ATOM feed.
	gemlog_to_feed {
		format atom
		author_name "Tester"
		author_email "nun@ya.biz"
	}

	file_server
}
```

#### Parameters

**format**

Optional format of the feed to output. Can be one of `rss`, `atom`, or `json`,
defaulting to `atom`.

The `Content-Type` of the response will be set accordingly.

**author_name** and **author_email**

Optional parameters which will be used to populate the top-level author fields
in the output feed.

**base_url**

Optional URL in format `[scheme://host[:port]]/path` to use as the absolute URL
all links in the feed will be relative to. If not given then it will be inferred
from the request.

[gemlog]: https://geminiprotocol.net/docs/companion/subscription.gmi

### http.handlers.git_remote_repo

This module will serve a git repo using either the [dumb or
smart][git_transport] HTTP protocols, allowing clients to push to or pull from
the repo.

This module does _not_ deal with authentication or any other kind of access
control, take care not to leave your private repos publicly exposed.

[git_transport]: https://git-scm.com/book/en/v2/Git-Internals-Transfer-Protocols

```text
# git_remote_repo requires that the sub-directory in the URL path has already
# been stripped. handle_path takes care of this.
handle_path /repo.git/* {

	# Serve the git repository which can be found in the test-repo.git
	# sub-directory of the site root.
	git_remote_repo * "{http.vars.root}/test-repo.git"
}
```

### http.handlers.proof_of_work

This module which will intercept all requests and check that they were made by a
browser which has performed a proof-of-work (PoW) challenge in the recent past.

Any requests which lack a PoW solution will be redirected to a page where a
challenge will be automatically solved. The challenge and solution will be
stored in cookies, and then the browser will be redirected back to the page it
was originally trying to get to.

The objective of this middleware is to allow normal users to continue using a
website, while trying to prevent search engine crawlers, denial-of-service
attacks, and AI scrapers from getting through.

Example Usage:

```text
proof_of_work [matcher] {
	# all parameters are optional
	secret "some secret value"
	target 0x00FFFFFF
	challenge_timeout 12h
	challenge_seed_cookie "__pow_challenge_seed"
	challenge_solution_cookie "__pow_challenge_solution"
	template_path "{http.vars.root}/tpl.html"
}
```

#### Parameters

**secret**

Used to validate a PoW challenge seed. This string should never be shared with
clients, but _must_ be shared amongst all Caddy servers which are serving the
same domain.

If not given then one will be generated on startup. Note that in this case
restarting Caddy will result in all clients requiring a new PoW solution.

**target**

A uint32 indicating how difficult each challenge will be to solve. A _lower_
Target value is more difficult than a higher one.

Defaults to `0x000FFFFF`.

**challenge_timeout**

How long before Challenges are considered expired and cannot be solved. Any
solutions are also expired, and browsers will be redirected back to the
challenge page to solve a new challenge.

Defaults to `12h`.

**challenge_seed_cookie**

The name of the cookie which should be used to store the challenge seed once a
challenge has been solved.

Defaults to `__pow_challenge_seed`.

**challenge_solution_cookie**

The name of the cookie which should be used to store the challenge solution once
a challenge has been solved.

Defaults to `__pow_challenge_solution`.

**template**

Path to HTML template to render in the browser when it is being challenged. If
not given then a simple default is shown.

The template file should include the line
`<script>{{ template "pow.js" . }}</script>` at the end of the `body`
tag. This script will solve a challenge, set the solution to a cookie,
and reload the page.

### http.handlers.{request_timing_metric, response_size_metric}

Usage of these modules requires histograms to be defined under the
`mediocre_caddy_plugins.metrics` global option set. Example:

```text
# top-level options block, where directives like 'admin' and 'debug' go.
{
	mediocre_caddy_plugins {
		metrics {
			# Metric names can be arbitrary, they are only used within address
			# blocks to refer back to the metrics
			histogram custom_request_seconds {
				# All fields inside the block are optional
				help "Optional description of the metric"

				# buckets defaults to this set of thresholds if not given
				buckets 0.005 0.01 0.025 0.05 0.1 0.25 0.5 1 2.5 5 10

				labels vhost path
			}

			histogram custom_response_bytes {
				buckets 256 1024 4096 16384 65536 262144 1048576 4194304
				labels vhost status
			}
		}
	}
}
```

These modules, which are used within an address block, will then passthrough all
requests untouched, recording their timing/response size under the histogram
[metric][metrics] referenced by name in the global options.

Example Usage:

```text
mydomain.com {
	request_timing_metric "custom_request_seconds" {
		label vhost mydomain.com
		label path {http.request.uri.path}
		match status 200
	}

	response_size_metric "custom_response_bytes" {
		label vhost mydomain.com
		label status {http.response.status_code}
	}

	# ...
}
```

[metrics]: https://caddyserver.com/docs/caddyfile/directives/metrics

#### Parameters

**label**

Attach a label to the observation of each request. `label` can be specified
multiple times to attach more than one label, and there must be the exact same
set of labels within the metric as are defined in the globally defined
histogram.

`label` values can contain placeholders, and the following placeholders are made
available in this handler:

* `http.response.header.<header name>`
* `http.response.status_code`

**match**

A [response matcher][respMatcher] which can be used to only record metrics for
requests whose response has particular characteristics.

[respMatcher]: https://caddyserver.com/docs/caddyfile/response-matchers

### http.handlers.templates.functions.gemtext_function

This extension to `templates` allows for rendering a [gemtext][gemtext] string
as a roughly equivalent set of HTML tags. It is similar to the [markdown template
function][mdfunc] in its usage. It can be enabled by being included in the
`templates.extensions` set.

```text
templates {
    extensions {
        gemtext_function {
            # All parameters are optional
            gateway_url "https://some.gateway/x/"
        }
    }
}
```

See the `template.localhost` virtual host in `./example/Caddyfile`, and the
associated `./example/tpl/render_gemtext.html` template file, for an example of
how to use this directive.

[mdfunc]: https://caddyserver.com/docs/modules/http.handlers.templates#markdown

#### Parameters

Optional parameters to the `gemtext_function` extension include:

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

[gemtext]: https://geminiprotocol.net/docs/gemtext.gmi

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
