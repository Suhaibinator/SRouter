package prometheus

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srouter_metrics "github.com/Suhaibinator/SRouter/pkg/metrics"
)

// --- Mock Prometheus Registerer for Error Injection ---

// mockPrometheusRegisterer satisfies the prometheus.Registerer interface.
type mockPrometheusRegisterer struct {
	registered      map[string]prometheus.Collector
	registerError   error // Error to return from Register
	mustRegisterErr error // Error to simulate panic in MustRegister (optional)
}

func newMockPrometheusRegisterer() *mockPrometheusRegisterer {
	return &mockPrometheusRegisterer{
		registered: make(map[string]prometheus.Collector),
	}
}

// Helper to get FQName from collector description
func getFQName(c prometheus.Collector) string {
	descChan := make(chan *prometheus.Desc, 1)
	c.Describe(descChan)
	desc := <-descChan
	// A more robust mock might parse the Desc properly to build FQName.
	// Using String() might include variable label info, making it less reliable as a unique key.
	// For simplicity in testing specific error paths, we'll use it carefully.
	return desc.String()
}

func (m *mockPrometheusRegisterer) Register(c prometheus.Collector) error {
	if m.registerError != nil {
		return m.registerError // Inject specific error
	}
	fqName := getFQName(c)
	if _, exists := m.registered[fqName]; exists {
		// Return AlreadyRegisteredError only if it's truly the same collector instance for simplicity
		// A real registry checks descriptor equality.
		if m.registered[fqName] == c {
			return prometheus.AlreadyRegisteredError{ExistingCollector: m.registered[fqName]}
		}
		// Simulate a conflict if FQName matches but collector is different (e.g., different help text)
		return fmt.Errorf("mock registry conflict for %s", fqName)

	}
	m.registered[fqName] = c
	return nil
}

func (m *mockPrometheusRegisterer) MustRegister(cs ...prometheus.Collector) {
	if m.mustRegisterErr != nil {
		panic(m.mustRegisterErr) // Simulate panic from MustRegister
	}
	for _, c := range cs {
		if err := m.Register(c); err != nil {
			// Simulate Prometheus panic for non-AlreadyRegisteredError
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				panic(err) // Panic on any error other than AlreadyRegisteredError
			}
		}
	}
}

func (m *mockPrometheusRegisterer) Unregister(collector prometheus.Collector) bool {
	fqName := getFQName(collector)
	_, exists := m.registered[fqName]
	if exists {
		delete(m.registered, fqName)
	}
	return exists
}

// Gather implements prometheus.Gatherer (optional, for completeness if needed later)
// Note: Our mock only implements Registerer, so this wouldn't be used unless the mock is extended.
func (m *mockPrometheusRegisterer) Gather() ([]*dto.MetricFamily, error) {
	return []*dto.MetricFamily{}, nil
}

// --- Original Tests (Updated for Registerer Interface) ---

func TestPrometheusRegistry_New(t *testing.T) {
	registry := prometheus.NewRegistry() // Real registry satisfies Registerer
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	assert.NotNil(t, promRegistry)
	// Cannot directly compare registry field as it's an interface now.
	// assert.Equal(t, registry, promRegistry.registry)
	assert.Equal(t, "test", promRegistry.namespace)
	assert.Equal(t, "router", promRegistry.subsystem)
	assert.NotNil(t, promRegistry.tags)

	// Test nil registry panic
	assert.Panics(t, func() {
		NewPrometheusRegistry(nil, "test", "nil")
	}, "Should panic if registry is nil")
}

func TestPrometheusRegistry_constLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	promRegistry.tags = srouter_metrics.Tags{"env": "prod", "app": "test-app"}
	labels := promRegistry.constLabels()
	assert.Equal(t, prometheus.Labels{"env": "prod", "app": "test-app"}, labels)
}

func TestPrometheusRegistry_WithTags(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	promRegistry.tags = srouter_metrics.Tags{"env": "prod"}

	newRegistry := promRegistry.WithTags(srouter_metrics.Tags{"app": "test-app", "env": "staging"})

	assert.Equal(t, srouter_metrics.Tags{"env": "prod"}, promRegistry.tags) // Original unchanged

	promRegistry2, ok := newRegistry.(*PrometheusRegistry)
	require.True(t, ok)
	assert.Equal(t, srouter_metrics.Tags{"env": "staging", "app": "test-app"}, promRegistry2.tags) // Merged/overwritten
	assert.Equal(t, registry, promRegistry2.registry, "Registry instance should be shared")        // Ensure registry is passed
}

// Test Counter Builder and Counter
func TestPrometheusCounterBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	builder := promRegistry.NewCounter()

	counter := builder.
		Name("requests_total").
		Description("Total number of requests").
		Tag("handler", "test").
		Build()

	assert.NotNil(t, counter)
	assert.Equal(t, "requests_total", counter.Name())
	assert.Equal(t, "Total number of requests", counter.Description())
	assert.Equal(t, srouter_metrics.CounterType, counter.Type())
	assert.Equal(t, srouter_metrics.Tags{"handler": "test"}, counter.Tags())

	counter.Inc()
	counter.Add(5)

	metricFamilies, err := registry.Gather() // Gather from the real registry
	require.NoError(t, err)
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_requests_total" {
			found = true
			require.Equal(t, 1, len(mf.GetMetric()))
			m := mf.GetMetric()[0]
			assert.Equal(t, float64(6), m.GetCounter().GetValue())
			require.Equal(t, 1, len(m.GetLabel()))
			assert.Equal(t, "handler", m.GetLabel()[0].GetName())
			assert.Equal(t, "test", m.GetLabel()[0].GetValue())
		}
	}
	assert.True(t, found, "Counter metric not found")

	// Test re-registering the exact same metric (should reuse)
	builderAgain := promRegistry.NewCounter()
	counterAgain := builderAgain.
		Name("requests_total").
		Description("Total number of requests").
		Tag("handler", "test").
		Build()
	assert.NotNil(t, counterAgain)
	// Verify it's the same underlying metric (optional check)
	promCounter1, ok1 := counter.(*PrometheusCounter)
	promCounter2, ok2 := counterAgain.(*PrometheusCounter)
	if ok1 && ok2 {
		assert.Equal(t, promCounter1.metric, promCounter2.metric)
	}
}

// Test Gauge Builder and Gauge
func TestPrometheusGaugeBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	builder := promRegistry.NewGauge()

	gauge := builder.
		Name("active_requests").
		Description("Number of active requests").
		Tag("pool", "workers").
		Build()

	assert.NotNil(t, gauge)
	assert.Equal(t, "active_requests", gauge.Name())
	assert.Equal(t, "Number of active requests", gauge.Description())
	assert.Equal(t, srouter_metrics.GaugeType, gauge.Type())
	assert.Equal(t, srouter_metrics.Tags{"pool": "workers"}, gauge.Tags())

	gauge.Set(10)
	gauge.Inc()
	gauge.Add(5)
	gauge.Sub(2)
	gauge.Dec() // 10 + 1 + 5 - 2 - 1 = 13

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_active_requests" {
			found = true
			require.Equal(t, 1, len(mf.GetMetric()))
			m := mf.GetMetric()[0]
			assert.Equal(t, float64(13), m.GetGauge().GetValue())
			require.Equal(t, 1, len(m.GetLabel()))
			assert.Equal(t, "pool", m.GetLabel()[0].GetName())
			assert.Equal(t, "workers", m.GetLabel()[0].GetValue())
		}
	}
	assert.True(t, found, "Gauge metric not found")

	// Test re-registering the exact same metric (should reuse)
	builderAgain := promRegistry.NewGauge()
	gaugeAgain := builderAgain.
		Name("active_requests").
		Description("Number of active requests").
		Tag("pool", "workers").
		Build()
	assert.NotNil(t, gaugeAgain)
}

// Test Histogram Builder and Histogram
func TestPrometheusHistogramBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	builder := promRegistry.NewHistogram()
	buckets := []float64{0.1, 0.5, 1, 2, 5, 10}

	histogram := builder.
		Name("request_duration").
		Description("Request duration in seconds").
		Tag("handler", "api").
		Buckets(buckets).
		Build()

	assert.NotNil(t, histogram)
	assert.Equal(t, "request_duration", histogram.Name())
	assert.Equal(t, "Request duration in seconds", histogram.Description())
	assert.Equal(t, srouter_metrics.HistogramType, histogram.Type())
	assert.Equal(t, srouter_metrics.Tags{"handler": "api"}, histogram.Tags())

	histogram.Observe(0.3)
	histogram.Observe(1.5)
	histogram.Observe(7.0)

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_request_duration" {
			found = true
			require.Equal(t, 1, len(mf.GetMetric()))
			m := mf.GetMetric()[0]
			assert.NotNil(t, m.GetHistogram())
			assert.Equal(t, uint64(3), m.GetHistogram().GetSampleCount())
			require.Equal(t, 1, len(m.GetLabel()))
			assert.Equal(t, "handler", m.GetLabel()[0].GetName())
			assert.Equal(t, "api", m.GetLabel()[0].GetValue())
		}
	}
	assert.True(t, found, "Histogram metric not found")

	// Test re-registering the exact same metric (should reuse)
	builderAgain := promRegistry.NewHistogram()
	histogramAgain := builderAgain.
		Name("request_duration").
		Description("Request duration in seconds").
		Tag("handler", "api").
		Buckets(buckets).
		Build()
	assert.NotNil(t, histogramAgain)
}

// Test Summary Builder and Summary
func TestPrometheusSummaryBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	builder := promRegistry.NewSummary()
	objectives := map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}

	summary := builder.
		Name("request_latency").
		Description("Request latency in seconds").
		Tag("handler", "api").
		Objectives(objectives).
		MaxAge(10 * time.Second).
		AgeBuckets(5).
		Build()

	assert.NotNil(t, summary)
	assert.Equal(t, "request_latency", summary.Name())
	assert.Equal(t, "Request latency in seconds", summary.Description())
	assert.Equal(t, srouter_metrics.SummaryType, summary.Type())
	assert.Equal(t, srouter_metrics.Tags{"handler": "api"}, summary.Tags())

	promSummary, ok := summary.(*PrometheusSummary)
	require.True(t, ok)
	assert.Equal(t, objectives, promSummary.objectives)

	summary.Observe(0.3)
	summary.Observe(1.5)
	summary.Observe(7.0)

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_request_latency" {
			found = true
			require.Equal(t, 1, len(mf.GetMetric()))
			m := mf.GetMetric()[0]
			assert.NotNil(t, m.GetSummary())
			assert.Equal(t, uint64(3), m.GetSummary().GetSampleCount())
			require.Equal(t, 1, len(m.GetLabel()))
			assert.Equal(t, "handler", m.GetLabel()[0].GetName())
			assert.Equal(t, "api", m.GetLabel()[0].GetValue())
		}
	}
	assert.True(t, found, "Summary metric not found")

	// Test re-registering the exact same metric (should reuse)
	builderAgain := promRegistry.NewSummary()
	summaryAgain := builderAgain.
		Name("request_latency").
		Description("Request latency in seconds").
		Tag("handler", "api").
		Objectives(objectives).
		MaxAge(10 * time.Second).
		AgeBuckets(5).
		Build()
	assert.NotNil(t, summaryAgain)
}

// Test Registry Methods (using real registry)
func TestPrometheusRegistry_Methods(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Register is a no-op in the adapter, but test it doesn't error
	counter := promRegistry.NewCounter().Name("reg_test_counter").Build()
	err := promRegistry.Register(counter) // This uses the adapter's Register
	assert.NoError(t, err)

	// Get is not reliably supported by Prometheus client by name only
	metric, found := promRegistry.Get("reg_test_counter")
	assert.False(t, found)
	assert.Nil(t, metric)

	// Unregister is not reliably supported by name only
	result := promRegistry.Unregister("reg_test_counter")
	assert.False(t, result)

	// Clear is a no-op
	promRegistry.Clear() // Just ensure it doesn't panic
}

// Test random values and edge cases (using real registry)
func TestRandomValuesAndEdgeCases(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomSuffix := r.Int63()

	// Gauge with large value
	largeValueGauge := promRegistry.NewGauge().
		Name(fmt.Sprintf("large_value_gauge_%d", randomSuffix)).
		Build()
	largeValueGauge.Set(1e20)
	assert.NotNil(t, largeValueGauge)

	// Histogram with very small observations
	smallValueHistogram := promRegistry.NewHistogram().
		Name(fmt.Sprintf("small_value_histogram_%d", randomSuffix)).
		Build()
	smallValueHistogram.Observe(1e-10)
	assert.NotNil(t, smallValueHistogram)

	// Summary with zero observations
	zeroSummary := promRegistry.NewSummary().
		Name(fmt.Sprintf("zero_summary_%d", randomSuffix)).
		Build()
	zeroSummary.Observe(0)
	assert.NotNil(t, zeroSummary)

	// Registry with many tags
	manyTagsRegistry := promRegistry.WithTags(srouter_metrics.Tags{
		"tag1": "v1", "tag2": "v2", "tag3": "v3", "tag4": "v4", "tag5": "v5",
	})
	assert.NotNil(t, manyTagsRegistry)

	// Metric with many tags
	manyTagsCounter := manyTagsRegistry.NewCounter().
		Name(fmt.Sprintf("many_tags_counter_%d", randomSuffix)).
		Tag("tag6", "v6").Tag("tag7", "v7").Tag("tag8", "v8").
		Build()
	assert.NotNil(t, manyTagsCounter)
	// Tags should include registry tags + builder tags
	assert.GreaterOrEqual(t, len(manyTagsCounter.Tags()), 8)
}

// Test CounterVecNoOp verifies that counter operations on a vec-based counter
// without labels are properly handled as no-ops.
func TestCounterVecNoOp(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Setup counter vec
	counterBuilderInterface := promRegistry.NewCounter()
	counterBuilder := counterBuilderInterface.(*PrometheusCounterBuilder)
	counterBuilder.Name("vec_noop_counter")
	counterBuilder.LabelNames("label") // This forces it to be a vec
	counterVec := counterBuilder.Build()

	// Before operations, gather metrics to capture the initial state
	metricFamiliesBefore, err := registry.Gather()
	require.NoError(t, err)

	// Test Inc - should be a no-op on vec without labels
	counterVec.Inc()

	// After Inc, gather metrics to see if anything changed
	metricFamiliesAfter, err := registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical before and after the no-op Inc
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Inc on counter vec")

	// Test Add - should be a no-op on vec without labels
	counterVec.Add(10.0)

	// After Add, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Add on counter vec")
}

// Test GaugeVecNoOp verifies that gauge operations on a vec-based gauge
// without labels are properly handled as no-ops.
func TestGaugeVecNoOp(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Setup gauge vec
	gaugeBuilderInterface := promRegistry.NewGauge()
	gaugeBuilder := gaugeBuilderInterface.(*PrometheusGaugeBuilder)
	gaugeBuilder.Name("vec_noop_gauge")
	gaugeBuilder.LabelNames("label") // This forces it to be a vec
	gaugeVec := gaugeBuilder.Build()

	// Before operations, gather metrics to capture the initial state
	metricFamiliesBefore, err := registry.Gather()
	require.NoError(t, err)

	// Test Set - should be a no-op on vec without labels
	gaugeVec.Set(42.0)

	// After Set, gather metrics to see if anything changed
	metricFamiliesAfter, err := registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical before and after the no-op Set
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Set on gauge vec")

	// Test Inc - should be a no-op on vec without labels
	gaugeVec.Inc()

	// After Inc, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Inc on gauge vec")

	// Test Dec - should be a no-op on vec without labels
	gaugeVec.Dec()

	// After Dec, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Dec on gauge vec")

	// Test Add - should be a no-op on vec without labels
	gaugeVec.Add(10.0)

	// After Add, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Add on gauge vec")

	// Test Sub - should be a no-op on vec without labels
	gaugeVec.Sub(5.0)

	// After Sub, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Sub on gauge vec")
}

// Test VecObserveNoOp verifies that calling Observe directly on a vec-based
// histogram or summary (which would require labels) is a no-op.
func TestVecObserveNoOp(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Setup histogram vec
	histoBuilderInterface := promRegistry.NewHistogram()
	histoBuilder := histoBuilderInterface.(*PrometheusHistogramBuilder)
	histoBuilder.Name("vec_noop_histogram")
	histoBuilder.LabelNames("label") // This forces it to be a vec
	histogramVec := histoBuilder.Build()

	// Before observation, gather metrics to capture the initial state
	metricFamiliesBefore, err := registry.Gather()
	require.NoError(t, err)

	// Call Observe directly on the vec-based histogram (should be a no-op)
	histogramVec.Observe(42.0)

	// After observation, gather metrics to see if anything changed
	metricFamiliesAfter, err := registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical before and after the no-op observation
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Observe on histogram vec")

	// Setup summary vec
	summaryBuilderInterface := promRegistry.NewSummary()
	summaryBuilder := summaryBuilderInterface.(*PrometheusSummaryBuilder)
	summaryBuilder.Name("vec_noop_summary")
	summaryBuilder.LabelNames("label") // This forces it to be a vec
	summaryVec := summaryBuilder.Build()

	// Before observation, gather metrics again
	metricFamiliesBefore, err = registry.Gather()
	require.NoError(t, err)

	// Call Observe directly on the vec-based summary (should be a no-op)
	summaryVec.Observe(42.0)

	// After observation, gather metrics to see if anything changed
	metricFamiliesAfter, err = registry.Gather()
	require.NoError(t, err)

	// The metrics should be identical before and after the no-op observation
	assert.Equal(t, metricFamiliesBefore, metricFamiliesAfter,
		"Metrics should be identical before and after no-op Observe on summary vec")
}

// Test adapter-specific implementation features (using real registry where needed)
func TestPrometheusAdapterSpecifics(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Test WithTags on concrete types
	gauge := promRegistry.NewGauge().Name("specifics_gauge").Build()
	promGauge, ok := gauge.(*PrometheusGauge)
	require.True(t, ok)
	newGauge := promGauge.WithTags(srouter_metrics.Tags{"region": "us-west"})
	assert.NotNil(t, newGauge)
	assert.Equal(t, srouter_metrics.Tags{"region": "us-west"}, newGauge.Tags())

	histogram := promRegistry.NewHistogram().Name("specifics_histogram").Build()
	promHistogram, ok := histogram.(*PrometheusHistogram)
	require.True(t, ok)
	newHistogram := promHistogram.WithTags(srouter_metrics.Tags{"region": "eu-central"})
	assert.NotNil(t, newHistogram)
	assert.Equal(t, srouter_metrics.Tags{"region": "eu-central"}, newHistogram.Tags())

	summary := promRegistry.NewSummary().Name("specifics_summary").Build()
	promSummary, ok := summary.(*PrometheusSummary)
	require.True(t, ok)
	newSummary := promSummary.WithTags(srouter_metrics.Tags{"env": "prod"})
	assert.NotNil(t, newSummary)
	assert.Equal(t, srouter_metrics.Tags{"env": "prod"}, newSummary.Tags())

	// Test getting Objectives from Summary
	objectives := promSummary.Objectives() // From concrete type
	assert.NotNil(t, objectives)           // Default objectives should exist

	// Test LabelNames on concrete builder types
	counterBuilderInterface := promRegistry.NewCounter()
	counterBuilder := counterBuilderInterface.(*PrometheusCounterBuilder) // Cast
	counterBuilder.Name("specifics_labeled_counter")                      // Call interface methods first
	counterBuilder.LabelNames("method", "path")                           // Call specific method
	counterWithLabels := counterBuilder.Build()
	assert.NotNil(t, counterWithLabels)

	// Test BufCap on concrete SummaryBuilder
	summaryBuilderInterface := promRegistry.NewSummary()
	summaryBuilder := summaryBuilderInterface.(*PrometheusSummaryBuilder) // Cast
	summaryBuilder.Name("specifics_summary_bufcap")                       // Call interface methods first
	summaryBuilder.BufCap(100)                                            // Call specific method
	summaryWithBufCap := summaryBuilder.Build()
	assert.NotNil(t, summaryWithBufCap)

	// --- Test Tag Merging Precedence ---
	registryWithTags := prometheus.NewRegistry()
	promRegistryWithTags := NewPrometheusRegistry(registryWithTags, "test", "router").
		WithTags(srouter_metrics.Tags{"registry_tag": "registry_value", "overlap_tag": "registry_overlap"}).(*PrometheusRegistry)

	counterBuilderMerged := promRegistryWithTags.NewCounter().(*PrometheusCounterBuilder)
	counterMerged := counterBuilderMerged.
		Name("merged_counter").
		Tag("builder_tag", "builder_value").
		Tag("overlap_tag", "builder_overlap"). // This should win
		Build()
	expectedCounterTags := srouter_metrics.Tags{"registry_tag": "registry_value", "builder_tag": "builder_value", "overlap_tag": "builder_overlap"}
	assert.Equal(t, expectedCounterTags, counterMerged.Tags())

	gaugeBuilderMerged := promRegistryWithTags.NewGauge().(*PrometheusGaugeBuilder)
	gaugeMerged := gaugeBuilderMerged.
		Name("merged_gauge").
		Tag("builder_tag", "builder_value").
		Tag("overlap_tag", "builder_overlap"). // This should win
		Build()
	expectedGaugeTags := srouter_metrics.Tags{"registry_tag": "registry_value", "builder_tag": "builder_value", "overlap_tag": "builder_overlap"}
	assert.Equal(t, expectedGaugeTags, gaugeMerged.Tags())

	// --- Test AlreadyRegisteredError handling (using real registry) ---

	// CounterVec
	manualCounterVec := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_counter_vec"}, []string{"status"})
	registry.MustRegister(manualCounterVec) // Register in the *real* registry
	adapterCounterBuilderInterface := promRegistry.NewCounter()
	adapterCounterBuilder := adapterCounterBuilderInterface.(*PrometheusCounterBuilder)
	adapterCounterBuilder.Name("already_reg_counter_vec")
	adapterCounterBuilder.Description("") // Description must match if reusing! Let's assume empty for test simplicity or fetch original.
	adapterCounterBuilder.LabelNames("status")
	adapterCounter := adapterCounterBuilder.Build()
	assert.NotNil(t, adapterCounter)
	promCounterAdapter, ok := adapterCounter.(*PrometheusCounter)
	require.True(t, ok)
	assert.Equal(t, manualCounterVec, promCounterAdapter.metricVec)

	// GaugeVec
	manualGaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_gauge_vec"}, []string{"type"})
	registry.MustRegister(manualGaugeVec)
	adapterGaugeBuilderInterface := promRegistry.NewGauge()
	adapterGaugeBuilder := adapterGaugeBuilderInterface.(*PrometheusGaugeBuilder)
	adapterGaugeBuilder.Name("already_reg_gauge_vec")
	adapterGaugeBuilder.LabelNames("type")
	adapterGauge := adapterGaugeBuilder.Build()
	assert.NotNil(t, adapterGauge)
	promGaugeAdapter, ok := adapterGauge.(*PrometheusGauge)
	require.True(t, ok)
	assert.Equal(t, manualGaugeVec, promGaugeAdapter.metricVec)

	// HistogramVec
	manualHistoVec := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_histo_vec"}, []string{"code"})
	registry.MustRegister(manualHistoVec)
	adapterHistoBuilderInterface := promRegistry.NewHistogram()
	adapterHistoBuilder := adapterHistoBuilderInterface.(*PrometheusHistogramBuilder)
	adapterHistoBuilder.Name("already_reg_histo_vec")
	adapterHistoBuilder.LabelNames("code")
	adapterHisto := adapterHistoBuilder.Build()
	assert.NotNil(t, adapterHisto)
	promHistoAdapter, ok := adapterHisto.(*PrometheusHistogram)
	require.True(t, ok)
	assert.Equal(t, manualHistoVec, promHistoAdapter.metricVec)

	// SummaryVec
	manualSummaryVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_summary_vec"}, []string{"path"})
	registry.MustRegister(manualSummaryVec)
	adapterSummaryBuilderInterface := promRegistry.NewSummary()
	adapterSummaryBuilder := adapterSummaryBuilderInterface.(*PrometheusSummaryBuilder)
	adapterSummaryBuilder.Name("already_reg_summary_vec")
	adapterSummaryBuilder.LabelNames("path")
	adapterSummary := adapterSummaryBuilder.Build()
	assert.NotNil(t, adapterSummary)
	promSummaryAdapter, ok := adapterSummary.(*PrometheusSummary)
	require.True(t, ok)
	assert.Equal(t, manualSummaryVec, promSummaryAdapter.metricVec)

	// Non-vector Counter
	manualCounter := prometheus.NewCounter(prometheus.CounterOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_counter"})
	registry.MustRegister(manualCounter)
	adapterCounterNonVec := promRegistry.NewCounter().Name("already_reg_counter").Build()
	assert.NotNil(t, adapterCounterNonVec)
	promCounterAdapterNonVec, ok := adapterCounterNonVec.(*PrometheusCounter)
	require.True(t, ok)
	assert.Equal(t, manualCounter, promCounterAdapterNonVec.metric)
}

// Test for registry tags being applied correctly when not overridden by builder tags
func TestRegistryConstLabelsApplied(t *testing.T) {
	registry := prometheus.NewRegistry()

	// Create a registry with predefined tags
	promRegistry := NewPrometheusRegistry(registry, "test", "router")
	promRegistry.tags = srouter_metrics.Tags{
		"registry_tag1": "registry_value1",
		"common_tag":    "registry_value", // This one will be overridden by builder
	}

	// Create a counter with some tags that overlap and some that don't
	counter := promRegistry.NewCounter().
		Name("const_label_test").
		Tag("builder_tag1", "builder_value1").
		Tag("common_tag", "builder_value"). // This should take precedence
		Build()

	// Verify the tags contain both registry and builder tags, with builder taking precedence for common keys
	expectedTags := srouter_metrics.Tags{
		"registry_tag1": "registry_value1", // From registry
		"builder_tag1":  "builder_value1",  // From builder
		"common_tag":    "builder_value",   // Builder value overrides registry
	}
	assert.Equal(t, expectedTags, counter.Tags())

	// Verify the same behavior for other metric types
	gauge := promRegistry.NewGauge().
		Name("const_label_gauge_test").
		Tag("builder_tag1", "builder_value1").
		Tag("common_tag", "builder_value").
		Build()
	assert.Equal(t, expectedTags, gauge.Tags())

	histogram := promRegistry.NewHistogram().
		Name("const_label_histo_test").
		Tag("builder_tag1", "builder_value1").
		Tag("common_tag", "builder_value").
		Build()
	assert.Equal(t, expectedTags, histogram.Tags())

	summary := promRegistry.NewSummary().
		Name("const_label_summary_test").
		Tag("builder_tag1", "builder_value1").
		Tag("common_tag", "builder_value").
		Build()
	assert.Equal(t, expectedTags, summary.Tags())
}

// Test for proper conversion of ConstLabels to Tags
func TestConstLabelsToTagsConversion(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Test with Counter
	counterBuilder := promRegistry.NewCounter().(*PrometheusCounterBuilder)
	// Set const labels directly on the opts
	if counterBuilder.opts.ConstLabels == nil {
		counterBuilder.opts.ConstLabels = make(prometheus.Labels)
	}
	counterBuilder.opts.ConstLabels["direct_label"] = "direct_value"
	counterBuilder.Name("conversion_counter")
	counter := counterBuilder.Build()

	// Verify the direct label was converted to a tag
	assert.Equal(t, "direct_value", counter.Tags()["direct_label"])

	// Test with Gauge
	gaugeBuilder := promRegistry.NewGauge().(*PrometheusGaugeBuilder)
	if gaugeBuilder.opts.ConstLabels == nil {
		gaugeBuilder.opts.ConstLabels = make(prometheus.Labels)
	}
	gaugeBuilder.opts.ConstLabels["direct_label"] = "direct_value"
	gaugeBuilder.Name("conversion_gauge")
	gauge := gaugeBuilder.Build()
	assert.Equal(t, "direct_value", gauge.Tags()["direct_label"])

	// Test with Histogram
	histoBuilder := promRegistry.NewHistogram().(*PrometheusHistogramBuilder)
	if histoBuilder.opts.ConstLabels == nil {
		histoBuilder.opts.ConstLabels = make(prometheus.Labels)
	}
	histoBuilder.opts.ConstLabels["direct_label"] = "direct_value"
	histoBuilder.Name("conversion_histogram")
	histogram := histoBuilder.Build()
	assert.Equal(t, "direct_value", histogram.Tags()["direct_label"])

	// Test with Summary
	summaryBuilder := promRegistry.NewSummary().(*PrometheusSummaryBuilder)
	if summaryBuilder.opts.ConstLabels == nil {
		summaryBuilder.opts.ConstLabels = make(prometheus.Labels)
	}
	summaryBuilder.opts.ConstLabels["direct_label"] = "direct_value"
	summaryBuilder.Name("conversion_summary")
	summary := summaryBuilder.Build()
	assert.Equal(t, "direct_value", summary.Tags()["direct_label"])
}

// Test for handling negative AgeBuckets value
func TestNegativeAgeBuckets(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewPrometheusRegistry(registry, "test", "router")

	// Create a summary builder and set a negative AgeBuckets value
	summaryBuilder := promRegistry.NewSummary().(*PrometheusSummaryBuilder)
	summaryBuilder.Name("negative_agebuckets_summary")
	summaryBuilder.AgeBuckets(-5) // Negative value should be converted to 0

	// Build the summary and verify it was created successfully
	summary := summaryBuilder.Build()
	assert.NotNil(t, summary)

	// We can't directly test the AgeBuckets value as it's internal to the Prometheus client
	// But we can verify the Summary was created without error
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Find our summary in the metrics
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_negative_agebuckets_summary" {
			found = true
			break
		}
	}
	assert.True(t, found, "Summary with negative AgeBuckets should be registered successfully")
}

// --- New Test for Panic Paths (using Mock Registerer) ---

func TestPrometheusBuilder_RegisterErrorPanic(t *testing.T) {
	mockRegistry := newMockPrometheusRegisterer()
	genericError := errors.New("generic registration error")
	mockRegistry.registerError = genericError // Configure mock to return a generic error

	// Create adapter instance using the mock registerer
	promRegistry := NewPrometheusRegistry(mockRegistry, "test", "panic_test")

	// Test Counter Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		promRegistry.NewCounter().Name("panic_counter").Build()
	}, "Counter Build should panic with generic error")

	// Test CounterVec Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		builderInterface := promRegistry.NewCounter()
		builder := builderInterface.(*PrometheusCounterBuilder) // Cast
		builder.Name("panic_counter_vec")                       // Call interface methods first
		builder.LabelNames("a")                                 // Call specific method
		builder.Build()
	}, "CounterVec Build should panic with generic error")

	// Test Gauge Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		promRegistry.NewGauge().Name("panic_gauge").Build()
	}, "Gauge Build should panic with generic error")

	// Test GaugeVec Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		builderInterface := promRegistry.NewGauge()
		builder := builderInterface.(*PrometheusGaugeBuilder) // Cast
		builder.Name("panic_gauge_vec")                       // Call interface methods first
		builder.LabelNames("b")                               // Call specific method
		builder.Build()
	}, "GaugeVec Build should panic with generic error")

	// Test Histogram Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		promRegistry.NewHistogram().Name("panic_histogram").Build()
	}, "Histogram Build should panic with generic error")

	// Test HistogramVec Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		builderInterface := promRegistry.NewHistogram()
		builder := builderInterface.(*PrometheusHistogramBuilder) // Cast
		builder.Name("panic_histogram_vec")                       // Call interface methods first
		builder.LabelNames("c")                                   // Call specific method
		builder.Build()
	}, "HistogramVec Build should panic with generic error")

	// Test Summary Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		promRegistry.NewSummary().Name("panic_summary").Build()
	}, "Summary Build should panic with generic error")

	// Test SummaryVec Panic
	assert.PanicsWithError(t, genericError.Error(), func() {
		builderInterface := promRegistry.NewSummary()
		builder := builderInterface.(*PrometheusSummaryBuilder) // Cast
		builder.Name("panic_summary_vec")                       // Call interface methods first
		builder.LabelNames("d")                                 // Call specific method
		builder.Build()
	}, "SummaryVec Build should panic with generic error")
}
