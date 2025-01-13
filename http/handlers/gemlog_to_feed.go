package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/gemtext"
	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/toolkit"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

const (
	feedFormatRSS  = "rss"
	feedFormatAtom = "atom"
	feedFormatJSON = "json"
)

func init() {
	caddy.RegisterModule(GemlogToFeed{})
	httpcaddyfile.RegisterHandlerDirective("gemlog_to_feed", gemlogToFeedParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"gemlog_to_feed", httpcaddyfile.Before, "templates",
	)
}

// GemlogToFeed is an HTTP middleware module which will convert a gemtext
// response document into an RSS, Atom, or JSON feed, by first intepreting the
// gemtext document as a [gemlog] and making an appropriate conversion from
// there.
//
// [gemlog]: https://geminiprotocol.net/docs/companion/subscription.gmi
type GemlogToFeed struct {

	// Format to output the feed as, either `rss`, `atom`, or `json`. Defaults
	// to `atom`.
	Format string `json:"format"`

	// Optional name to provide in the output feed under author metadata.
	AuthorName string `json:"author_name"`

	// Optional email to provide in the output feed under author metadata.
	AuthorEmail string `json:"author_email"`

	// Optional URL in format `[scheme://host[:port]]/path` to use as the
	// absolute URL all links in the feed will be relative to. If not given then
	// it will be inferred from the request.
	BaseURL string `json:"base_url"`
	baseURL *url.URL
}

var _ caddyhttp.MiddlewareHandler = (*GemlogToFeed)(nil)

func (GemlogToFeed) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.gemlog_to_feed",
		New: func() caddy.Module { return new(GemlogToFeed) },
	}
}

func (g *GemlogToFeed) Provision(ctx caddy.Context) error {
	g.Format = strings.ToLower(g.Format)
	switch g.Format {
	case feedFormatRSS, feedFormatAtom, feedFormatJSON:
	case "":
		g.Format = feedFormatAtom
	}

	if g.BaseURL != "" {
		var err error
		if g.baseURL, err = url.Parse(g.BaseURL); err != nil {
			return fmt.Errorf("Parsing BaseURL failed: %w", err)
		}
	}

	return nil
}

func (g *GemlogToFeed) Validate() error {
	switch strings.ToLower(g.Format) {
	case feedFormatRSS, feedFormatAtom, feedFormatJSON, "":
	default:
		return fmt.Errorf("invalid feed format %q", g.Format)
	}

	return nil
}

func (g *GemlogToFeed) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	buf, bufDone := toolkit.GetBuffer()
	defer bufDone()

	shouldBuf := func(int, http.Header) bool { return true }

	rec := caddyhttp.NewResponseRecorder(rw, buf, shouldBuf)
	if err := next.ServeHTTP(rec, r); err != nil || !rec.Buffered() {
		return err
	}

	// the response recorder still writes the headers, I'm not actually sure
	// why, but some will conflict.
	rec.Header().Del("Content-Length")
	rec.Header().Del("Accept-Ranges")
	rec.Header().Del("Etag")

	buf = rec.Buffer() // probably redundant, but just in case

	var (
		repl    = r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
		baseURL = g.baseURL
		err     error
	)

	if baseURL == nil {
		reqURIStr, ok := repl.GetString("http.request.orig_uri")
		if !ok {
			return errors.New("Placeholder http.request.orig_uri not found in context")
		}

		if baseURL, err = url.Parse(reqURIStr); err != nil {
			return fmt.Errorf("parsing req url %q: %w", reqURIStr, err)
		}

		if baseURL.Host == "" {
			baseURL.Host = r.Host
		}

		if baseURL.Scheme == "" {
			baseURL.Scheme, _ = repl.GetString("http.request.scheme")
		}
	}

	translator := gemtext.FeedTranslator{
		BaseURL:     baseURL,
		AuthorName:  g.AuthorName,
		AuthorEmail: g.AuthorEmail,
	}

	switch g.Format {
	case feedFormatRSS:
		rw.Header().Set("Content-Type", "application/rss+xml")
		return translator.ToRSS(rw, buf)

	case feedFormatAtom:
		rw.Header().Set("Content-Type", "application/atom+xml")
		return translator.ToAtom(rw, buf)

	case feedFormatJSON:
		rw.Header().Set("Content-Type", "application/feed+json")
		return translator.ToJSON(rw, buf)

	default:
		return fmt.Errorf("invalid feed format %q", g.Format)
	}
}

// gemlogToFeedParseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	gemlog_to_feed [<matcher>] {
//		format <format>
//		author_name <author name>
//		author_email <author email>
//	}
func gemlogToFeedParseCaddyfile(
	h httpcaddyfile.Helper,
) (
	caddyhttp.MiddlewareHandler, error,
) {
	h.Next() // consume directive name
	g := new(GemlogToFeed)
	for h.NextBlock(0) {
		switch h.Val() {
		case "format":
			if !h.Args(&g.Format) {
				return nil, h.ArgErr()
			}
		case "author_name":
			if !h.Args(&g.AuthorName) {
				return nil, h.ArgErr()
			}
		case "author_email":
			if !h.Args(&g.AuthorEmail) {
				return nil, h.ArgErr()
			}
		case "base_url":
			if !h.Args(&g.BaseURL) {
				return nil, h.ArgErr()
			}
		}
	}
	return g, nil
}
