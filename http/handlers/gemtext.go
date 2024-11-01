package handlers

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/gemtext"
	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/toolkit"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/templates"
	"go.uber.org/zap"
)

// The implementation here is heavily based on the implementation of the
// `templates` module:
// https://github.com/caddyserver/caddy/blob/350ad38f63f7a49ceb3821c58d689b85a27ec4e5/modules/caddyhttp/templates/templates.go

const gemtextMIME = "text/gemini"

func init() {
	caddy.RegisterModule(Gemtext{})
	httpcaddyfile.RegisterHandlerDirective("gemtext", gemtextParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"gemtext", httpcaddyfile.Before, "templates",
	)

	// Since this module relies on Content-Type, but text/gemtext is not a
	// standard type, we add it if it's missing
	if mime.TypeByExtension(".gmi") == "" {
		mime.AddExtensionType(".gmi", gemtextMIME)
	}
}

// Gemtext is an HTTP middleware module which will render gemtext documents as
// HTML documents, using user-provided templates to do so.
//
// Only responses with a Content-Type of `text/gemini` will be modified by this
// module.
type Gemtext struct {

	// Path to the template which will be used to render the HTML page, relative
	// to the `file_root`.
	//
	// The template will be rendered with these extra data fields:
	//
	// ##### `.Title`
	//
	// The Title of the gemini document, determined based on the first primary
	// header (single `#` prefix) found. This will be an empty string if no
	// primary header is found.
	//
	// ##### `.Body`
	//
	// A string containing all rendered HTML DOM elements.
	//
	TemplatePath string `json:"template"`

	// Path to a template which will be used for rendering links. If not given
	// then links will be rendered using an anchor tag wrapped in a paragraph
	// tag.
	//
	// The template will be rendered with these extra data fields:
	//
	// ##### `.URL`
	//
	// The URL the link points to.
	//
	// ##### `.Label`
	//
	// The label attached to the link. If the original link had no label then
	// this will be equivalent to `.URL`.
	LinkTemplatePath string `json:"link_template"`

	// The root path from which to load files. Default is `{http.vars.root}` if
	// set, or current working directory otherwise.
	FileRoot string `json:"file_root,omitempty"`

	// The template action delimiters. If set, must be precisely two elements:
	// the opening and closing delimiters. Default: `["{{", "}}"]`
	Delimiters []string `json:"delimiters,omitempty"`

	logger *zap.Logger
}

var _ caddyhttp.MiddlewareHandler = (*Gemtext)(nil)

func (Gemtext) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.gemtext",
		New: func() caddy.Module { return new(Gemtext) },
	}
}

func (g *Gemtext) Provision(ctx caddy.Context) error {
	g.logger = ctx.Logger()

	if g.FileRoot == "" {
		g.FileRoot = "{http.vars.root}"
	}

	if len(g.Delimiters) == 0 {
		g.Delimiters = []string{"{{", "}}"}
	}

	return nil
}

// Validate ensures t has a valid configuration.
func (g *Gemtext) Validate() error {
	if g.TemplatePath == "" {
		return errors.New("TemplatePath is required")
	}

	if len(g.Delimiters) != 0 && len(g.Delimiters) != 2 {
		return fmt.Errorf("delimiters must consist of exactly two elements: opening and closing")
	}
	return nil
}

func (g *Gemtext) render(
	into io.Writer,
	ctx *templates.TemplateContext,
	osFS fs.FS,
	tplPath string,
	payload any,
) error {
	tplStr, err := fs.ReadFile(osFS, tplPath)
	if err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	tpl := ctx.NewTemplate(tplPath)
	if _, err := tpl.Parse(string(tplStr)); err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	tpl.Delims(g.Delimiters[0], g.Delimiters[1])

	if err := tpl.Execute(into, payload); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

func (g *Gemtext) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	buf, bufDone := toolkit.GetBuffer()
	defer bufDone()

	// We only want to buffer and work on responses which are gemtext files.
	shouldBuf := func(status int, header http.Header) bool {
		ct := header.Get("Content-Type")
		return strings.HasPrefix(ct, gemtextMIME)
	}

	rec := caddyhttp.NewResponseRecorder(rw, buf, shouldBuf)
	if err := next.ServeHTTP(rec, r); err != nil || !rec.Buffered() {
		return err
	}

	buf = rec.Buffer() // probably redundant, but just in case

	var (
		repl    = r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
		rootDir = repl.ReplaceAll(g.FileRoot, ".")
		osFS    = os.DirFS(rootDir)
		httpFS  = http.Dir(rootDir)
		ctx     = &templates.TemplateContext{
			Root:       httpFS,
			Req:        r,
			RespHeader: templates.WrappedHeader{Header: rec.Header()},
		}
	)

	parser := gemtext.HTMLTranslator{}

	if g.LinkTemplatePath != "" {
		parser.RenderLink = func(w io.Writer, url, label string) error {
			payload := struct {
				*templates.TemplateContext
				URL   string
				Label string
			}{
				ctx, url, label,
			}

			return g.render(w, ctx, osFS, g.LinkTemplatePath, payload)
		}
	}

	translated, err := parser.Translate(buf)
	if err != nil {
		return fmt.Errorf("translating gemtext: %w", err)
	}

	payload := struct {
		*templates.TemplateContext
		gemtext.HTML
	}{
		ctx, translated,
	}

	buf.Reset()
	if err := g.render(
		buf, ctx, osFS, g.TemplatePath, payload,
	); err != nil {
		// templates may return a custom HTTP error to be propagated to the
		// client, otherwise for any other error we assume the template is
		// broken
		var handlerErr caddyhttp.HandlerError
		if errors.As(err, &handlerErr) {
			return handlerErr
		}
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	rec.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	rec.Header().Del("Accept-Ranges") // we don't know ranges for dynamically-created content
	rec.Header().Del("Last-Modified") // useless for dynamic content since it's always changing

	// we don't know a way to quickly generate etag for dynamic content,
	// and weak etags still cause browsers to rely on it even after a
	// refresh, so disable them until we find a better way to do this
	rec.Header().Del("Etag")

	// The Content-Type was originally text/gemini, but now it will be text/html
	// (we assume, since the HTML translator was used). Deleting here will cause
	// Caddy to do an auto-detect of the Content-Type, so it will even get the
	// charset properly set.
	rec.Header().Del("Content-Type")

	return rec.WriteResponse()
}

// gemtextParseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	gemtext [<matcher>] {
//	    between <open_delim> <close_delim>
//	    root <path>
//	}
func gemtextParseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	h.Next() // consume directive name
	g := new(Gemtext)
	for h.NextBlock(0) {
		switch h.Val() {
		case "template":
			if !h.Args(&g.TemplatePath) {
				return nil, h.ArgErr()
			}
		case "link_template":
			if !h.Args(&g.LinkTemplatePath) {
				return nil, h.ArgErr()
			}
		case "root":
			if !h.Args(&g.FileRoot) {
				return nil, h.ArgErr()
			}
		case "between":
			g.Delimiters = h.RemainingArgs()
			if len(g.Delimiters) != 2 {
				return nil, h.ArgErr()
			}
		}
	}
	return g, nil
}
