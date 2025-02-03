package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"time"

	"dev.mediocregopher.com/mediocre-caddy-plugins.git/internal/pow"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"

	_ "embed"
)

func init() {
	caddy.RegisterModule(ProofOfWork{})
	httpcaddyfile.RegisterHandlerDirective("proof_of_work", proofOfWorkParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"proof_of_work", httpcaddyfile.Before, "basicauth",
	)
}

var (
	//go:embed pow.js
	powJS string

	//go:embed pow.html
	powHTML string
)

// ProofOfWork is an HTTP middleware module which will intercept all requests
// and check that they were made by a browser which has performed a
// proof-of-work (PoW) challenge in the recent past.
//
// Any requests which lack a PoW solution will be redirected to a page where a
// challenge will be automatically solved. The challenge and solution will be
// stored in cookies, and then the browser will be redirected back to the page
// it was originally trying to get to.
//
// The objective of this middleware is to allow normal users to continue using a
// website, while trying to prevent search engine crawlers, denial-of-service
// attacks, and AI scrapers from getting through.
type ProofOfWork struct {

	// Secret is used to validate a PoW challenge seed. This string should never
	// be shared with clients, but _must_ be shared amongst all Caddy servers
	// which are serving the same domain.
	//
	// If not given then one will be generated on startup. Note that in this
	// case restarting Caddy will result in all clients requiring a new PoW
	// solution.
	Secret string `json:"secret,omitempty"`

	// Target is a uint32 indicating how difficult each challenge will be to
	// solve. A _lower_ Target value is more difficult than a higher one.
	//
	// Defaults to 0x000FFFFF
	Target uint32 `json:"target,omitempty"`

	// ChallengeTimeout indicates how long before Challenges are considered
	// expired and cannot be solved. Any solutions are also expired, and
	// browsers will be redirected back to the challenge page to solve a new
	// challenge.
	//
	// Defaults to 12h.
	ChallengeTimeout time.Duration `json:"challenge_timeout,omitempty"`

	// ChallengeSeedCookie indicates the name of the cookie which should be used
	// to store the challenge seed once a challenge has been solved.
	//
	// Defaults to "__pow_challenge_seed".
	ChallengeSeedCookie string `json:"challenge_seed_cookie,omitempty"`

	// ChallengeSolutionCookie indicates the name of the cookie which should be
	// used to store the challenge solution once a challenge has been solved.
	//
	// Defaults to "__pow_challenge_solution".
	ChallengeSolutionCookie string `json:"challenge_solution_cookie,omitempty"`

	// Path to HTML template to render in the browser when it is being
	// challenged. If not given then a simple default is shown.
	//
	// The template file should include the line
	// `<script>{{ template "pow.js" . }}</script>` at the end of the `body`
	// tag. This script will solve a challenge, set the solution to a cookie,
	// and reload the page.
	TemplatePath string `json:"template"`

	store  pow.Store
	mgr    pow.Manager
	logger *zap.Logger
}

var _ caddyhttp.MiddlewareHandler = (*ProofOfWork)(nil)

func (ProofOfWork) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.proof_of_work",
		New: func() caddy.Module { return new(ProofOfWork) },
	}
}

func (p *ProofOfWork) Provision(ctx caddy.Context) error {
	secret := []byte(p.Secret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generating secret value: %w", err)
		}
	}

	if p.Target == 0 {
		p.Target = 0x000FFFFF
	}

	if p.ChallengeSeedCookie == "" {
		p.ChallengeSeedCookie = "__pow_challenge_seed"
	}

	if p.ChallengeSolutionCookie == "" {
		p.ChallengeSolutionCookie = "__pow_challenge_solution"
	}

	p.store = pow.NewMemoryStore(nil)
	p.mgr = pow.NewManager(p.store, secret, &pow.ManagerOpts{
		Target:           p.Target,
		ChallengeTimeout: p.ChallengeTimeout,
	})

	p.logger = ctx.Logger()

	return nil
}

func (p *ProofOfWork) Cleanup() error {
	if err := p.store.Close(); err != nil {
		return fmt.Errorf("closing the storage component: %w", err)
	}
	return nil
}

func (p *ProofOfWork) loadTemplate(path string) (*template.Template, error) {
	powTpl, err := template.New("pow.js").Parse(powJS)
	if err != nil {
		return nil, fmt.Errorf("parsing pow.js: %w", err)
	}

	var (
		powHTMLBody = powHTML
		powHTMLName = "pow.html"
	)

	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading file: %w", err)
		}

		powHTMLBody = string(b)
		powHTMLName = path
	}

	if powTpl, err = powTpl.New("").Parse(powHTMLBody); err != nil {
		return nil, fmt.Errorf("parsing %q: %w", powHTMLName, err)
	}

	return powTpl, nil
}

func (p *ProofOfWork) checkSolution(r *http.Request) error {
	var (
		getCookieBytes = func(name string) []byte {
			cookie, err := r.Cookie(name)
			if err != nil {
				return nil
			}

			b, _ := hex.DecodeString(cookie.Value)
			return b
		}

		seed     = getCookieBytes(p.ChallengeSeedCookie)
		solution = getCookieBytes(p.ChallengeSolutionCookie)
	)

	if len(seed) == 0 || len(solution) == 0 {
		return errors.New("seed and/or solution not given")
	}

	return p.mgr.CheckSolution(seed, solution)
}

func (p *ProofOfWork) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	err := p.checkSolution(r)
	if err == nil {
		return next.ServeHTTP(rw, r)
	}

	p.logger.Warn(
		"Proof-of-work solution not present or not valid, will force a challenge",
		zap.String("userAgent", r.UserAgent()),
		zap.String("url", r.URL.String()),
		zap.Error(err),
	)

	tplPath := ""
	if p.TemplatePath != "" {
		repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
		tplPath = repl.ReplaceAll(p.TemplatePath, ".")
	}

	powTpl, err := p.loadTemplate(tplPath)
	if err != nil {
		return fmt.Errorf("loading template from %q: %w", tplPath, err)
	}

	c := p.mgr.NewChallenge()

	tplData := struct {
		Seed                    string
		Target                  uint32
		ChallengeSeedCookie     string
		ChallengeSolutionCookie string
	}{
		Seed:                    hex.EncodeToString(c.Seed),
		Target:                  c.Target,
		ChallengeSeedCookie:     p.ChallengeSeedCookie,
		ChallengeSolutionCookie: p.ChallengeSolutionCookie,
	}

	if err := powTpl.Execute(rw, tplData); err != nil {
		return fmt.Errorf("executing PoW template failed: %w", err)
	}

	return nil
}

// proofOfWorkParseCaddyfile sets up the handler from Caddyfile tokens. Syntax:
//
//	proof_of_work [matcher] {
//		# all parameters are optional
//		secret "some secret value"
//		target 0x00FFFFFF
//		challenge_timeout 12h
//		challenge_seed_cookie "__pow_challenge_seed"
//		challenge_solution_cookie "__pow_challenge_solution"
//		template_path "{http.vars.root}/tpl.html"
//	}
func proofOfWorkParseCaddyfile(
	h httpcaddyfile.Helper,
) (
	caddyhttp.MiddlewareHandler, error,
) {
	h.Next() // consume directive name
	p := new(ProofOfWork)
	for h.NextBlock(0) {
		switch h.Val() {
		case "secret":
			if !h.Args(&p.Secret) {
				return nil, h.ArgErr()
			}

		case "target":
			if !h.NextArg() {
				return nil, h.ArgErr()
			}

			target, err := strconv.ParseUint(h.Val(), 0, 32)
			if err != nil {
				return nil, fmt.Errorf("parsing %q as a uint32: %w", h.Val(), err)
			}

			p.Target = uint32(target)

		case "challenge_timeout":
			if !h.NextArg() {
				return nil, h.ArgErr()
			}

			var err error
			if p.ChallengeTimeout, err = time.ParseDuration(h.Val()); err != nil {
				return nil, fmt.Errorf("parsing %q as timeout: %w", h.Val(), err)
			}

		case "challenge_seed_cookie":
			if !h.Args(&p.ChallengeSeedCookie) {
				return nil, h.ArgErr()
			}

		case "challenge_solution_cookie":
			if !h.Args(&p.ChallengeSolutionCookie) {
				return nil, h.ArgErr()
			}

		case "template":
			if !h.Args(&p.TemplatePath) {
				return nil, h.ArgErr()
			}
		}
	}

	return p, nil
}
