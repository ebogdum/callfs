package telemetry

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func newTestSampler(t *testing.T, registry *prometheus.Registry, interval, retention time.Duration, declared ...FamilyDeclaration) *Sampler {
	t.Helper()
	sampler, err := NewSampler(SamplerConfig{Gatherer: registry, Interval: interval, Retention: retention, Declared: declared})
	if nil != err {
		t.Fatal(err)
	}
	return sampler
}

func registerCollector(t *testing.T, registry *prometheus.Registry, collector prometheus.Collector) {
	t.Helper()
	if err := registry.Register(collector); nil != err {
		t.Fatal(err)
	}
}

func mustSample(t *testing.T, sampler *Sampler, at time.Time) {
	t.Helper()
	if err := sampler.Sample(at); nil != err {
		t.Fatal(err)
	}
}

func pointValues(t *testing.T, points []Point) []float64 {
	t.Helper()
	values := make([]float64, 0, len(points))
	for _, point := range points {
		if nil == point.Value {
			t.Fatalf("point at %d is null", point.Unix)
		}
		values = append(values, *point.Value)
	}
	return values
}

func TestSamplerRateAndTotalFromCounterDeltas(t *testing.T) {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sampler_requests_total", Help: "Requests."}, []string{"method"})
	registerCollector(t, registry, requests)
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour)
	base := time.Unix(1800000000, 0).UTC()

	requests.WithLabelValues("GET").Add(5)
	mustSample(t, sampler, base)
	requests.WithLabelValues("GET").Add(20)
	mustSample(t, sampler, base.Add(10*time.Second))
	requests.WithLabelValues("GET").Add(10)
	mustSample(t, sampler, base.Add(20*time.Second))

	to := base.Add(20 * time.Second)
	total, err := sampler.Series("sampler_requests_total", MeasureTotal, []string{"method"}, nil, base, to, 10*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	if 1 != len(total.Series) || "GET" != total.Series[0].Labels["method"] {
		t.Fatalf("unexpected groups: %+v", total.Series)
	}
	if got := pointValues(t, total.Series[0].Points); !slices.Equal(got, []float64{20, 10}) {
		t.Fatalf("total = %v", got)
	}
	rate, err := sampler.Series("sampler_requests_total", MeasureRate, []string{"method"}, nil, base, to, 10*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	if got := pointValues(t, rate.Series[0].Points); !slices.Equal(got, []float64{2, 1}) {
		t.Fatalf("rate = %v", got)
	}
}

func TestSamplerHistogramQuantilesAndAverageFromBucketDeltas(t *testing.T) {
	registry := prometheus.NewRegistry()
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "sampler_latency_seconds", Help: "Latency.", Buckets: []float64{0.1, 0.5, 1}})
	registerCollector(t, registry, latency)
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour)
	base := time.Unix(1800000000, 0).UTC()

	mustSample(t, sampler, base)
	for range 5 {
		latency.Observe(0.05)
	}
	for range 3 {
		latency.Observe(0.3)
	}
	for range 2 {
		latency.Observe(0.8)
	}
	mustSample(t, sampler, base.Add(10*time.Second))

	expected := map[Measure]float64{
		MeasureP50: 0.1,
		MeasureP95: 0.875,
		MeasureAvg: 0.275,
		MeasureMax: 1,
	}
	for measure, want := range expected {
		result, err := sampler.Series("sampler_latency_seconds", measure, nil, nil, base, base.Add(10*time.Second), 10*time.Second)
		if nil != err {
			t.Fatal(err)
		}
		got := pointValues(t, result.Series[0].Points)
		if 1 != len(got) || math.Abs(got[0]-want) > 1e-9 {
			t.Fatalf("%s = %v, want %v", measure, got, want)
		}
	}
}

func TestSamplerFamiliesMarksFamiliesWithoutSeriesAsNotEmitted(t *testing.T) {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sampler_requests_total", Help: "Requests."}, []string{"method"})
	registerCollector(t, registry, requests)
	requests.WithLabelValues("POST").Inc()
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour, FamilyDeclaration{Name: "sampler_unused_total", Type: "counter", Help: "Never emitted."})
	mustSample(t, sampler, time.Unix(1800000000, 0).UTC())

	families := sampler.Families()
	if 2 != len(families) {
		t.Fatalf("families = %d, want 2", len(families))
	}
	emitted, unused := families[0], families[1]
	if "sampler_requests_total" != emitted.Name || !emitted.Emitted || 1 != len(emitted.Series) || nil == emitted.Series[0].Value || 1 != *emitted.Series[0].Value {
		t.Fatalf("unexpected emitted family: %+v", emitted)
	}
	if "sampler_unused_total" != unused.Name || unused.Emitted || 0 != len(unused.Series) || "counter" != unused.Type {
		t.Fatalf("unexpected unused family: %+v", unused)
	}
	encoded, err := json.Marshal(Point{Unix: 5})
	if nil != err {
		t.Fatal(err)
	}
	if "[5,null]" != string(encoded) {
		t.Fatalf("point json = %s", encoded)
	}
}

func TestSamplerRingKeepsOnlyRetentionWindow(t *testing.T) {
	registry := prometheus.NewRegistry()
	total := prometheus.NewCounter(prometheus.CounterOpts{Name: "sampler_ring_total", Help: "Ring."})
	registerCollector(t, registry, total)
	sampler := newTestSampler(t, registry, 10*time.Second, 30*time.Second)
	base := time.Unix(1800000000, 0).UTC()

	for i := range 5 {
		total.Inc()
		mustSample(t, sampler, base.Add(time.Duration(i)*10*time.Second))
	}
	result, err := sampler.Series("sampler_ring_total", MeasureValue, nil, nil, base, base.Add(50*time.Second), 10*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	if nil == result.SampledSince || !result.SampledSince.Equal(base.Add(20*time.Second)) {
		t.Fatalf("sampled_since = %v", result.SampledSince)
	}
	points := result.Series[0].Points
	if nil != points[0].Value {
		t.Fatalf("point before retention is %v, want null", *points[0].Value)
	}
	if got := pointValues(t, points[1:]); !slices.Equal(got, []float64{3, 4, 5, 5}) {
		t.Fatalf("values = %v", got)
	}
}

func TestSamplerCountsSeriesBornInsideTheWindow(t *testing.T) {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sampler_born_total", Help: "Born."}, []string{"status"})
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "sampler_born_seconds", Help: "Born.", Buckets: []float64{0.1, 1}})
	registerCollector(t, registry, requests)
	registerCollector(t, registry, latency)
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour)
	base := time.Unix(1800000000, 0).UTC()

	requests.WithLabelValues("200").Add(4)
	latency.Observe(0.05)
	mustSample(t, sampler, base.Add(10*time.Second))
	requests.WithLabelValues("200").Add(2)
	requests.WithLabelValues("404").Add(3)
	mustSample(t, sampler, base.Add(20*time.Second))

	total, err := sampler.Series("sampler_born_total", MeasureTotal, []string{"status"}, nil, base.Add(-time.Hour), base.Add(20*time.Second), time.Hour+20*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, line := range total.Series {
		got[line.Labels["status"]] = pointValues(t, line.Points)[0]
	}
	if 6 != got["200"] || 3 != got["404"] {
		t.Fatalf("totals = %v", got)
	}
	p50, err := sampler.Series("sampler_born_seconds", MeasureP50, nil, nil, base.Add(-time.Hour), base.Add(20*time.Second), time.Hour+20*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	if values := pointValues(t, p50.Series[0].Points); 1 != len(values) || 0.05 != values[0] {
		t.Fatalf("p50 = %v", values)
	}
}

func TestSamplerSplitsHistogramByBucketBound(t *testing.T) {
	registry := prometheus.NewRegistry()
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "sampler_bucket_seconds", Help: "Buckets.", Buckets: []float64{0.1, 0.5, 1}})
	registerCollector(t, registry, latency)
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour)
	base := time.Unix(1800000000, 0).UTC()

	mustSample(t, sampler, base)
	for range 5 {
		latency.Observe(0.05)
	}
	for range 3 {
		latency.Observe(0.3)
	}
	latency.Observe(2)
	mustSample(t, sampler, base.Add(10*time.Second))

	result, err := sampler.Series("sampler_bucket_seconds", MeasureTotal, []string{"le"}, nil, base, base.Add(10*time.Second), 10*time.Second)
	if nil != err {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, line := range result.Series {
		got[line.Labels["le"]] = pointValues(t, line.Points)[0]
	}
	if 3 != len(got) || 5 != got["0.1"] || 3 != got["0.5"] || 0 != got["1"] {
		t.Fatalf("buckets = %v", got)
	}
}

func TestSamplerSeriesRejectsInvalidQueries(t *testing.T) {
	registry := prometheus.NewRegistry()
	total := prometheus.NewCounter(prometheus.CounterOpts{Name: "sampler_invalid_total", Help: "Invalid."})
	registerCollector(t, registry, total)
	sampler := newTestSampler(t, registry, 10*time.Second, time.Hour)
	base := time.Unix(1800000000, 0).UTC()
	mustSample(t, sampler, base)

	cases := []struct {
		name    string
		measure Measure
		to      time.Time
		step    time.Duration
		want    error
	}{
		{"sampler_invalid_total", MeasureP95, base.Add(time.Minute), time.Minute, ErrInvalidQuery},
		{"sampler_invalid_total", MeasureValue, base, time.Minute, ErrInvalidQuery},
		{"sampler_invalid_total", MeasureValue, base.Add(time.Minute), 0, ErrInvalidQuery},
		{"sampler_missing_total", MeasureValue, base.Add(time.Minute), time.Minute, ErrUnknownMetric},
	}
	for _, tc := range cases {
		_, err := sampler.Series(tc.name, tc.measure, nil, nil, base, tc.to, tc.step)
		if !errors.Is(err, tc.want) {
			t.Fatalf("Series(%s, %s) error = %v, want %v", tc.name, tc.measure, err, tc.want)
		}
	}
}
