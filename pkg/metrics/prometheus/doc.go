// Package prometheus adapts SRouter's backend-neutral metrics interfaces to
// github.com/prometheus/client_golang/prometheus.
//
// Builders register collectors when Build is called. SRouter tags are mapped to
// Prometheus const labels. Variable-label vectors are not supported by the
// backend-neutral mutation interfaces; use Tag-based instruments for constant
// dimensions or native Prometheus vectors for variable dimensions.
package prometheus
