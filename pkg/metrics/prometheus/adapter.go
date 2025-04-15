package prometheus

import (
	"maps"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	srouter_metrics "github.com/Suhaibinator/SRouter/pkg/metrics"
)

// --- SRouter MetricsRegistry Adapter ---

// SRouterPrometheusRegistry adapts a Prometheus Registerer/Gatherer to SRouter's MetricsRegistry interface.
type SRouterPrometheusRegistry struct {
	// Use the Registerer interface for broader compatibility and easier testing.
	// Note: For Gather() functionality (if needed later), the underlying type
	// would likely need to also implement prometheus.Gatherer.
	registry  prometheus.Registerer
	namespace string
	subsystem string
	tags      srouter_metrics.Tags // Prometheus doesn't directly support arbitrary tags in the same way, use const labels
}

// NewSRouterPrometheusRegistry creates a new adapter using a prometheus.Registerer.
func NewSRouterPrometheusRegistry(registry prometheus.Registerer, namespace, subsystem string) *SRouterPrometheusRegistry {
	if registry == nil {
		// Default to a new standard registry if nil is provided? Or panic?
		// For now, let's assume a valid registry is required.
		// Consider adding error handling or defaulting if needed.
		panic("prometheus registry cannot be nil")
	}
	return &SRouterPrometheusRegistry{
		registry:  registry,
		namespace: namespace,
		subsystem: subsystem,
		tags:      make(srouter_metrics.Tags),
	}
}

// Helper to convert SRouter tags to Prometheus const labels
func (s *SRouterPrometheusRegistry) constLabels() prometheus.Labels {
	labels := prometheus.Labels{}
	maps.Copy(labels, s.tags)
	return labels
}

// --- Builder Implementations ---

// PrometheusCounterBuilder adapts Prometheus counter creation.
type PrometheusCounterBuilder struct {
	registry *SRouterPrometheusRegistry
	opts     prometheus.CounterOpts
	labels   []string
}

func (b *PrometheusCounterBuilder) Name(name string) srouter_metrics.CounterBuilder {
	b.opts.Name = name
	return b
}
func (b *PrometheusCounterBuilder) Description(desc string) srouter_metrics.CounterBuilder {
	b.opts.Help = desc
	return b
}
func (b *PrometheusCounterBuilder) Tag(key, value string) srouter_metrics.CounterBuilder {
	// Tags applied at build time become const labels
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}
func (b *PrometheusCounterBuilder) LabelNames(names ...string) srouter_metrics.CounterBuilder {
	b.labels = names
	return b
}

// Build creates and registers the Prometheus counter.
func (b *PrometheusCounterBuilder) Build() srouter_metrics.Counter {
	// Merge registry tags (const labels)
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	for k, v := range b.registry.constLabels() {
		if _, exists := b.opts.ConstLabels[k]; !exists { // Don't overwrite builder tags
			b.opts.ConstLabels[k] = v
		}
	}

	var counterVec *prometheus.CounterVec
	if len(b.labels) > 0 {
		counterVec = prometheus.NewCounterVec(b.opts, b.labels)
		if err := b.registry.registry.Register(counterVec); err != nil {
			// Handle already registered error gracefully if needed
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				// If it's already registered, assume we want to use the existing one.
				// Note: This might not be the exact same metric if labels/help changed.
				// Prometheus client doesn't easily allow getting the existing metric by opts.
				// We'll re-fetch based on the Vec structure.
				counterVec = are.ExistingCollector.(*prometheus.CounterVec)
			} else {
				// SRouter interface expects Build to return the metric directly.
				// Handle fatal error - cannot proceed without the metric.
				b.registry.registry.MustRegister(counterVec) // Re-attempt registration, will panic on non-AlreadyRegisteredError
			}
		}
		// Convert prometheus.Labels to srouter_metrics.Tags
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		for k, v := range b.opts.ConstLabels {
			tags[k] = v
		}
		return &PrometheusCounter{registry: b.registry, metricVec: counterVec, name: b.opts.Name, description: b.opts.Help, tags: tags, labelNames: b.labels}
	} else {
		// For non-vector counter
		promCounter := prometheus.NewCounter(b.opts)
		if err := b.registry.registry.Register(promCounter); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				promCounter = are.ExistingCollector.(prometheus.Counter)
			} else {
				b.registry.registry.MustRegister(promCounter) // Re-attempt registration, will panic on non-AlreadyRegisteredError
			}
		}
		// Convert prometheus.Labels to srouter_metrics.Tags
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusCounter{registry: b.registry, metric: promCounter, name: b.opts.Name, description: b.opts.Help, tags: tags}
	}
}

// PrometheusGaugeBuilder adapts Prometheus gauge creation.
type PrometheusGaugeBuilder struct {
	registry *SRouterPrometheusRegistry
	opts     prometheus.GaugeOpts
	labels   []string
}

func (b *PrometheusGaugeBuilder) Name(name string) srouter_metrics.GaugeBuilder {
	b.opts.Name = name
	return b
}
func (b *PrometheusGaugeBuilder) Description(desc string) srouter_metrics.GaugeBuilder {
	b.opts.Help = desc
	return b
}
func (b *PrometheusGaugeBuilder) Tag(key, value string) srouter_metrics.GaugeBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}
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
				b.registry.registry.MustRegister(gaugeVec) // Panic on error
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
				b.registry.registry.MustRegister(promGauge) // Panic on error
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		for k, v := range b.opts.ConstLabels {
			tags[k] = v
		}
		return &PrometheusGauge{registry: b.registry, metric: promGauge, name: b.opts.Name, description: b.opts.Help, tags: tags}
	}
}

// PrometheusHistogramBuilder adapts Prometheus histogram creation.
type PrometheusHistogramBuilder struct {
	registry *SRouterPrometheusRegistry
	opts     prometheus.HistogramOpts
	labels   []string
}

func (b *PrometheusHistogramBuilder) Name(name string) srouter_metrics.HistogramBuilder {
	b.opts.Name = name
	return b
}
func (b *PrometheusHistogramBuilder) Description(desc string) srouter_metrics.HistogramBuilder {
	b.opts.Help = desc
	return b
}
func (b *PrometheusHistogramBuilder) Tag(key, value string) srouter_metrics.HistogramBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}
func (b *PrometheusHistogramBuilder) LabelNames(names ...string) srouter_metrics.HistogramBuilder {
	b.labels = names
	return b
}

// Buckets sets the histogram buckets, accepting a slice per the interface.
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
		b.opts.Buckets = prometheus.DefBuckets // Default buckets
	}

	var histoVec *prometheus.HistogramVec
	if len(b.labels) > 0 {
		histoVec = prometheus.NewHistogramVec(b.opts, b.labels)
		if err := b.registry.registry.Register(histoVec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				histoVec = are.ExistingCollector.(*prometheus.HistogramVec)
			} else {
				b.registry.registry.MustRegister(histoVec) // Panic on error
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
				b.registry.registry.MustRegister(promHisto) // Panic on error
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		maps.Copy(tags, b.opts.ConstLabels)
		return &PrometheusHistogram{registry: b.registry, metric: promHisto, name: b.opts.Name, description: b.opts.Help, tags: tags, buckets: b.opts.Buckets}
	}
}

// PrometheusSummaryBuilder adapts Prometheus summary creation.
type PrometheusSummaryBuilder struct {
	registry *SRouterPrometheusRegistry
	opts     prometheus.SummaryOpts
	labels   []string
}

func (b *PrometheusSummaryBuilder) Name(name string) srouter_metrics.SummaryBuilder {
	b.opts.Name = name
	return b
}
func (b *PrometheusSummaryBuilder) Description(desc string) srouter_metrics.SummaryBuilder {
	b.opts.Help = desc
	return b
}
func (b *PrometheusSummaryBuilder) Tag(key, value string) srouter_metrics.SummaryBuilder {
	if b.opts.ConstLabels == nil {
		b.opts.ConstLabels = make(prometheus.Labels)
	}
	b.opts.ConstLabels[key] = value
	return b
}
func (b *PrometheusSummaryBuilder) LabelNames(names ...string) srouter_metrics.SummaryBuilder {
	b.labels = names
	return b
}
func (b *PrometheusSummaryBuilder) Objectives(objectives map[float64]float64) srouter_metrics.SummaryBuilder {
	b.opts.Objectives = objectives
	return b
}
func (b *PrometheusSummaryBuilder) MaxAge(age time.Duration) srouter_metrics.SummaryBuilder {
	b.opts.MaxAge = age
	return b
}

// AgeBuckets sets the number of buckets used to calculate quantiles over time. Accepts int per the interface.
func (b *PrometheusSummaryBuilder) AgeBuckets(buckets int) srouter_metrics.SummaryBuilder {
	// Prometheus client uses uint32, cast carefully.
	if buckets < 0 {
		// Handle invalid input, maybe log or default? Defaulting to 0 for now.
		b.opts.AgeBuckets = 0
	} else {
		b.opts.AgeBuckets = uint32(buckets)
	}
	return b
}
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
	// Default objectives if none provided
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
				b.registry.registry.MustRegister(summaryVec) // Panic on error
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
				b.registry.registry.MustRegister(promSummary) // Panic on error
			}
		}
		tags := make(srouter_metrics.Tags, len(b.opts.ConstLabels))
		for k, v := range b.opts.ConstLabels {
			tags[k] = v
		}
		return &PrometheusSummary{registry: b.registry, metric: promSummary, name: b.opts.Name, description: b.opts.Help, tags: tags, objectives: b.opts.Objectives}
	}
}

// --- Metric Implementations ---

// PrometheusCounter adapts prometheus.Counter/CounterVec to srouter_metrics.Counter.
type PrometheusCounter struct {
	registry    *SRouterPrometheusRegistry // Keep registry reference if needed for WithTags logic
	metric      prometheus.Counter
	metricVec   *prometheus.CounterVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string // Store label names for validation/consistency if needed
}

// Inc increments the counter. Label values are ignored as the SRouter interface expects Inc().
func (c *PrometheusCounter) Inc() {
	if c.metricVec != nil {
		// Cannot Inc a Vec without labels per the interface. This is an interface mismatch.
		// Log error or no-op. Choosing no-op for simplicity here.
	} else if c.metric != nil {
		c.metric.Inc()
	}
}

// Add increments the counter by a given value. Label values are ignored as the SRouter interface expects Add(float64).
func (c *PrometheusCounter) Add(val float64) {
	if c.metricVec != nil {
		// Cannot Add to a Vec without labels per the interface. This is an interface mismatch.
		// Log error or no-op. Choosing no-op for simplicity here.
	} else if c.metric != nil {
		c.metric.Add(val)
	}
}
func (c *PrometheusCounter) Name() string                     { return c.name }
func (c *PrometheusCounter) Description() string              { return c.description }
func (c *PrometheusCounter) Type() srouter_metrics.MetricType { return srouter_metrics.CounterType }
func (c *PrometheusCounter) Tags() srouter_metrics.Tags       { return c.tags }

// PrometheusGauge adapts prometheus.Gauge/GaugeVec to srouter_metrics.Gauge.
type PrometheusGauge struct {
	registry    *SRouterPrometheusRegistry
	metric      prometheus.Gauge
	metricVec   *prometheus.GaugeVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
}

// Set sets the gauge value. Label values are ignored as the SRouter interface expects Set(float64).
func (g *PrometheusGauge) Set(val float64) {
	if g.metricVec != nil {
		// Cannot Set on a Vec without labels per the interface. Interface mismatch.
		// No-op for simplicity.
	} else if g.metric != nil {
		g.metric.Set(val)
	}
}

// Inc increments the gauge. Label values are ignored as the SRouter interface expects Inc().
func (g *PrometheusGauge) Inc() {
	if g.metricVec != nil {
		// No-op on Vec without labels (interface mismatch)
	} else if g.metric != nil {
		g.metric.Inc()
	}
}

// Dec decrements the gauge. Label values are ignored as the SRouter interface expects Dec().
func (g *PrometheusGauge) Dec() {
	if g.metricVec != nil {
		// No-op on Vec without labels (interface mismatch)
	} else if g.metric != nil {
		g.metric.Dec()
	}
}

// Add adds the given value to the gauge. Label values are ignored as the SRouter interface expects Add(float64).
func (g *PrometheusGauge) Add(val float64) {
	if g.metricVec != nil {
		// No-op on Vec without labels (interface mismatch)
	} else if g.metric != nil {
		g.metric.Add(val)
	}
}

// Sub subtracts the given value from the gauge. Label values are ignored as the SRouter interface expects Sub(float64).
func (g *PrometheusGauge) Sub(val float64) {
	if g.metricVec != nil {
		// No-op on Vec without labels (interface mismatch)
	} else if g.metric != nil {
		g.metric.Sub(val)
	}
}
func (g *PrometheusGauge) Name() string                     { return g.name }
func (g *PrometheusGauge) Description() string              { return g.description }
func (g *PrometheusGauge) Type() srouter_metrics.MetricType { return srouter_metrics.GaugeType }
func (g *PrometheusGauge) Tags() srouter_metrics.Tags       { return g.tags }

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

// PrometheusHistogram adapts prometheus.Histogram/HistogramVec to srouter_metrics.Histogram.
type PrometheusHistogram struct {
	registry    *SRouterPrometheusRegistry
	metric      prometheus.Histogram
	metricVec   *prometheus.HistogramVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
	buckets     []float64 // Store buckets
}

// Observe adds a single observation to the histogram. Label values are ignored as the SRouter interface expects Observe(float64).
func (h *PrometheusHistogram) Observe(val float64) {
	if h.metricVec != nil {
		// Cannot Observe on a Vec without labels per the interface. Interface mismatch.
		// No-op for simplicity.
	} else if h.metric != nil {
		h.metric.Observe(val)
	}
}
func (h *PrometheusHistogram) Name() string                     { return h.name }
func (h *PrometheusHistogram) Description() string              { return h.description }
func (h *PrometheusHistogram) Type() srouter_metrics.MetricType { return srouter_metrics.HistogramType }
func (h *PrometheusHistogram) Tags() srouter_metrics.Tags       { return h.tags }
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

// PrometheusSummary adapts prometheus.Summary/SummaryVec to srouter_metrics.Summary.
type PrometheusSummary struct {
	registry    *SRouterPrometheusRegistry
	metric      prometheus.Summary
	metricVec   *prometheus.SummaryVec
	name        string
	description string
	tags        srouter_metrics.Tags
	labelNames  []string
	objectives  map[float64]float64 // Store objectives
}

// Observe adds a single observation to the summary. Label values are ignored as the SRouter interface expects Observe(float64).
func (s *PrometheusSummary) Observe(val float64) {
	if s.metricVec != nil {
		// Cannot Observe on a Vec without labels per the interface. Interface mismatch.
		// No-op for simplicity.
	} else if s.metric != nil {
		s.metric.Observe(val)
	}
}
func (s *PrometheusSummary) Name() string                     { return s.name }
func (s *PrometheusSummary) Description() string              { return s.description }
func (s *PrometheusSummary) Type() srouter_metrics.MetricType { return srouter_metrics.SummaryType }
func (s *PrometheusSummary) Tags() srouter_metrics.Tags       { return s.tags }
func (s *PrometheusSummary) Objectives() map[float64]float64  { return s.objectives } // Implement Objectives method
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

// --- MetricsRegistry Implementation ---

func (s *SRouterPrometheusRegistry) NewCounter() srouter_metrics.CounterBuilder {
	return &PrometheusCounterBuilder{
		registry: s,
		opts: prometheus.CounterOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
			// Name, Help, ConstLabels set by builder methods
		},
	}
}

func (s *SRouterPrometheusRegistry) NewGauge() srouter_metrics.GaugeBuilder {
	return &PrometheusGaugeBuilder{
		registry: s,
		opts: prometheus.GaugeOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

func (s *SRouterPrometheusRegistry) NewHistogram() srouter_metrics.HistogramBuilder {
	return &PrometheusHistogramBuilder{
		registry: s,
		opts: prometheus.HistogramOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

func (s *SRouterPrometheusRegistry) NewSummary() srouter_metrics.SummaryBuilder {
	return &PrometheusSummaryBuilder{
		registry: s,
		opts: prometheus.SummaryOpts{
			Namespace: s.namespace,
			Subsystem: s.subsystem,
		},
	}
}

// Register is handled implicitly by the Build methods using registry.Register
func (s *SRouterPrometheusRegistry) Register(m srouter_metrics.Metric) error {
	// In this Prometheus adapter, registration happens during Build().
	// This method could potentially be used to register pre-built collectors
	// if the underlying metric implements prometheus.Collector.
	// For now, it's a no-op as Build() handles registration.
	return nil
}

// Get attempts to retrieve a metric by name. Prometheus client doesn't directly
// support this easily, especially differentiating between metrics with the same
// name but different labels/tags or types. This implementation returns nil, false.
// The application should retain references to the metrics it builds.
func (s *SRouterPrometheusRegistry) Get(name string) (srouter_metrics.Metric, bool) {
	// Prometheus client doesn't provide a reliable Get by name that aligns
	// perfectly with this interface's expectation (especially considering tags/labels
	// and potential type conflicts). Returning not found.
	return nil, false
}

// Unregister attempts to unregister a metric by name.
// NOTE: Prometheus client library makes unregistering by name difficult and potentially unsafe
// if multiple metrics share the same name (e.g., different labels). This implementation
// currently cannot reliably unregister by name only. It's effectively a no-op.
func (s *SRouterPrometheusRegistry) Unregister(name string) bool {
	// Prometheus client requires the actual Collector instance to unregister.
	// Finding the collector solely by name is not directly supported and error-prone.
	// Returning false as we cannot guarantee unregistration by name.
	// A more robust (but complex) solution might involve tracking created metrics internally.
	// s.logger.Warn("Unregistering Prometheus metric by name is not reliably supported", zap.String("name", name))
	return false
}

// Clear attempts to unregister all metrics. Prometheus registry doesn't have a ClearAll.
// We can iterate and unregister, but it's not atomic.
func (s *SRouterPrometheusRegistry) Clear() {
	// Prometheus registry doesn't have a simple ClearAll.
	// Unregistering all requires iterating through registered metrics, which isn't
	// directly exposed. This is generally not a common operation with Prometheus.
	// Log or handle as appropriate if this functionality is critical.
}

// WithTags creates a new registry instance scoped with additional tags (const labels).
func (s *SRouterPrometheusRegistry) WithTags(tags srouter_metrics.Tags) srouter_metrics.MetricsRegistry {
	newTags := make(srouter_metrics.Tags)
	// Copy existing tags
	maps.Copy(newTags, s.tags)
	// Add/overwrite with new tags
	maps.Copy(newTags, tags)
	return &SRouterPrometheusRegistry{
		registry:  s.registry,
		namespace: s.namespace,
		subsystem: s.subsystem,
		tags:      newTags,
	}
}
