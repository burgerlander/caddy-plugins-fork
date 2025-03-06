package handlers

import (
	"errors"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(ResponseSizeMetric{})
	httpcaddyfile.RegisterHandlerDirective("response_size_metric", responseSizeMetricParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"response_size_metric", httpcaddyfile.Before, "tracing",
	)
}

// ResponseSizeMetric is an HTTP middleware module which will passthrough all
// requests untouched, recording the size of the response body under the
// `mediocre_caddy_plugins_http_response_bytes` histogram metric.
type ResponseSizeMetric struct {
	RequestResponseHistogramMetric
}

var _ caddyhttp.MiddlewareHandler = (*ResponseSizeMetric)(nil)

func (ResponseSizeMetric) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.response_size_metric",
		New: func() caddy.Module { return new(ResponseSizeMetric) },
	}
}

func (m *ResponseSizeMetric) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	var (
		rec     = caddyhttp.NewResponseRecorder(rw, nil, nil)
		err     = next.ServeHTTP(rec, r)
		status  = rec.Status()
		headers = rec.Header()
	)

	if hErr := (caddyhttp.HandlerError{}); errors.As(err, &hErr) {
		status = hErr.StatusCode
	}

	m.observe(r.Context(), status, headers, float64(rec.Size()))

	return err
}

func responseSizeMetricParseCaddyfile(
	h httpcaddyfile.Helper,
) (
	caddyhttp.MiddlewareHandler, error,
) {
	var (
		m   = new(ResponseSizeMetric)
		err error
	)

	m.RequestResponseHistogramMetric, err = requestResponseHistogramMetricParseCaddyfile(h)
	return m, err
}
