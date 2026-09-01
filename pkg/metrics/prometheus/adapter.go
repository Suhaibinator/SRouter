package prometheus

import (
	"maps"
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	srouter_metrics "github.com/Suhaibinator/SRouter/pkg/metrics"
)

// PrometheusRegistry adapts a Prometheus Registerer to SRouter's MetricsRegistry interface.
type PrometheusRegistry struct {
	registry  prometheus.Registerer
	namespace string
	subsystem string
	tags      srouter_metrics.Tags
	logger    *zap.Logger
}

// NewPrometheusRegistry creates an adapter for registry. It panics when registry
// is nil. Namespace and subsystem prefix Prometheus metric names; a nil logger
// is replaced by a no-op logger.
func NewPrometheusRegistry(registry prometheus.Registerer, namespace, subsystem string, logger *zap.Logger) *PrometheusRegistry {
	if registry == nil {
		panic("prometheus registry cannot be nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PrometheusRegistry{
		registry:  registry,
		namespace: namespace,
		subsystem: subsystem,
		tags:      make(srouter_metrics.Tags),
		logger:    logger.Named("prom_registry_adapter"),
	}
}

func (s *PrometheusRegistry) constLabels() prometheus.Labels {
	labels := prometheus.Labels{}
	maps.Copy(labels, s.tags)
	return labels
}

// PrometheusCounterBuilder adapts Prometheus counter creation.
type PrometheusCounterBuilder struct {
	registry *PrometheusRegistry
	opts     prometheus.CounterOpts
	labels   []string
}

// Name sets the counter name.
func (b *PrometheusCounterBuilder) Name(name string) srouter_metrics.CounterBuilder {
	b.opts.Name = name
	return b
}

// Description sets the counter help text.
func (b *PrometheusCounterBuilder) Description(desc string) srouter_metrics.CounterBuilder {
	b.opts.Help = desc
	return b
}

// Tag adds a Prometheus const label to the counter.
func (b *PrometheusCounterBuilder) Tag(key, value string) srouter_metrics.CounterBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}

// LabelNames configures variable labels on the Prometheus collector.
//
// Deprecated: SRouter's Counter interface cannot select label values, so Inc
// and Add are no-ops on the resulting vector-backed counter. Use Tag for
// constant dimensions, or use a native prometheus.CounterVec and
// WithLabelValues for variable dimensions.
func (b *PrometheusCounterBuilder) LabelNames(names ...string) srouter_metrics.CounterBuilder {
	b.labels = names
	return b
}

// Build creates and registers the Prometheus counter.
func (b *PrometheusCounterBuilder) Build() srouter_metrics.Counter {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	for k, v := range b.registry.constLabels() {
		if _, exists := b.opts.ConstLabels[k]; !exists {
			b.opts.ConstLabels[k] = v
		}
	}

	var counterVec *prometheus.CounterVec
	if len(b.labels) > 0 {
		counterVec = prometheus.NewCounterVec(b.opts, b.labels)
		if err := b.registry.registry.Register(counterVec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				counterVec = are.ExistingCollector.(*prometheus.CounterVec)
			} else {
				// SRouter interface expects Build to return the metric directly,
				// and Build can be called from the request path, so never panic.
				// The metric still works locally; it just won't be exported.
				b.registry.logger.Error("Failed to register Prometheus counter; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusCounter{registry: b.registry, metricVec: counterVec, name: b.opts.Name, description: b.opts.Help, tags: tags, labelNames: b.labels}
	} else {
		promCounter := prometheus.NewCounter(b.opts)
		if err := b.registry.registry.Register(promCounter); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				promCounter = are.ExistingCollector.(prometheus.Counter)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus counter; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusCounter{registry: b.registry, metric: promCounter, name: b.opts.Name, description: b.opts.Help, tags: tags}
	}
}

// PrometheusGaugeBuilder adapts Prometheus gauge creation.
type PrometheusGaugeBuilder struct {
	registry *PrometheusRegistry
	opts     prometheus.GaugeOpts
	labels   []string
}

// Name sets the gauge name.
func (b *PrometheusGaugeBuilder) Name(name string) srouter_metrics.GaugeBuilder {
	b.opts.Name = name
	return b
}

// Description sets the gauge help text.
func (b *PrometheusGaugeBuilder) Description(desc string) srouter_metrics.GaugeBuilder {
	b.opts.Help = desc
	return b
}

// Tag adds a Prometheus const label to the gauge.
func (b *PrometheusGaugeBuilder) Tag(key, value string) srouter_metrics.GaugeBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}

// LabelNames configures variable labels on the Prometheus collector.
//
// Deprecated: SRouter's Gauge interface cannot select label values, so mutation
// methods are no-ops on the resulting vector-backed gauge. Use Tag for constant
// dimensions, or use a native prometheus.GaugeVec and WithLabelValues for
// variable dimensions.
func (b *PrometheusGaugeBuilder) LabelNames(names ...string) srouter_metrics.GaugeBuilder {
	b.labels = names
	return b
}

// Build creates and registers the Prometheus gauge.
func (b *PrometheusGaugeBuilder) Build() srouter_metrics.Gauge {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	for k, v := range b.registry.constLabels() {
		if _, exists := b.opts.ConstLabels[k]; !exists {
			b.opts.ConstLabels[k] = v
		}
	}

	var gaugeVec *prometheus.GaugeVec
	if len(b.labels) > 0 {
		gaugeVec = prometheus.NewGaugeVec(b.opts, b.labels)
		if err := b.registry.registry.Register(gaugeVec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				gaugeVec = are.ExistingCollector.(*prometheus.GaugeVec)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus gauge; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusGauge{registry: b.registry, metricVec: gaugeVec, name: b.opts.Name, description: b.opts.Help, tags: tags, labelNames: b.labels}
	} else {
		promGauge := prometheus.NewGauge(b.opts)
		if err := b.registry.registry.Register(promGauge); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				promGauge = are.ExistingCollector.(prometheus.Gauge)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus gauge; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusGauge{registry: b.registry, metric: promGauge, name: b.opts.Name, description: b.opts.Help, tags: tags}
	}
}

// PrometheusHistogramBuilder adapts Prometheus histogram creation.
type PrometheusHistogramBuilder struct {
	registry *PrometheusRegistry
	opts     prometheus.HistogramOpts
	labels   []string
}

// Name sets the histogram name.
func (b *PrometheusHistogramBuilder) Name(name string) srouter_metrics.HistogramBuilder {
	b.opts.Name = name
	return b
}

// Description sets the histogram help text.
func (b *PrometheusHistogramBuilder) Description(desc string) srouter_metrics.HistogramBuilder {
	b.opts.Help = desc
	return b
}

// Tag adds a Prometheus const label to the histogram.
func (b *PrometheusHistogramBuilder) Tag(key, value string) srouter_metrics.HistogramBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}

// LabelNames configures variable labels on the Prometheus collector.
//
// Deprecated: SRouter's Histogram interface cannot select label values, so
// Observe is a no-op on the resulting vector-backed histogram. Use Tag for
// constant dimensions, or use a native prometheus.HistogramVec and
// WithLabelValues for variable dimensions.
func (b *PrometheusHistogramBuilder) LabelNames(names ...string) srouter_metrics.HistogramBuilder {
	b.labels = names
	return b
}

// Buckets sets the histogram bucket boundaries.
func (b *PrometheusHistogramBuilder) Buckets(buckets []float64) srouter_metrics.HistogramBuilder {
	b.opts.Buckets = buckets
	return b
}

// Build creates and registers the Prometheus histogram.
func (b *PrometheusHistogramBuilder) Build() srouter_metrics.Histogram {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	for k, v := range b.registry.constLabels() {
		if _, exists := b.opts.ConstLabels[k]; !exists {
			b.opts.ConstLabels[k] = v
		}
	}
	if len(b.opts.Buckets) == 0 {
		b.opts.Buckets = prometheus.DefBuckets
	}

	var histoVec *prometheus.HistogramVec
	if len(b.labels) > 0 {
		histoVec = prometheus.NewHistogramVec(b.opts, b.labels)
		if err := b.registry.registry.Register(histoVec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				histoVec = are.ExistingCollector.(*prometheus.HistogramVec)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus histogram; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusHistogram{registry: b.registry, metricVec: histoVec, name: b.opts.Name, description: b.opts.Help, tags: tags, labelNames: b.labels, buckets: b.opts.Buckets}
	} else {
		promHisto := prometheus.NewHistogram(b.opts)
		if err := b.registry.registry.Register(promHisto); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				promHisto = are.ExistingCollector.(prometheus.Histogram)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus histogram; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusHistogram{registry: b.registry, metric: promHisto, name: b.opts.Name, description: b.opts.Help, tags: tags, buckets: b.opts.Buckets}
	}
}

// PrometheusSummaryBuilder adapts Prometheus summary creation.
type PrometheusSummaryBuilder struct {
	registry *PrometheusRegistry
	opts     prometheus.SummaryOpts
	labels   []string
}

// Name sets the summary name.
func (b *PrometheusSummaryBuilder) Name(name string) srouter_metrics.SummaryBuilder {
	b.opts.Name = name
	return b
}

// Description sets the summary help text.
func (b *PrometheusSummaryBuilder) Description(desc string) srouter_metrics.SummaryBuilder {
	b.opts.Help = desc
	return b
}

// Tag adds a Prometheus const label to the summary.
func (b *PrometheusSummaryBuilder) Tag(key, value string) srouter_metrics.SummaryBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}

// LabelNames configures variable labels on the Prometheus collector.
//
// Deprecated: SRouter's Summary interface cannot select label values, so
// Observe is a no-op on the resulting vector-backed summary. Use Tag for
// constant dimensions, or use a native prometheus.SummaryVec and
// WithLabelValues for variable dimensions.
func (b *PrometheusSummaryBuilder) LabelNames(names ...string) srouter_metrics.SummaryBuilder {
	b.labels = names
	return b
}

// Objectives sets the summary's quantile objectives.
func (b *PrometheusSummaryBuilder) Objectives(objectives map[float64]float64) srouter_metrics.SummaryBuilder {
	b.opts.Objectives = objectives
	return b
}

// MaxAge sets the maximum age of observations in the summary.
func (b *PrometheusSummaryBuilder) MaxAge(age time.Duration) srouter_metrics.SummaryBuilder {
	b.opts.MaxAge = age
	return b
}

// AgeBuckets sets the number of buckets used to calculate quantiles over time.
func (b *PrometheusSummaryBuilder) AgeBuckets(buckets int) srouter_metrics.SummaryBuilder {
	if buckets < 0 {
		b.registry.logger.Warn("Invalid negative value provided for AgeBuckets, defaulting to 0",
			zap.Int(logkeys.ProvidedBuckets, buckets),
			zap.String(logkeys.MetricName, b.opts.Name),
		)
		b.opts.AgeBuckets = 0
	} else if buckets > math.MaxUint32 {
		b.registry.logger.Warn("Value provided for AgeBuckets exceeds MaxUint32, clamping",
			zap.Int(logkeys.ProvidedBuckets, buckets),
			zap.Uint32(logkeys.ClampedValue, math.MaxUint32),
			zap.String(logkeys.MetricName, b.opts.Name),
		)
		b.opts.AgeBuckets = math.MaxUint32
	} else {
		b.opts.AgeBuckets = uint32(buckets)
	}
	return b
}

// BufCap sets the summary's observation buffer capacity.
func (b *PrometheusSummaryBuilder) BufCap(cap uint32) srouter_metrics.SummaryBuilder {
	b.opts.BufCap = cap
	return b
}

// Build creates and registers the Prometheus summary.
func (b *PrometheusSummaryBuilder) Build() srouter_metrics.Summary {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	for k, v := range b.registry.constLabels() {
		if _, exists := b.opts.ConstLabels[k]; !exists {
			b.opts.ConstLabels[k] = v
		}
	}
	if len(b.opts.Objectives) == 0 {
		b.opts.Objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	}

	var summaryVec *prometheus.SummaryVec
	if len(b.labels) > 0 {
		summaryVec = prometheus.NewSummaryVec(b.opts, b.labels)
		if err := b.registry.registry.Register(summaryVec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				summaryVec = are.ExistingCollector.(*prometheus.SummaryVec)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus summary; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusSummary{registry: b.registry, metricVec: summaryVec, name: b.opts.Name, description: b.opts.Help, tags: tags, labelNames: b.labels, objectives: b.opts.Objectives}
	} else {
		promSummary := prometheus.NewSummary(b.opts)
		if err := b.registry.registry.Register(promSummary); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				promSummary = are.ExistingCollector.(prometheus.Summary)
			} else {
				// Never panic in the request path; keep the unregistered metric.
				b.registry.logger.Error("Failed to register Prometheus summary; metric will not be exported",
					zap.String(logkeys.MetricName, b.opts.Name), zap.NamedError(logkeys.Error, err))
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusSummary{registry: b.registry, metric: promSummary, name: b.opts.Name, description: b.opts.Help, tags: tags, objectives: b.opts.Objectives}
	}
}

// PrometheusCounter adapts a Prometheus counter to metrics.Counter. A
// vector-backed value can describe and expose the collector but cannot be
// mutated through the backend-neutral interface.
type PrometheusCounter struct {
	registry    *PrometheusRegistry
	metric      prometheus.Counter
	metricVec   *prometheus.CounterVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
}

// Inc increments a scalar counter. It is a no-op for a vector-backed counter
// because the metrics.Counter interface cannot supply label values.
func (c *PrometheusCounter) Inc() {
	if c.metricVec == nil && c.metric != nil {
		c.metric.Inc()
	}
}

// Add adds val to a scalar counter. It is a no-op for a vector-backed counter
// because the metrics.Counter interface cannot supply label values.
func (c *PrometheusCounter) Add(val float64) {
	if c.metricVec == nil && c.metric != nil {
		c.metric.Add(val)
	}
}

// Name returns the unqualified metric name supplied to the builder.
func (c *PrometheusCounter) Name() string { return c.name }

// Description returns the Prometheus help text.
func (c *PrometheusCounter) Description() string { return c.description }

// Type returns metrics.CounterType.
func (c *PrometheusCounter) Type() srouter_metrics.MetricType { return srouter_metrics.CounterType }

// Tags returns the const labels captured when the counter was built.
func (c *PrometheusCounter) Tags() srouter_metrics.Tags { return c.tags }

// PrometheusGauge adapts a Prometheus gauge to metrics.Gauge. A vector-backed
// value cannot be mutated through the backend-neutral interface.
type PrometheusGauge struct {
	registry    *PrometheusRegistry
	metric      prometheus.Gauge
	metricVec   *prometheus.GaugeVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
}

// Set sets a scalar gauge. It is a no-op for a vector-backed gauge because the
// metrics.Gauge interface cannot supply label values.
func (g *PrometheusGauge) Set(val float64) {
	if g.metricVec == nil && g.metric != nil {
		g.metric.Set(val)
	}
}

// Inc increments a scalar gauge. It is a no-op for a vector-backed gauge.
func (g *PrometheusGauge) Inc() {
	if g.metricVec == nil && g.metric != nil {
		g.metric.Inc()
	}
}

// Dec decrements a scalar gauge. It is a no-op for a vector-backed gauge.
func (g *PrometheusGauge) Dec() {
	if g.metricVec == nil && g.metric != nil {
		g.metric.Dec()
	}
}

// Add adds val to a scalar gauge. It is a no-op for a vector-backed gauge.
func (g *PrometheusGauge) Add(val float64) {
	if g.metricVec == nil && g.metric != nil {
		g.metric.Add(val)
	}
}

// Sub subtracts val from a scalar gauge. It is a no-op for a vector-backed gauge.
func (g *PrometheusGauge) Sub(val float64) {
	if g.metricVec == nil && g.metric != nil {
		g.metric.Sub(val)
	}
}

// Name returns the unqualified metric name supplied to the builder.
func (g *PrometheusGauge) Name() string { return g.name }

// Description returns the Prometheus help text.
func (g *PrometheusGauge) Description() string { return g.description }

// Type returns metrics.GaugeType.
func (g *PrometheusGauge) Type() srouter_metrics.MetricType { return srouter_metrics.GaugeType }

// Tags returns the adapter's tag metadata. After WithTags, this metadata may
// differ from the labels on the already-registered collector.
func (g *PrometheusGauge) Tags() srouter_metrics.Tags { return g.tags }

// WithTags returns a metadata copy with merged tags. It does not relabel the
// already-registered Prometheus collector and is not a label-selection API.
func (g *PrometheusGauge) WithTags(tags srouter_metrics.Tags) srouter_metrics.Metric {
	newTags := make(srouter_metrics.Tags)
	maps.Copy(newTags, g.tags)
	maps.Copy(newTags, tags)
	return &PrometheusGauge{
		registry:    g.registry,
		metric:      g.metric,
		metricVec:   g.metricVec,
		name:        g.name,
		description: g.description,
		tags:        newTags,
		labelNames:  g.labelNames,
	}
}

// PrometheusHistogram adapts a Prometheus histogram to metrics.Histogram. A
// vector-backed value cannot be observed through the backend-neutral interface.
type PrometheusHistogram struct {
	registry    *PrometheusRegistry
	metric      prometheus.Histogram
	metricVec   *prometheus.HistogramVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
	buckets     []float64
}

// Observe records val in a scalar histogram. It is a no-op for a vector-backed
// histogram because the metrics.Histogram interface cannot supply label values.
func (h *PrometheusHistogram) Observe(val float64) {
	if h.metricVec == nil && h.metric != nil {
		h.metric.Observe(val)
	}
}

// Name returns the unqualified metric name supplied to the builder.
func (h *PrometheusHistogram) Name() string { return h.name }

// Description returns the Prometheus help text.
func (h *PrometheusHistogram) Description() string { return h.description }

// Type returns metrics.HistogramType.
func (h *PrometheusHistogram) Type() srouter_metrics.MetricType {
	return srouter_metrics.HistogramType
}

// Tags returns the adapter's tag metadata. After WithTags, this metadata may
// differ from the labels on the already-registered collector.
func (h *PrometheusHistogram) Tags() srouter_metrics.Tags { return h.tags }

// WithTags returns a metadata copy with merged tags. It does not relabel the
// already-registered Prometheus collector and is not a label-selection API.
func (h *PrometheusHistogram) WithTags(tags srouter_metrics.Tags) srouter_metrics.Metric {
	newTags := make(srouter_metrics.Tags)
	maps.Copy(newTags, h.tags)
	maps.Copy(newTags, tags)
	return &PrometheusHistogram{
		registry:    h.registry,
		metric:      h.metric,
		metricVec:   h.metricVec,
		name:        h.name,
		description: h.description,
		tags:        newTags,
		labelNames:  h.labelNames,
		buckets:     h.buckets,
	}
}

// PrometheusSummary adapts a Prometheus summary to metrics.Summary. A
// vector-backed value cannot be observed through the backend-neutral interface.
type PrometheusSummary struct {
	registry    *PrometheusRegistry
	metric      prometheus.Summary
	metricVec   *prometheus.SummaryVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
	objectives  map[float64]float64
}

// Observe records val in a scalar summary. It is a no-op for a vector-backed
// summary because the metrics.Summary interface cannot supply label values.
func (s *PrometheusSummary) Observe(val float64) {
	if s.metricVec == nil && s.metric != nil {
		s.metric.Observe(val)
	}
}

// Name returns the unqualified metric name supplied to the builder.
func (s *PrometheusSummary) Name() string { return s.name }

// Description returns the Prometheus help text.
func (s *PrometheusSummary) Description() string { return s.description }

// Type returns metrics.SummaryType.
func (s *PrometheusSummary) Type() srouter_metrics.MetricType { return srouter_metrics.SummaryType }

// Tags returns the adapter's tag metadata. After WithTags, this metadata may
// differ from the labels on the already-registered collector.
func (s *PrometheusSummary) Tags() srouter_metrics.Tags { return s.tags }

// Objectives returns the configured quantile objectives.
func (s *PrometheusSummary) Objectives() map[float64]float64 { return s.objectives }

// WithTags returns a metadata copy with merged tags. It does not relabel the
// already-registered Prometheus collector and is not a label-selection API.
func (s *PrometheusSummary) WithTags(tags srouter_metrics.Tags) srouter_metrics.Metric {
	newTags := make(srouter_metrics.Tags)
	maps.Copy(newTags, s.tags)
	maps.Copy(newTags, tags)
	return &PrometheusSummary{
		registry:    s.registry,
		metric:      s.metric,
		metricVec:   s.metricVec,
		name:        s.name,
		description: s.description,
		tags:        newTags,
		labelNames:  s.labelNames,
		objectives:  s.objectives,
	}
}

// NewCounter returns a Prometheus counter builder.
func (s *PrometheusRegistry) NewCounter() srouter_metrics.CounterBuilder {
	return &PrometheusCounterBuilder{
		registry: s,
		opts: prometheus.CounterOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
			// Name, Help, ConstLabels set by builder methods
		},
	}
}

// NewGauge returns a Prometheus gauge builder.
func (s *PrometheusRegistry) NewGauge() srouter_metrics.GaugeBuilder {
	return &PrometheusGaugeBuilder{
		registry: s,
		opts: prometheus.GaugeOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

// NewHistogram returns a Prometheus histogram builder.
func (s *PrometheusRegistry) NewHistogram() srouter_metrics.HistogramBuilder {
	return &PrometheusHistogramBuilder{
		registry: s,
		opts: prometheus.HistogramOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

// NewSummary returns a Prometheus summary builder.
func (s *PrometheusRegistry) NewSummary() srouter_metrics.SummaryBuilder {
	return &PrometheusSummaryBuilder{
		registry: s,
		opts: prometheus.SummaryOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

// Register is a no-op because each Prometheus builder registers its collector
// during Build.
func (s *PrometheusRegistry) Register(_ srouter_metrics.Metric) error {
	return nil
}

// Get always returns nil, false. The Prometheus client does not expose reliable
// lookup by name; applications should retain references to built metrics.
func (s *PrometheusRegistry) Get(_ string) (srouter_metrics.Metric, bool) {
	return nil, false
}

// Unregister always returns false. The Prometheus client requires the original
// collector rather than a metric name for unregistration.
func (s *PrometheusRegistry) Unregister(_ string) bool {
	return false
}

// Clear is a no-op. Prometheus Registerer does not expose the collectors needed
// to unregister everything.
func (s *PrometheusRegistry) Clear() {
}

// WithTags returns a registry view that applies merged const labels to metrics
// built through that view. New values replace existing values with the same key.
func (s *PrometheusRegistry) WithTags(tags srouter_metrics.Tags) srouter_metrics.MetricsRegistry {
	newTags := make(srouter_metrics.Tags)
	maps.Copy(newTags, s.tags)
	maps.Copy(newTags, tags)
	return &PrometheusRegistry{
		registry:  s.registry,
		namespace: s.namespace,
		subsystem: s.subsystem,
		tags:      newTags,
		logger:    s.logger,
	}
}
