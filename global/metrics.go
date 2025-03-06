package global

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricHistogram describes a histogram metric which will be registered with
// Caddy's prometheus registry.
type MetricHistogram struct {
	Name    string    `json:"name"`
	Help    string    `json:"help"`
	Buckets []float64 `json:"buckets"`
	Labels  []string  `json:"labels"`
}

func (mh *MetricHistogram) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	if !d.Args(&mh.Name) {
		return d.ArgErr()
	}

	for nesting := d.Nesting(); d.NextBlock(nesting); {
		switch d.Val() {
		case "help":
			if !d.Args(&mh.Help) {
				return d.ArgErr()
			}

		case "buckets":
			bucketsStrs := d.RemainingArgs()
			if len(bucketsStrs) == 0 {
				return d.ArgErr()
			}

			for _, bucketStr := range bucketsStrs {
				bucketStr = strings.TrimSpace(bucketStr)
				bucket, err := strconv.ParseFloat(bucketStr, 64)
				if err != nil {
					return fmt.Errorf(
						"parsing bucket %q: %w", bucketStr, err,
					)
				}
				mh.Buckets = append(mh.Buckets, bucket)
			}

		case "labels":
			mh.Labels = d.RemainingArgs()

		default:
			return d.ArgErr()
		}
	}
	return nil
}

// Metrics describe all global metrics used within a running Caddy instance.
type Metrics struct {
	Histograms []MetricHistogram `json:"histograms"`
	histograms map[string]*prometheus.HistogramVec
}

// HistogramByName returns the prometheus histogram object configured with the
// given name.
func (m Metrics) HistogramByName(name string) (*prometheus.HistogramVec, bool) {
	h, ok := m.histograms[name]
	return h, ok
}

func (m *Metrics) provision(ctx caddy.Context) error {
	m.histograms = make(map[string]*prometheus.HistogramVec, len(m.Histograms))
	for _, hCfg := range m.Histograms {
		if _, ok := m.histograms[hCfg.Name]; ok {
			return fmt.Errorf("name already used: %q", hCfg.Name)
		}

		histogram := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    hCfg.Name,
				Help:    hCfg.Help,
				Buckets: hCfg.Buckets,
			},
			hCfg.Labels,
		)

		if err := ctx.GetMetricsRegistry().Register(histogram); err != nil {
			return fmt.Errorf("registering histogram %q: %w", hCfg.Name, err)
		}

		m.histograms[hCfg.Name] = histogram
	}

	return nil
}

func (m *Metrics) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name
	for d.NextBlock(0) {
		switch d.Val() {
		case "histogram":
			var mh MetricHistogram
			if err := mh.UnmarshalCaddyfile(d); err != nil {
				return fmt.Errorf("unmarshaling histogram: %w", err)
			}
			m.Histograms = append(m.Histograms, mh)

		default:
			return d.ArgErr()
		}
	}
	return nil
}
