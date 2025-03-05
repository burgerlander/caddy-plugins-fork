package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	// Labels will be included as the labels on all measurements made to the
	// metric. The label values may have placeholders in them, but the keys may
	// not.
	Labels map[string]string `json:"labels,omitempty"`

	// Buckets will be used as the buckets of the histogram. The default depends
	// on the metric itself.
	Buckets []float64 `json:"buckets,omitempty"`

	// Only observe the value when the response matches against this
	// ResponseMatcher. The default is to always observe the value.
	Matcher *caddyhttp.ResponseMatcher `json:"match,omitempty"`

	histogram       *prometheus.HistogramVec
	hasPlaceholders bool
}

func (m *RequestResponseHistogramMetric) provision(
	ctx caddy.Context, defaultBuckets []float64, metricName string,
) error {
	if m.Buckets == nil {
		m.Buckets = defaultBuckets
	}

	m.histogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      metricName,
			Buckets:   m.Buckets,
		},
		maps.Keys(m.Labels),
	)

	if err := ctx.GetMetricsRegistry().Register(m.histogram); err != nil {
		return fmt.Errorf("registering request timing histogram: %w", err)
	}

	for _, v := range m.Labels {
		if strings.Contains(v, "{") && strings.Contains(v, "}") {
			m.hasPlaceholders = true
			break
		}
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
//		// label can be specified multiple times, its value can have
//		// placeholders, including the special placeholders:
//		//	http.response.header.*
//		//	http.response.status_code
//		label name value
//
//		buckets .005 .01 .025 .05 .1 .25 .5 1 2.5 5 10
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

		case "buckets":
			bucketsStrs := h.RemainingArgs()
			if len(bucketsStrs) == 0 {
				return zero, h.ArgErr()
			}

			for _, bucketStr := range bucketsStrs {
				bucketStr = strings.TrimSpace(bucketStr)
				bucket, err := strconv.ParseFloat(bucketStr, 64)
				if err != nil {
					return zero, fmt.Errorf(
						"parsing bucket %q: %w", bucketStr, err,
					)
				}
				m.Buckets = append(m.Buckets, bucket)
			}

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
