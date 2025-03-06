package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"dev.mediocregopher.com/mediocre-caddy-plugins.git/global"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/exp/maps"
)

const metricsNamespace = "mediocre_caddy_plugins_http"

// RequestResponseHistogramMetric contains common fields and logic for metrics
// which record HTTP request/response data into a hisogram.
type RequestResponseHistogramMetric struct {
	// Name refers to the name of a histogram defined as part of the
	// `mediocre_caddy_plugins.metrics` global configuration. It is using this
	// histogram which values will be observed.
	Name string `json:"name"`

	// Labels will be included as the labels on all measurements made to the
	// metric. The label keys must match 1:1 with the labels defined in the
	// global config for the histogram. The label values may have placeholders
	// in them, but the keys may not.
	Labels map[string]string `json:"labels,omitempty"`

	// Only observe the value when the response matches against this
	// ResponseMatcher. The default is to always observe the value.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`

	histogram       *prometheus.HistogramVec
	hasPlaceholders bool
}

func (m *RequestResponseHistogramMetric) Provision(ctx caddy.Context) error {
	for _, v := range m.Labels {
		if strings.Contains(v, "{") && strings.Contains(v, "}") {
			m.hasPlaceholders = true
			break
		}
	}

	appI, err := ctx.AppIfConfigured("mediocre_caddy_plugins")
	if err != nil {
		return err
	}
	app := appI.(*global.App)

	var ok bool
	if m.histogram, ok = app.Metrics.HistogramByName(m.Name); !ok {
		return fmt.Errorf("histogram %q not configured globally", m.Name)
	}

	return nil
}

func (m *RequestResponseHistogramMetric) observe(
	ctx context.Context,
	status int,
	headers http.Header,
	val float64,
) {
	if m.Matcher != nil && !m.Matcher.Match(status, headers) {
		return
	}

	labels := m.Labels
	if m.hasPlaceholders {
		labels = maps.Clone(labels)

		repl := ctx.Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
		for field, value := range headers {
			repl.Set("http.response.header."+field, strings.Join(value, ","))
		}
		repl.Set("http.response.status_code", status)

		for k, v := range labels {
			labels[k] = repl.ReplaceAll(v, "malformed_placeholder")
		}
	}

	m.histogram.With(prometheus.Labels(labels)).Observe(val)
}

// requestResponseHistogramMetricParseCaddyfile sets up the handler helper from
// Caddyfile tokens. Syntax:
//
//	request_timing_metric {
//		name "global_metric_name"
//
//		// label can be specified multiple times, its value can have
//		// placeholders, including the special placeholders:
//		//	http.response.header.*
//		//	http.response.status_code
//		label name value
//
//		match <response matcher>
//	}
func requestResponseHistogramMetricParseCaddyfile(
	h httpcaddyfile.Helper,
) (
	RequestResponseHistogramMetric, error,
) {
	var (
		zero RequestResponseHistogramMetric
		m    = RequestResponseHistogramMetric{
			Labels: map[string]string{},
		}
		responseMatchers = make(map[string]caddyhttp.ResponseMatcher)
	)

	h.Next() // consume directive name

	if !h.Args(&m.Name) {
		return zero, h.ArgErr()
	}

	for h.NextBlock(0) {
		switch h.Val() {
		case "label":
			if !h.NextArg() {
				return zero, h.ArgErr()
			}
			k := h.Val()

			if !h.NextArg() {
				return zero, h.ArgErr()
			}
			m.Labels[k] = h.Val()

		case "match":
			if err := caddyhttp.ParseNamedResponseMatcher(
				h.NewFromNextSegment(), responseMatchers,
			); err != nil {
				return zero, fmt.Errorf("parsing response matcher: %w", err)
			}
			matcher := responseMatchers["match"]
			m.Matcher = &matcher

		default:
			return zero, fmt.Errorf("unknown field: %q", h.Val())
		}
	}

	return m, nil
}
