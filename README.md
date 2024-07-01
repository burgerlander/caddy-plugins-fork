# mediocre-caddy-plugins

TODO proper introduction

## Development

A nix-based development environment is provided with the correct versions of all
development dependencies. It can be activated by doing:

```
nix-shell -A shell
```

The `./cmd/mediocre-caddy` binary package can be used to run a Caddy instance
with all plugins provided by this package pre-installed.

The Caddyfile `./example/Caddyfile` can be used to spin up a Caddy instance with
various virtual-hosts predefined with usefule configurations for testing. See
that file for a description of the available virtual hosts.

```
go run ./cmd/mediocre-caddy run --config ./example/Caddyfile
```
