package functions

import (
	"fmt"
	"html"
	"io"
	"net/url"
	"strings"
	"text/template"

	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/gemtext"
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

func (g *Gemtext) funcGemtext(input any) (gemtext.HTML, error) {
	var (
		r          = strings.NewReader(caddy.ToString(input))
		translator gemtext.HTMLTranslator
	)

	if g.GatewayURL != "" {
		translator.RenderLink = func(w io.Writer, urlStr, label string) error {
			if u, err := url.Parse(urlStr); err == nil && u.Scheme == "gemini" {
				urlStr = g.GatewayURL + u.Host + u.Path
			}

			_, err := fmt.Fprintf(
				w, "<p><a href=\"%s\">%s (proxied)</a></p>\n", urlStr, label,
			)
			return err
		}
	}

	return translator.Translate(r)
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
