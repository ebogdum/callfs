package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
)

var (
	ErrInvalidSamplerConfig = errors.New("invalid metrics sampler configuration")
	ErrUnknownMetric        = errors.New("unknown metric family")
)

type Measure string

const (
	MeasureRate  Measure = "rate"
	MeasureTotal Measure = "total"
	MeasureAvg   Measure = "avg"
	MeasureP50   Measure = "p50"
	MeasureP95   Measure = "p95"
	MeasureP99   Measure = "p99"
	MeasureMax   Measure = "max"
	MeasureValue Measure = "value"
)

const (
	kindCounter   = "counter"
	kindGauge     = "gauge"
	kindUntyped   = "untyped"
	kindHistogram = "histogram"
	kindSummary   = "summary"
)

const (
	maxSamplerCapacity = 100000
	maxSeriesPoints    = 10000
	bucketLabel        = "le"
)

var quantiles = map[Measure]float64{
	MeasureP50: 0.5,
	MeasureP95: 0.95,
	MeasureP99: 0.99,
}

var measuresByKind = map[string][]Measure{
	kindCounter:   {MeasureRate, MeasureTotal, MeasureValue},
	kindGauge:     {MeasureValue, MeasureMax, MeasureAvg},
	kindUntyped:   {MeasureValue, MeasureMax, MeasureAvg},
	kindSummary:   {MeasureRate, MeasureTotal, MeasureAvg, MeasureValue},
	kindHistogram: {MeasureRate, MeasureTotal, MeasureAvg, MeasureMax, MeasureP50, MeasureP95, MeasureP99, MeasureValue},
}

type FamilyDeclaration struct {
	Name string
	Type string
	Help string
}

type SamplerConfig struct {
	Gatherer  prometheus.Gatherer
	Interval  time.Duration
	Retention time.Duration
	Declared  []FamilyDeclaration
}

type Bucket struct {
	LE    float64 `json:"le"`
	Count uint64  `json:"count"`
}

func (b Bucket) MarshalJSON() ([]byte, error) {
	if math.IsInf(b.LE, 1) {
		return json.Marshal(struct {
			LE    string `json:"le"`
			Count uint64 `json:"count"`
		}{LE: "+Inf", Count: b.Count})
	}
	return json.Marshal(struct {
		LE    float64 `json:"le"`
		Count uint64  `json:"count"`
	}{LE: b.LE, Count: b.Count})
}

type Point struct {
	Unix  int64
	Value *float64
}

func (p Point) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{p.Unix, p.Value})
}

type SeriesSnapshot struct {
	Labels  map[string]string `json:"labels"`
	Value   *float64          `json:"value,omitempty"`
	Count   *uint64           `json:"count,omitempty"`
	Sum     *float64          `json:"sum,omitempty"`
	Buckets []Bucket          `json:"buckets,omitempty"`
}

type FamilySnapshot struct {
	Name    string           `json:"name"`
	Type    string           `json:"type"`
	Help    string           `json:"help"`
	Emitted bool             `json:"emitted"`
	Series  []SeriesSnapshot `json:"series"`
}

type SeriesPoints struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

type SeriesResult struct {
	Name         string         `json:"name"`
	Measure      Measure        `json:"measure"`
	Unit         string         `json:"unit"`
	StepS        float64        `json:"step_s"`
	Series       []SeriesPoints `json:"series"`
	SampledSince *time.Time     `json:"sampled_since"`
}

type familyMeta struct {
	kind string
	help string
}

type sample struct {
	at      time.Time
	value   float64
	count   uint64
	sum     float64
	buckets []Bucket
}

type sampleRing struct {
	samples  []sample
	capacity int
	next     int
	wrapped  bool
}

func (r *sampleRing) add(s sample) {
	if len(r.samples) < r.capacity {
		r.samples = append(r.samples, s)
		return
	}
	r.samples[r.next] = s
	r.next = (r.next + 1) % r.capacity
	r.wrapped = true
}

func (r *sampleRing) size() int {
	return len(r.samples)
}

func (r *sampleRing) at(i int) sample {
	return r.samples[(r.next+i)%len(r.samples)]
}

func (r *sampleRing) lastAtOrBefore(t time.Time) int {
	return sort.Search(r.size(), func(i int) bool { return r.at(i).at.After(t) }) - 1
}

func (r *sampleRing) window(start, end time.Time) (sample, sample, bool) {
	prevIdx, curIdx := r.lastAtOrBefore(start), r.lastAtOrBefore(end)
	if 0 > curIdx || curIdx <= prevIdx {
		return sample{}, sample{}, false
	}
	if 0 <= prevIdx {
		return r.at(prevIdx), r.at(curIdx), true
	}
	if r.wrapped {
		if 0 == curIdx {
			return sample{}, sample{}, false
		}
		return r.at(0), r.at(curIdx), true
	}
	return sample{}, r.at(curIdx), true
}

type seriesState struct {
	family  string
	labels  map[string]string
	samples sampleRing
}

type seriesGroup struct {
	labels  map[string]string
	members []*seriesState
}

type Sampler struct {
	gatherer prometheus.Gatherer
	interval time.Duration
	capacity int
	mu       sync.RWMutex
	families map[string]familyMeta
	series   map[string]*seriesState
}

func NewSampler(cfg SamplerConfig) (*Sampler, error) {
	if 0 >= cfg.Interval || cfg.Interval > cfg.Retention {
		return nil, fmt.Errorf("%w: interval and retention", ErrInvalidSamplerConfig)
	}
	capacity := int(cfg.Retention / cfg.Interval)
	if maxSamplerCapacity < capacity {
		return nil, fmt.Errorf("%w: more than %d samples per series", ErrInvalidSamplerConfig, maxSamplerCapacity)
	}
	gatherer := cfg.Gatherer
	if nil == gatherer {
		gatherer = prometheus.DefaultGatherer
	}
	families := make(map[string]familyMeta, len(cfg.Declared))
	for _, declared := range cfg.Declared {
		families[declared.Name] = familyMeta{kind: declared.Type, help: declared.Help}
	}
	return &Sampler{
		gatherer: gatherer,
		interval: cfg.Interval,
		capacity: capacity,
		families: families,
		series:   map[string]*seriesState{},
	}, nil
}

func (s *Sampler) Run(ctx context.Context, logger *zap.Logger) {
	if nil == logger {
		logger = zap.NewNop()
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			if err := s.Sample(at); nil != err {
				logger.Warn("metrics sample failed", zap.Error(err))
			}
		}
	}
}

func (s *Sampler) Sample(at time.Time) error {
	families, gatherErr := s.gatherer.Gather()
	s.mu.Lock()
	for _, family := range families {
		s.record(at, family)
	}
	s.mu.Unlock()
	if nil != gatherErr {
		return fmt.Errorf("gather metrics: %w", gatherErr)
	}
	return nil
}

func (s *Sampler) record(at time.Time, family *dto.MetricFamily) {
	name := family.GetName()
	kind := strings.ToLower(family.GetType().String())
	s.families[name] = familyMeta{kind: kind, help: family.GetHelp()}
	for _, metric := range family.GetMetric() {
		state := s.stateFor(name, labelsOf(metric))
		state.samples.add(sampleOf(at, kind, metric))
	}
}

func (s *Sampler) stateFor(name string, labels map[string]string) *seriesState {
	key := seriesKey(name, labels)
	state, ok := s.series[key]
	if !ok {
		state = &seriesState{family: name, labels: labels, samples: sampleRing{capacity: s.capacity}}
		s.series[key] = state
	}
	return state
}

func labelsOf(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.GetLabel()))
	for _, pair := range metric.GetLabel() {
		labels[pair.GetName()] = pair.GetValue()
	}
	return labels
}

func sampleOf(at time.Time, kind string, metric *dto.Metric) sample {
	switch kind {
	case kindHistogram:
		histogram := metric.GetHistogram()
		return sample{at: at, count: histogram.GetSampleCount(), sum: histogram.GetSampleSum(), buckets: bucketsOf(histogram)}
	case kindSummary:
		summary := metric.GetSummary()
		return sample{at: at, count: summary.GetSampleCount(), sum: summary.GetSampleSum()}
	case kindCounter:
		return sample{at: at, value: metric.GetCounter().GetValue()}
	case kindGauge:
		return sample{at: at, value: metric.GetGauge().GetValue()}
	}
	return sample{at: at, value: metric.GetUntyped().GetValue()}
}

func bucketsOf(histogram *dto.Histogram) []Bucket {
	buckets := make([]Bucket, 0, len(histogram.GetBucket())+1)
	for _, bucket := range histogram.GetBucket() {
		buckets = append(buckets, Bucket{LE: bucket.GetUpperBound(), Count: bucket.GetCumulativeCount()})
	}
	if 0 == len(buckets) || !math.IsInf(buckets[len(buckets)-1].LE, 1) {
		buckets = append(buckets, Bucket{LE: math.Inf(1), Count: histogram.GetSampleCount()})
	}
	return buckets
}

func seriesKey(name string, labels map[string]string) string {
	var key strings.Builder
	key.WriteString(name)
	for _, label := range slices.Sorted(maps.Keys(labels)) {
		key.WriteByte(0)
		key.WriteString(label)
		key.WriteByte(0)
		key.WriteString(labels[label])
	}
	return key.String()
}

func (s *Sampler) Families() []FamilySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byFamily := make(map[string][]SeriesSnapshot, len(s.families))
	for _, key := range slices.Sorted(maps.Keys(s.series)) {
		state := s.series[key]
		if 0 == state.samples.size() {
			continue
		}
		kind := s.families[state.family].kind
		latest := state.samples.at(state.samples.size() - 1)
		byFamily[state.family] = append(byFamily[state.family], snapshotOf(kind, state.labels, latest))
	}
	families := make([]FamilySnapshot, 0, len(s.families))
	for _, name := range slices.Sorted(maps.Keys(s.families)) {
		series := byFamily[name]
		if nil == series {
			series = []SeriesSnapshot{}
		}
		meta := s.families[name]
		families = append(families, FamilySnapshot{
			Name:    name,
			Type:    meta.kind,
			Help:    meta.help,
			Emitted: 0 < len(series),
			Series:  series,
		})
	}
	return families
}

func snapshotOf(kind string, labels map[string]string, latest sample) SeriesSnapshot {
	snapshot := SeriesSnapshot{Labels: labels}
	switch kind {
	case kindHistogram, kindSummary:
		count := latest.count
		snapshot.Count = &count
		snapshot.Sum = finite(latest.sum, true)
		snapshot.Buckets = latest.buckets
	default:
		snapshot.Value = finite(latest.value, true)
	}
	return snapshot
}

func (s *Sampler) Series(name string, measure Measure, by []string, filter map[string]string, from, to time.Time, step time.Duration) (SeriesResult, error) {
	if 0 >= step || !to.After(from) {
		return SeriesResult{}, fmt.Errorf("%w: from, to and step", ErrInvalidQuery)
	}
	span := to.Sub(from)
	pointCount := int64(span / step)
	if 0 < span%step {
		pointCount++
	}
	if maxSeriesPoints < pointCount {
		return SeriesResult{}, fmt.Errorf("%w: more than %d points", ErrInvalidQuery, maxSeriesPoints)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	family, known := s.families[name]
	if !known {
		return SeriesResult{}, fmt.Errorf("%w: %s", ErrUnknownMetric, name)
	}
	if !slices.Contains(measuresByKind[family.kind], measure) {
		return SeriesResult{}, fmt.Errorf("%w: %s for %s", ErrInvalidQuery, measure, family.kind)
	}
	groups := s.groupsFor(name, filter, by)
	var since *time.Time
	for _, group := range groups {
		for _, member := range group.members {
			if 0 < member.samples.size() {
				first := member.samples.at(0).at
				if nil == since || first.Before(*since) {
					since = &first
				}
			}
		}
	}
	result := SeriesResult{
		Name:         name,
		Measure:      measure,
		Unit:         unitOf(family.kind, name, measure),
		StepS:        step.Seconds(),
		Series:       make([]SeriesPoints, 0, len(groups)),
		SampledSince: since,
	}
	byBucket := kindHistogram == family.kind && slices.Contains(by, bucketLabel) && (MeasureRate == measure || MeasureTotal == measure)
	for _, group := range groups {
		if byBucket {
			result.Series = append(result.Series, bucketLines(group, measure, from, to, step)...)
			continue
		}
		points := make([]Point, 0, pointCount)
		for start := from; start.Before(to); start = start.Add(step) {
			end := start.Add(step)
			if end.After(to) {
				end = to
			}
			points = append(points, Point{
				Unix:  end.Unix(),
				Value: aggregate(family.kind, measure, group.members, start, end, end.Sub(start)),
			})
		}
		result.Series = append(result.Series, SeriesPoints{Labels: group.labels, Points: points})
	}
	return result, nil
}

type bucketWindow struct {
	end   time.Time
	width time.Duration
	delta histogramDelta
	ok    bool
}

func bucketLines(group seriesGroup, measure Measure, from, to time.Time, step time.Duration) []SeriesPoints {
	windows := []bucketWindow{}
	bounds := map[float64]struct{}{}
	for start := from; start.Before(to); start = start.Add(step) {
		end := start.Add(step)
		if end.After(to) {
			end = to
		}
		delta, ok := histogramDeltas(group.members, start, end)
		windows = append(windows, bucketWindow{end: end, width: end.Sub(start), delta: delta, ok: ok})
		for _, bucket := range delta.buckets {
			if ok && !math.IsInf(bucket.LE, 1) {
				bounds[bucket.LE] = struct{}{}
			}
		}
	}
	lines := make([]SeriesPoints, 0, len(bounds))
	for _, bound := range slices.Sorted(maps.Keys(bounds)) {
		labels := maps.Clone(group.labels)
		labels[bucketLabel] = strconv.FormatFloat(bound, 'g', -1, 64)
		points := make([]Point, 0, len(windows))
		for _, window := range windows {
			points = append(points, Point{Unix: window.end.Unix(), Value: bucketValue(window, bound, measure)})
		}
		lines = append(lines, SeriesPoints{Labels: labels, Points: points})
	}
	return lines
}

func bucketValue(window bucketWindow, bound float64, measure Measure) *float64 {
	if !window.ok {
		return nil
	}
	var below, at uint64
	for _, bucket := range window.delta.buckets {
		if bucket.LE < bound {
			below = bucket.Count
		}
		if bucket.LE == bound {
			at = bucket.Count
		}
	}
	count := float64(counterStep(below, at))
	if MeasureRate == measure {
		count /= window.width.Seconds()
	}
	return finite(count, true)
}

func (s *Sampler) groupsFor(name string, filter map[string]string, by []string) []seriesGroup {
	grouped := map[string]*seriesGroup{}
	for _, state := range s.series {
		if state.family != name || !matchesLabels(state.labels, filter) {
			continue
		}
		labels := make(map[string]string, len(by))
		values := make([]string, 0, len(by))
		for _, label := range by {
			labels[label] = state.labels[label]
			values = append(values, state.labels[label])
		}
		key := strings.Join(values, "\x00")
		group, ok := grouped[key]
		if !ok {
			group = &seriesGroup{labels: labels}
			grouped[key] = group
		}
		group.members = append(group.members, state)
	}
	groups := make([]seriesGroup, 0, len(grouped))
	for _, key := range slices.Sorted(maps.Keys(grouped)) {
		groups = append(groups, *grouped[key])
	}
	return groups
}

func matchesLabels(labels, filter map[string]string) bool {
	for name, value := range filter {
		if got, ok := labels[name]; !ok || got != value {
			return false
		}
	}
	return true
}

func unitOf(kind, name string, measure Measure) string {
	unit := "count"
	countOnly := (kindHistogram == kind || kindSummary == kind) && slices.Contains([]Measure{MeasureRate, MeasureTotal, MeasureValue}, measure)
	if !countOnly {
		switch {
		case strings.HasSuffix(name, "_seconds"):
			unit = "seconds"
		case strings.HasSuffix(name, "_bytes"):
			unit = "bytes"
		}
	}
	if MeasureRate == measure {
		return unit + "/s"
	}
	return unit
}

func aggregate(kind string, measure Measure, members []*seriesState, start, end time.Time, width time.Duration) *float64 {
	switch kind {
	case kindCounter:
		return counterMeasure(measure, members, start, end, width)
	case kindGauge, kindUntyped:
		return gaugeMeasure(measure, members, start, end)
	}
	return histogramMeasure(measure, members, start, end, width)
}

func counterMeasure(measure Measure, members []*seriesState, start, end time.Time, width time.Duration) *float64 {
	if MeasureValue == measure {
		return finite(lastSum(members, end, scalarValue))
	}
	total, ok := deltaSum(members, start, end)
	if MeasureRate == measure {
		total /= width.Seconds()
	}
	return finite(total, ok)
}

func gaugeMeasure(measure Measure, members []*seriesState, start, end time.Time) *float64 {
	maximum, mean, ok := windowStats(members, start, end)
	switch measure {
	case MeasureValue:
		return finite(lastSum(members, end, scalarValue))
	case MeasureMax:
		return finite(maximum, ok)
	case MeasureAvg:
		return finite(mean, ok)
	}
	return nil
}

func histogramMeasure(measure Measure, members []*seriesState, start, end time.Time, width time.Duration) *float64 {
	if MeasureValue == measure {
		return finite(lastSum(members, end, histogramCount))
	}
	delta, ok := histogramDeltas(members, start, end)
	if !ok {
		return nil
	}
	if quantile, isQuantile := quantiles[measure]; isQuantile {
		return finite(quantileOf(delta.buckets, quantile))
	}
	switch measure {
	case MeasureRate:
		return finite(float64(delta.count)/width.Seconds(), true)
	case MeasureTotal:
		return finite(float64(delta.count), true)
	case MeasureAvg:
		if 0 == delta.count {
			return nil
		}
		return finite(delta.sum/float64(delta.count), true)
	case MeasureMax:
		return finite(maxOf(delta.buckets))
	}
	return nil
}

func scalarValue(s sample) float64 {
	return s.value
}

func histogramCount(s sample) float64 {
	return float64(s.count)
}

func deltaSum(members []*seriesState, start, end time.Time) (float64, bool) {
	total, found := 0.0, false
	for _, member := range members {
		first, last, ok := member.samples.window(start, end)
		if !ok {
			continue
		}
		found = true
		prev, cur := first.value, last.value
		if prev > cur {
			total += cur
		} else {
			total += cur - prev
		}
	}
	return total, found
}

func lastSum(members []*seriesState, end time.Time, field func(sample) float64) (float64, bool) {
	total, found := 0.0, false
	for _, member := range members {
		idx := member.samples.lastAtOrBefore(end)
		if 0 > idx {
			continue
		}
		total += field(member.samples.at(idx))
		found = true
	}
	return total, found
}

func windowStats(members []*seriesState, start, end time.Time) (float64, float64, bool) {
	maximum, total, count := math.Inf(-1), 0.0, 0
	for _, member := range members {
		last := member.samples.lastAtOrBefore(end)
		for i := member.samples.lastAtOrBefore(start) + 1; i <= last; i++ {
			value := member.samples.at(i).value
			total += value
			count++
			if value > maximum {
				maximum = value
			}
		}
	}
	if 0 == count {
		return 0, 0, false
	}
	return maximum, total / float64(count), true
}

type histogramDelta struct {
	count   uint64
	sum     float64
	buckets []Bucket
}

func histogramDeltas(members []*seriesState, start, end time.Time) (histogramDelta, bool) {
	var delta histogramDelta
	merged := map[float64]uint64{}
	found := false
	for _, member := range members {
		prev, cur, ok := member.samples.window(start, end)
		if !ok {
			continue
		}
		found = true
		delta.count += counterStep(prev.count, cur.count)
		if prev.count > cur.count {
			delta.sum += cur.sum
		} else {
			delta.sum += cur.sum - prev.sum
		}
		previous := make(map[float64]uint64, len(prev.buckets))
		for _, bucket := range prev.buckets {
			previous[bucket.LE] = bucket.Count
		}
		for _, bucket := range cur.buckets {
			merged[bucket.LE] += counterStep(previous[bucket.LE], bucket.Count)
		}
	}
	delta.buckets = make([]Bucket, 0, len(merged))
	for _, le := range slices.Sorted(maps.Keys(merged)) {
		delta.buckets = append(delta.buckets, Bucket{LE: le, Count: merged[le]})
	}
	return delta, found
}

func counterStep(prev, cur uint64) uint64 {
	if prev > cur {
		return cur
	}
	return cur - prev
}

func quantileOf(buckets []Bucket, quantile float64) (float64, bool) {
	if 0 == len(buckets) || !math.IsInf(buckets[len(buckets)-1].LE, 1) {
		return 0, false
	}
	total := buckets[len(buckets)-1].Count
	if 0 == total {
		return 0, false
	}
	rank := quantile * float64(total)
	lower, lowerCount := 0.0, uint64(0)
	for _, bucket := range buckets {
		if math.IsInf(bucket.LE, 1) {
			return lower, true
		}
		if float64(bucket.Count) >= rank {
			return lower + (bucket.LE-lower)*(rank-float64(lowerCount))/float64(bucket.Count-lowerCount), true
		}
		lower, lowerCount = bucket.LE, bucket.Count
	}
	return lower, true
}

func maxOf(buckets []Bucket) (float64, bool) {
	var highest float64
	found := false
	var previous uint64
	for _, bucket := range buckets {
		if previous < bucket.Count && !math.IsInf(bucket.LE, 1) {
			highest, found = bucket.LE, true
		}
		previous = bucket.Count
	}
	return highest, found
}

func finite(value float64, ok bool) *float64 {
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}
