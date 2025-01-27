package handlers

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/sosedoff/gitkit"
)

func init() {
	caddy.RegisterModule(GitRemoteRepo{})
	httpcaddyfile.RegisterHandlerDirective("git_remote_repo", gitRemoteRepoParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"git_remote_repo", httpcaddyfile.Before, "file_server",
	)
}

// GitRemoteRepo is an HTTP middleware module which will serve a git repo using
// either the [dumb or smart][git_transport] HTTP protocols, allowing clients to
// push to or pull from the repo.
//
// This module does _not_ deal with authentication or any other kind of access
// control, take care not to leave your private repos publicly exposed.
//
// [git_transport]: https://git-scm.com/book/en/v2/Git-Internals-Transfer-Protocols
type GitRemoteRepo struct {

	// The path of the git repo's directory. This directory will be created if
	// it doesn't already exist. Default is `{http.vars.root}` if set, or
	// current working directory otherwise.
	Path string `json:"path,omitempty"`
}

var _ caddyhttp.MiddlewareHandler = (*GitRemoteRepo)(nil)

func (GitRemoteRepo) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.git_remote_repo",
		New: func() caddy.Module { return new(GitRemoteRepo) },
	}
}

func (g *GitRemoteRepo) Provision(ctx caddy.Context) error {
	if g.Path == "" {
		g.Path = "{http.vars.root}"
	}

	return nil
}

func (g *GitRemoteRepo) Validate() error {
	return nil
}

func (g *GitRemoteRepo) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	// `gitkit.Server` only exposes the ability to work with a directory of
	// repos, not just a single repo. To get around this we pass into
	// `gitkit.Server` the parent directory of Path, and then to all HTTP
	// requests we'll prefix the name of the repo directory within that parent
	// directory.
	var (
		repl        = r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
		repoDir     = repl.ReplaceAll(g.Path, ".")
		repoDirName = filepath.Base(repoDir)
		parentDir   = filepath.Dir(repoDir)
	)

	if repoDir == "/" {
		return errors.New("Repo cannot be in root directory, must be in some sub-directory")
	}

	srv := gitkit.New(gitkit.Config{
		Dir:        parentDir,
		AutoCreate: true,
	})

	r.URL.Path = caddyhttp.SanitizedPathJoin("/"+repoDirName, r.URL.Path)
	srv.ServeHTTP(rw, r)
	return nil
}

// gitRemoteRepoParseCaddyfile sets up the handler from Caddyfile tokens.
// Syntax:
//
//	git_remote_repo [<matcher>] [<path>]
func gitRemoteRepoParseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	h.Next() // consume directive name
	g := new(GitRemoteRepo)
	if h.NextArg() {
		g.Path = h.Val()
	}
	return g, nil
}
