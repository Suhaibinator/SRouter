package prometheus

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	srouter_metrics "github.com/Suhaibinator/SRouter/pkg/metrics"
)

func TestSRouterPrometheusRegistry_New(t *testing.T) {
	// Test creating a new registry
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	assert.NotNil(t, promRegistry)
	assert.Equal(t, registry, promRegistry.registry)
	assert.Equal(t, "test", promRegistry.namespace)
	assert.Equal(t, "router", promRegistry.subsystem)
	assert.NotNil(t, promRegistry.tags)
}

func TestSRouterPrometheusRegistry_constLabels(t *testing.T) {
	// Test converting tags to Prometheus labels
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Add some tags
	promRegistry.tags = srouter_metrics.Tags{
		"env": "prod",
		"app": "test-app",
	}

	labels := promRegistry.constLabels()
	assert.Equal(t, prometheus.Labels{"env": "prod", "app": "test-app"}, labels)
}

func TestSRouterPrometheusRegistry_WithTags(t *testing.T) {
	// Test WithTags method
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Add initial tags
	promRegistry.tags = srouter_metrics.Tags{
		"env": "prod",
	}

	// Add more tags
	newRegistry := promRegistry.WithTags(srouter_metrics.Tags{
		"app": "test-app",
		"env": "staging", // This should overwrite the existing tag
	})

	// Assert original registry is unchanged
	assert.Equal(t, srouter_metrics.Tags{"env": "prod"}, promRegistry.tags)

	// Assert new registry has merged tags
	promRegistry2, ok := newRegistry.(*SRouterPrometheusRegistry)
	assert.True(t, ok)
	assert.Equal(t, srouter_metrics.Tags{"env": "staging", "app": "test-app"}, promRegistry2.tags)
}

// Test Counter Builder and Counter
func TestPrometheusCounterBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Create a counter builder
	builder := promRegistry.NewCounter()

	// Test building a counter
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

	// Test incrementing
	counter.Inc()
	counter.Add(5)

	// Verify metric was registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, metricFamilies)

	// Check that we can retrieve the metric
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_requests_total" {
			found = true
			assert.Equal(t, "Total number of requests", mf.GetHelp())
			assert.Equal(t, 1, len(mf.GetMetric()))

			// Verify metric value
			m := mf.GetMetric()[0]
			assert.Equal(t, float64(6), m.GetCounter().GetValue())

			// Verify labels
			for _, label := range m.GetLabel() {
				if label.GetName() == "handler" {
					assert.Equal(t, "test", label.GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Counter metric should be registered and gatherable")

	// Test creating another counter
	anotherCounter := promRegistry.NewCounter().
		Name("another_counter").
		Description("Another counter").
		Tag("service", "api").
		Build()

	assert.NotNil(t, anotherCounter)
	assert.Equal(t, "another_counter", anotherCounter.Name())

	// The adapter handles already registered errors internally,
	// but Prometheus panics if the *same name* is registered with *different* help/labels.
	// We test the internal handling implicitly by registering the same metric again.
	// Re-registering the *exact same* metric should not cause issues.
	builderAgain := promRegistry.NewCounter()
	counterAgain := builderAgain.
		Name("requests_total").
		Description("Total number of requests"). // Must match original description
		Tag("handler", "test").                  // Must match original tags/labels
		Build()
	assert.NotNil(t, counterAgain)

	// Attempting to register with a different description would panic, which is expected Prometheus behavior.
	// We won't explicitly test for the panic here as it's Prometheus's responsibility.
}

// Test Gauge Builder and Gauge
func TestPrometheusGaugeBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Create a gauge builder
	builder := promRegistry.NewGauge()

	// Test building a gauge
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

	// Test gauge operations
	gauge.Set(10)
	gauge.Inc()
	gauge.Add(5)
	gauge.Sub(2)
	gauge.Dec()

	// Verify metric was registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Check that we can retrieve the metric
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_active_requests" {
			found = true
			assert.Equal(t, "Number of active requests", mf.GetHelp())
			assert.Equal(t, 1, len(mf.GetMetric()))

			// Verify metric value (should be 10+1+5-2-1 = 13)
			m := mf.GetMetric()[0]
			assert.Equal(t, float64(13), m.GetGauge().GetValue())

			// Verify labels
			for _, label := range m.GetLabel() {
				if label.GetName() == "pool" {
					assert.Equal(t, "workers", label.GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Gauge metric should be registered and gatherable")

	// Test creating another gauge
	anotherGauge := promRegistry.NewGauge().
		Name("another_gauge").
		Description("Another gauge").
		Tag("service", "api").
		Build()

	assert.NotNil(t, anotherGauge)
	assert.Equal(t, "another_gauge", anotherGauge.Name())

	// The adapter handles already registered errors internally,
	// but Prometheus panics if the *same name* is registered with *different* help/labels.
	// We test the internal handling implicitly by registering the same metric again.
	// Re-registering the *exact same* metric should not cause issues.
	builderAgain := promRegistry.NewGauge()
	gaugeAgain := builderAgain.
		Name("active_requests").
		Description("Number of active requests"). // Must match original description
		Tag("pool", "workers").                   // Must match original tags/labels
		Build()
	assert.NotNil(t, gaugeAgain)

	// Attempting to register with a different description would panic, which is expected Prometheus behavior.
	// We won't explicitly test for the panic here as it's Prometheus's responsibility.
}

// Test Histogram Builder and Histogram
func TestPrometheusHistogramBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Create a histogram builder
	builder := promRegistry.NewHistogram()

	// Test building a histogram with custom buckets
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

	// Test observing values
	histogram.Observe(0.3)
	histogram.Observe(1.5)
	histogram.Observe(7.0)

	// Verify metric was registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Check that we can retrieve the metric
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_request_duration" {
			found = true
			assert.Equal(t, "Request duration in seconds", mf.GetHelp())
			assert.Equal(t, 1, len(mf.GetMetric()))

			// Verify metric has histogram type
			m := mf.GetMetric()[0]
			assert.NotNil(t, m.GetHistogram())

			// Verify sample count is 3
			assert.Equal(t, uint64(3), m.GetHistogram().GetSampleCount())

			// Verify labels
			for _, label := range m.GetLabel() {
				if label.GetName() == "handler" {
					assert.Equal(t, "api", label.GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Histogram metric should be registered and gatherable")

	// Test creating another histogram
	anotherHistogram := promRegistry.NewHistogram().
		Name("another_histogram").
		Description("Another histogram").
		Tag("service", "api").
		Build()

	assert.NotNil(t, anotherHistogram)
	assert.Equal(t, "another_histogram", anotherHistogram.Name())

	// Test histogram with default buckets
	defaultBucketsBuilder := promRegistry.NewHistogram()
	defaultHistogram := defaultBucketsBuilder.
		Name("default_buckets").
		Description("Histogram with default buckets").
		Build()

	assert.NotNil(t, defaultHistogram)

	// The adapter handles already registered errors internally,
	// but Prometheus panics if the *same name* is registered with *different* help/labels.
	// We test the internal handling implicitly by registering the same metric again.
	// Re-registering the *exact same* metric should not cause issues.
	builderAgain := promRegistry.NewHistogram()
	histogramAgain := builderAgain.
		Name("request_duration").
		Description("Request duration in seconds"). // Must match original description
		Tag("handler", "api").                      // Must match original tags/labels
		Buckets(buckets).                           // Must match original buckets
		Build()
	assert.NotNil(t, histogramAgain)

	// Attempting to register with a different description would panic, which is expected Prometheus behavior.
	// We won't explicitly test for the panic here as it's Prometheus's responsibility.
}

// Test Summary Builder and Summary
func TestPrometheusSummaryBuilder(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Create a summary builder
	builder := promRegistry.NewSummary()

	// Test building a summary with custom objectives
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

	// Additional Summary-specific assertions
	promSummary, ok := summary.(*PrometheusSummary)
	assert.True(t, ok)
	assert.Equal(t, objectives, promSummary.objectives)

	// Test observing values
	summary.Observe(0.3)
	summary.Observe(1.5)
	summary.Observe(7.0)

	// Verify metric was registered
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Check that we can retrieve the metric
	var found bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "test_router_request_latency" {
			found = true
			assert.Equal(t, "Request latency in seconds", mf.GetHelp())
			assert.Equal(t, 1, len(mf.GetMetric()))

			// Verify metric has summary type
			m := mf.GetMetric()[0]
			assert.NotNil(t, m.GetSummary())

			// Verify sample count is 3
			assert.Equal(t, uint64(3), m.GetSummary().GetSampleCount())

			// Verify labels
			for _, label := range m.GetLabel() {
				if label.GetName() == "handler" {
					assert.Equal(t, "api", label.GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Summary metric should be registered and gatherable")

	// Test creating another summary
	anotherSummary := promRegistry.NewSummary().
		Name("another_summary").
		Description("Another summary").
		Tag("service", "api").
		Build()

	assert.NotNil(t, anotherSummary)
	assert.Equal(t, "another_summary", anotherSummary.Name())

	// Test summary with default objectives
	defaultObjectivesBuilder := promRegistry.NewSummary()
	defaultSummary := defaultObjectivesBuilder.
		Name("default_objectives").
		Description("Summary with default objectives").
		Build()

	assert.NotNil(t, defaultSummary)

	// Test handling of negative age buckets
	negativeBucketsBuilder := promRegistry.NewSummary()
	negativeBucketsSummary := negativeBucketsBuilder.
		Name("negative_buckets").
		Description("Summary with negative age buckets").
		AgeBuckets(-1).
		Build()

	assert.NotNil(t, negativeBucketsSummary)

	// The adapter handles already registered errors internally,
	// but Prometheus panics if the *same name* is registered with *different* help/labels.
	// We test the internal handling implicitly by registering the same metric again.
	// Re-registering the *exact same* metric should not cause issues.
	builderAgain := promRegistry.NewSummary()
	summaryAgain := builderAgain.
		Name("request_latency").
		Description("Request latency in seconds"). // Must match original description
		Tag("handler", "api").                     // Must match original tags/labels
		Objectives(objectives).                    // Must match original objectives
		MaxAge(10 * time.Second).                  // Must match original MaxAge
		AgeBuckets(5).                             // Must match original AgeBuckets
		Build()
	assert.NotNil(t, summaryAgain)

	// Attempting to register with a different description would panic, which is expected Prometheus behavior.
	// We won't explicitly test for the panic here as it's Prometheus's responsibility.
}

// Test Registry Methods
func TestSRouterPrometheusRegistry_Methods(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Test Register (no-op) - just make sure it doesn't panic
	counter := promRegistry.NewCounter().
		Name("test_counter").
		Description("Test counter").
		Build()

	err := promRegistry.Register(counter)
	assert.NoError(t, err)

	// Test Get (should return nil, false)
	metric, found := promRegistry.Get("test_counter")
	assert.False(t, found)
	assert.Nil(t, metric)

	// Test Unregister (should return false)
	result := promRegistry.Unregister("test_counter")
	assert.False(t, result)

	// Test Clear (no-op) - just make sure it doesn't panic
	promRegistry.Clear()
}

// Test random values and edge cases
func TestRandomValuesAndEdgeCases(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Random test names to ensure uniqueness
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomSuffix := r.Int63()

	// Test gauge with large value
	largeValueGauge := promRegistry.NewGauge().
		Name(fmt.Sprintf("large_value_gauge_%d", randomSuffix)).
		Description("Gauge with large value").
		Build()

	largeValueGauge.Set(1e20)

	// Test histogram with very small observations
	smallValueHistogram := promRegistry.NewHistogram().
		Name(fmt.Sprintf("small_value_histogram_%d", randomSuffix)).
		Description("Histogram with small values").
		Build()

	smallValueHistogram.Observe(1e-10)

	// Test summary with zero observations
	zeroSummary := promRegistry.NewSummary().
		Name(fmt.Sprintf("zero_summary_%d", randomSuffix)).
		Description("Summary with zero values").
		Build()

	zeroSummary.Observe(0)

	// Test registry with many tags
	manyTagsRegistry := promRegistry.WithTags(srouter_metrics.Tags{
		"tag1": "value1",
		"tag2": "value2",
		"tag3": "value3",
		"tag4": "value4",
		"tag5": "value5",
	})

	assert.NotNil(t, manyTagsRegistry)

	// Test metric with many tags
	manyTagsCounter := promRegistry.NewCounter().
		Name(fmt.Sprintf("many_tags_counter_%d", randomSuffix)).
		Description("Counter with many tags").
		Tag("tag1", "value1").
		Tag("tag2", "value2").
		Tag("tag3", "value3").
		Tag("tag4", "value4").
		Tag("tag5", "value5").
		Build()

	assert.NotNil(t, manyTagsCounter)
	assert.Equal(t, 5, len(manyTagsCounter.Tags()))
}

// Test adapter-specific implementation features
func TestPrometheusAdapterSpecifics(t *testing.T) {
	registry := prometheus.NewRegistry()
	promRegistry := NewSRouterPrometheusRegistry(registry, "test", "router")

	// Test WithTags on Gauge type
	gauge := promRegistry.NewGauge().
		Name("test_gauge").
		Description("Test gauge").
		Build()

	// Cast to implementation type to access adapter-specific methods
	promGauge, ok := gauge.(*PrometheusGauge)
	require.True(t, ok, "Should be able to cast to PrometheusGauge")

	newGauge := promGauge.WithTags(srouter_metrics.Tags{"region": "us-west"})
	assert.NotNil(t, newGauge)
	assert.Equal(t, srouter_metrics.Tags{"region": "us-west"}, newGauge.Tags())

	// Test WithTags on Histogram type
	histogram := promRegistry.NewHistogram().
		Name("test_histogram").
		Description("Test histogram").
		Build()

	promHistogram, ok := histogram.(*PrometheusHistogram)
	require.True(t, ok, "Should be able to cast to PrometheusHistogram")

	newHistogram := promHistogram.WithTags(srouter_metrics.Tags{"region": "us-west"})
	assert.NotNil(t, newHistogram)
	assert.Equal(t, srouter_metrics.Tags{"region": "us-west"}, newHistogram.Tags())

	// Test WithTags on Summary type
	summary := promRegistry.NewSummary().
		Name("test_summary").
		Description("Test summary").
		Build()

	promSummary, ok := summary.(*PrometheusSummary)
	require.True(t, ok, "Should be able to cast to PrometheusSummary")

	newSummary := promSummary.WithTags(srouter_metrics.Tags{"region": "us-west"})
	assert.NotNil(t, newSummary)
	assert.Equal(t, srouter_metrics.Tags{"region": "us-west"}, newSummary.Tags())

	// Test getting Objectives from Summary
	objectives := promSummary.Objectives()
	assert.NotNil(t, objectives)

	// Test LabelNames by using the concrete builder types directly
	counterBuilder := &PrometheusCounterBuilder{
		registry: promRegistry,
		opts: prometheus.CounterOpts{
			Namespace: promRegistry.namespace,
			Subsystem: promRegistry.subsystem,
			Name:      "labeled_counter",
			Help:      "Counter with labels",
		},
	}

	counterWithLabels := counterBuilder.
		LabelNames("method", "path").
		Build()

	assert.NotNil(t, counterWithLabels)

	// Test BufCap on SummaryBuilder by creating the builder directly
	summaryBuilder := &PrometheusSummaryBuilder{
		registry: promRegistry,
		opts: prometheus.SummaryOpts{
			Namespace: promRegistry.namespace,
			Subsystem: promRegistry.subsystem,
			Name:      "summary_with_bufcap",
			Help:      "Summary with buffer capacity",
		},
	}

	summaryWithBufCap := summaryBuilder.
		BufCap(100).
		Build()

	assert.NotNil(t, summaryWithBufCap)

	// Test AlreadyRegisteredError handling for CounterVec
	// Manually register a counter vec first
	manualCounterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: promRegistry.namespace,
			Subsystem: promRegistry.subsystem,
			Name:      "already_registered_counter_vec",
			Help:      "Manual counter vec",
		},
		[]string{"status"},
	)
	err := registry.Register(manualCounterVec)
	require.NoError(t, err, "Manual registration should succeed")

	// Now try to build the same counter vec via the adapter
	adapterCounterBuilderInterface := promRegistry.NewCounter()
	adapterCounterBuilder := adapterCounterBuilderInterface.(*PrometheusCounterBuilder) // Cast to concrete type
	adapterCounterBuilder.Name("already_registered_counter_vec")
	adapterCounterBuilder.Description("Manual counter vec") // Help must match
	adapterCounterBuilder.LabelNames("status")              // Call LabelNames on concrete type
	adapterCounter := adapterCounterBuilder.Build()

	// Build should succeed by reusing the existing collector
	assert.NotNil(t, adapterCounter, "Build should succeed even if already registered")

	// Verify it's the same underlying collector (optional, based on Prometheus client internals)
	// This part might be brittle depending on Prometheus client implementation details
	promCounterAdapter, ok := adapterCounter.(*PrometheusCounter)
	require.True(t, ok)
	assert.Equal(t, manualCounterVec, promCounterAdapter.metricVec, "Should reuse the manually registered vec")

	// Test AlreadyRegisteredError handling for other metric types (similar pattern)
	// GaugeVec
	manualGaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_gauge_vec"}, []string{"type"})
	registry.MustRegister(manualGaugeVec)
	adapterGaugeBuilderInterface := promRegistry.NewGauge()
	adapterGaugeBuilder := adapterGaugeBuilderInterface.(*PrometheusGaugeBuilder) // Cast to concrete type
	adapterGaugeBuilder.Name("already_reg_gauge_vec")
	adapterGaugeBuilder.LabelNames("type") // Call LabelNames on concrete type
	adapterGauge := adapterGaugeBuilder.Build()
	assert.NotNil(t, adapterGauge)
	promGaugeAdapter, ok := adapterGauge.(*PrometheusGauge)
	require.True(t, ok)
	assert.Equal(t, manualGaugeVec, promGaugeAdapter.metricVec)

	// HistogramVec
	manualHistoVec := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_histo_vec"}, []string{"code"})
	registry.MustRegister(manualHistoVec)
	adapterHistoBuilderInterface := promRegistry.NewHistogram()
	adapterHistoBuilder := adapterHistoBuilderInterface.(*PrometheusHistogramBuilder) // Cast to concrete type
	adapterHistoBuilder.Name("already_reg_histo_vec")
	adapterHistoBuilder.LabelNames("code") // Call LabelNames on concrete type
	adapterHisto := adapterHistoBuilder.Build()
	assert.NotNil(t, adapterHisto)
	promHistoAdapter, ok := adapterHisto.(*PrometheusHistogram)
	require.True(t, ok)
	assert.Equal(t, manualHistoVec, promHistoAdapter.metricVec)

	// SummaryVec
	manualSummaryVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_summary_vec"}, []string{"path"})
	registry.MustRegister(manualSummaryVec)
	adapterSummaryBuilderInterface := promRegistry.NewSummary()
	adapterSummaryBuilder := adapterSummaryBuilderInterface.(*PrometheusSummaryBuilder) // Cast to concrete type
	adapterSummaryBuilder.Name("already_reg_summary_vec")
	adapterSummaryBuilder.LabelNames("path") // Call LabelNames on concrete type
	adapterSummary := adapterSummaryBuilder.Build()
	assert.NotNil(t, adapterSummary)
	promSummaryAdapter, ok := adapterSummary.(*PrometheusSummary)
	require.True(t, ok)
	assert.Equal(t, manualSummaryVec, promSummaryAdapter.metricVec)

	// Test AlreadyRegisteredError for non-vector metrics
	manualCounter := prometheus.NewCounter(prometheus.CounterOpts{Namespace: "test", Subsystem: "router", Name: "already_reg_counter"})
	registry.MustRegister(manualCounter)
	adapterCounterNonVec := promRegistry.NewCounter().Name("already_reg_counter").Build()
	assert.NotNil(t, adapterCounterNonVec)
	promCounterAdapterNonVec, ok := adapterCounterNonVec.(*PrometheusCounter)
	require.True(t, ok)
	assert.Equal(t, manualCounter, promCounterAdapterNonVec.metric)

	// Note: Testing the 'else' path where err is not AlreadyRegisteredError is difficult
	// as MustRegister usually panics on other errors. We rely on Prometheus's behavior here.
}
