// Package global is used to set up a global mediocre_caddy_plugins App, which is
// primarily used for global options.
package global

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	caddy.RegisterModule(App{})
	httpcaddyfile.RegisterGlobalOption("mediocre_caddy_plugins", parseApp)
}

// App describes all global configuration options of the top-level [caddy.App]
// provided by this module.
type App struct {
	Metrics Metrics `json:"metrics"`
}

func (App) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "mediocre_caddy_plugins",
		New: func() caddy.Module { return new(App) },
	}
}

func (a *App) Start() error { return nil }
func (a *App) Stop() error  { return nil }

func (a *App) Provision(ctx caddy.Context) error {
	if err := a.Metrics.provision(ctx); err != nil {
		return fmt.Errorf("provisioning metrics: %w", err)
	}
	return nil
}

func (a *App) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name
	for d.NextBlock(0) {
		switch d.Val() {
		case "metrics":
			if err := a.Metrics.UnmarshalCaddyfile(d); err != nil {
				return fmt.Errorf("unmarshaling metrics: %w", err)
			}
		default:
			return d.ArgErr()
		}
	}
	return nil
}

// parseApp is used to parse an App from a Caddyfile in the context of a global
// option. Syntax:
//
//	mediocre_caddy_plugins {
//		metrics {
//			histogram <name> { // all fields inside the block are optional
//				help <help/description of the metric>
//				buckets <float> [<float>...]
//				labels <labelName> [<labelName>...]
//			}
//
//			// multiple histograms may be specified, but they must have
//			// different names.
//			histogram <name>
//		}
//	}
func parseApp(d *caddyfile.Dispenser, existingVal any) (any, error) {
	if existingVal != nil {
		return nil, errors.New("mediocre_caddy_plugins previously defined")
	}

	a := new(App)
	if err := a.UnmarshalCaddyfile(d); err != nil {
		return nil, err
	}

	b, err := json.Marshal(a)
	if err != nil {
		return nil, fmt.Errorf("json marshaling App %+v: %w", a, err)
	}

	return httpcaddyfile.App{
		Name:  "mediocre_caddy_plugins",
		Value: json.RawMessage(b),
	}, nil
}
