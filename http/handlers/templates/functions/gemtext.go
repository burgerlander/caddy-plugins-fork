package functions

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"strings"
	"text/template"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
)

func init() {
	caddy.RegisterModule(Gemtext{})
	httpcaddyfile.RegisterDirective(
		"gemtext",
		func(h httpcaddyfile.Helper) ([]httpcaddyfile.ConfigValue, error) {
			var f Gemtext
			err := f.UnmarshalCaddyfile(h.Dispenser)
			return []httpcaddyfile.ConfigValue{{
				Class: "template_function", Value: f,
			}}, err
		},
	)
}

type Gemtext struct {

	// If given then any `gemini://` URLs encountered as links within the
	// document will be appended to this URL, having their `gemini://` scheme
	// stripped off first.
	//
	// e.g. if `gateway_url` is `https://some.gateway/x/` then the following
	// line:
	//
	//	=> gemini://geminiprotocol.net Check it out!
	//
	// becomes
	//
	//	<a href="https://some.gateway/x/geminiprotocol.net">Check it out!</a>
	GatewayURL string `json:"gateway_url,omitempty"`
}

var _ templates.CustomFunctions = (*Gemtext)(nil)

func (f *Gemtext) CustomTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		"gemtext": f.funcGemtext,
	}
}

func (Gemtext) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.templates.functions.gemtext",
		New: func() caddy.Module { return new(Gemtext) },
	}
}

func sanitizeText(str string) string {
	return html.EscapeString(strings.TrimSpace(str))
}

type gemtextResult struct {
	Title string
	Body  string
}

func (g *Gemtext) funcGemtext(input any) (gemtextResult, error) {
	var (
		r         = bufio.NewReader(strings.NewReader(caddy.ToString(input)))
		w         = new(bytes.Buffer)
		title     string
		pft, list bool
		writeErr  error
	)

	write := func(fmtStr string, args ...any) {
		if writeErr != nil {
			return
		}
		fmt.Fprintf(w, fmtStr, args...)
	}

loop:
	for {
		if writeErr != nil {
			return gemtextResult{}, fmt.Errorf("writing line: %w", writeErr)
		}

		line, err := r.ReadString('\n')

		switch {
		case errors.Is(err, io.EOF):
			break loop

		case err != nil:
			return gemtextResult{}, fmt.Errorf("reading next line: %w", err)

		case strings.HasPrefix(line, "```"):
			if !pft {
				write("<pre>\n")
				pft = true
			} else {
				write("</pre>\n")
				pft = false
			}
			continue

		case pft:
			write(line)
			continue

		case len(strings.TrimSpace(line)) == 0:
			continue
		}

		// list case is special, because it requires a prefix and suffix tag
		if strings.HasPrefix(line, "*") {
			if !list {
				write("<ul>\n")
			}
			write("<li>%s</li>\n", sanitizeText(line[1:]))
			list = true
			continue
		} else if list {
			write("</ul>\n")
			list = false
		}

		switch {
		case strings.HasPrefix(line, "=>"):
			// TODO convert gemini:// links ?
			var (
				line   = strings.TrimSpace(line[2:])
				urlStr = line
				label  = urlStr
			)

			if i := strings.IndexAny(urlStr, " \t"); i > -1 {
				urlStr, label = urlStr[:i], sanitizeText(urlStr[i:])
			}

			if g.GatewayURL != "" {
				if u, err := url.Parse(urlStr); err == nil && u.Scheme == "gemini" {
					urlStr = g.GatewayURL + u.Host + u.Path
				}
			}

			write("<p><a href=\"%s\">%s</a></p>\n", urlStr, label)

		case strings.HasPrefix(line, "###"):
			write("<h3>%s</h3>\n", sanitizeText(line[3:]))

		case strings.HasPrefix(line, "##"):
			write("<h2>%s</h2>\n", sanitizeText(line[2:]))

		case strings.HasPrefix(line, "#"):
			line = sanitizeText(line[1:])
			if title == "" {
				title = line
			}
			write("<h1>%s</h1>\n", line)

		case strings.HasPrefix(line, ">"):
			write("<blockquote>%s</blockquote>\n", sanitizeText(line[1:]))

		default:
			line = strings.TrimSpace(line)
			write("<p>%s</p>\n", line)
		}
	}

	return gemtextResult{
		Title: title,
		Body:  w.String(),
	}, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (g *Gemtext) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name

	for nesting := d.Nesting(); d.NextBlock(nesting); {
		v := d.Val()
		switch v {
		case "gateway_url":
			if !d.Args(&g.GatewayURL) {
				return d.ArgErr()
			} else if _, err := url.Parse(g.GatewayURL); err != nil {
				return fmt.Errorf("invalid gateway url: %w", err)
			}

		default:
			return fmt.Errorf("unknown directive %q", v)
		}
	}

	return nil
}
