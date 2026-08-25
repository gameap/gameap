// Package telemetry exposes the panel's Prometheus metrics: the registry,
// the scrape handler and the collectors of the subsystems that report
// (plugins today).
package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Namespace prefixes every metric the panel exports.
const Namespace = "gameap"

// Registry owns the panel's metrics. It is a dedicated registry rather than
// the client library's global one, so tests can build isolated instances.
type Registry struct {
	registry *prometheus.Registry
}

// New builds a registry with the Go runtime and process collectors.
func New() *Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return &Registry{registry: registry}
}

// MustRegister adds collectors; it panics on a duplicate, which is a
// programming error.
func (r *Registry) MustRegister(cs ...prometheus.Collector) {
	r.registry.MustRegister(cs...)
}

// Gather exposes the raw registry for tests and the handler.
func (r *Registry) Gather() prometheus.Gatherer {
	return r.registry
}

// Handler serves the registry in the Prometheus text exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{
		// A collector error must not hide the other metrics from the scraper.
		ErrorHandling: promhttp.ContinueOnError,
	})
}
