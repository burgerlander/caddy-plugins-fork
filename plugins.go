// Package mediocrecaddyplugins is an index package which automatically imports
// and registers all plugins defined in this module.
package mediocrecaddyplugins

import (
	_ "dev.mediocregopher.com/mediocre-caddy-plugins.git/http/handlers"
	_ "dev.mediocregopher.com/mediocre-caddy-plugins.git/http/handlers/templates/functions"
)
