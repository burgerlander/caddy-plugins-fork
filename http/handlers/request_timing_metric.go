package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(RequestTimingMetric{})
	httpcaddyfile.RegisterHandlerDirective("request_timing_metric", requestTimingMetricParseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder(
		"request_timing_metric", httpcaddyfile.Before, "tracing",
	)
}

// RequestTimingMetric is an HTTP middleware module which will passthrough all
// requests untouched, recording their timing under the
// `mediocre_caddy_plugins_http_request_seconds` histogram metric.
type RequestTimingMetric struct {
	RequestResponseHistogramMetric
}

var (
	_ caddyhttp.MiddlewareHandler = (*RequestTimingMetric)(nil)

	requestTimingMetricDefaultBuckets = []float64{
		.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10,
	}
)

func (RequestTimingMetric) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.request_timing_metric",
		New: func() caddy.Module { return new(RequestTimingMetric) },
	}
}

func (m *RequestTimingMetric) Provision(ctx caddy.Context) error {
	return m.provision(
		ctx, requestTimingMetricDefaultBuckets, "request_seconds",
	)
}

func (m *RequestTimingMetric) ServeHTTP(
	rw http.ResponseWriter, r *http.Request, next caddyhttp.Handler,
) error {
	var (
		rec     = caddyhttp.NewResponseRecorder(rw, nil, nil)
		start   = time.Now()
		err     = next.ServeHTTP(rec, r)
		took    = time.Since(start)
		status  = rec.Status()
		headers = rec.Header()
	)

	if hErr := (caddyhttp.HandlerError{}); errors.As(err, &hErr) {
		status = hErr.StatusCode
	}

	m.observe(r.Context(), status, headers, took.Seconds())

	return err
}

func requestTimingMetricParseCaddyfile(
	h httpcaddyfile.Helper,
) (
	caddyhttp.MiddlewareHandler, error,
) {
	var (
		m   = new(RequestTimingMetric)
		err error
	)

	m.RequestResponseHistogramMetric, err = requestResponseHistogramMetricParseCaddyfile(h)
	return m, err
}
