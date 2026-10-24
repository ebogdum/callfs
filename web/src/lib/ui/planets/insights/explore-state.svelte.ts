import type { MetricFamily, SeriesResult } from '$lib/api/types';
import type { Series, Unit } from '$lib/ui/types';

export const EXPLORE_MEASURES = ['rate', 'total', 'avg', 'p50', 'p95', 'p99', 'max', 'value'] as const;
export type ExploreMeasure = (typeof EXPLORE_MEASURES)[number];

export const EXPLORE_STEPS = ['auto', '10s', '1m', '5m', '1h'] as const;
export type ExploreStep = (typeof EXPLORE_STEPS)[number];

export const EXPLORE_CHARTS = ['line', 'stacked', 'bars', 'table', 'heatmap'] as const;
export type ExploreChart = (typeof EXPLORE_CHARTS)[number];

export const EXPLORE_PRESETS: Readonly<Record<string, number>> = {
	'15m': 900_000,
	'1h': 3_600_000,
	'6h': 21_600_000,
	'24h': 86_400_000,
	'7d': 604_800_000
};

export const DEFAULT_PRESET = '6h';
export const LIVE_REFRESH_MS = 10_000;
export const ALL_SERIES = 'all';

const STEP_SECONDS: Readonly<Record<Exclude<ExploreStep, 'auto'>, number>> = { '10s': 10, '1m': 60, '5m': 300, '1h': 3600 };
const AUTO_STEP_SECONDS = [10, 60, 300, 900, 3600];
const MAX_AUTO_POINTS = 360;
const RATE_WINDOW = '5m';
const UNITS: readonly Unit[] = ['bytes', 'bytes_per_s', 'count', 'per_s', 'ms', 's', 'percent', 'ratio', 'none'];
const MEASURES_BY_TYPE: Readonly<Record<string, readonly ExploreMeasure[]>> = {
	counter: ['rate', 'total'],
	histogram: ['p50', 'p95', 'p99', 'avg', 'max'],
	gauge: ['value', 'avg', 'max']
};
const DEFAULT_MEASURE_BY_TYPE: Readonly<Record<string, ExploreMeasure>> = { counter: 'rate', histogram: 'p95', gauge: 'value' };
const QUANTILES: Partial<Record<ExploreMeasure, number>> = { p50: 0.5, p95: 0.95, p99: 0.99 };

export interface ExploreRange {
	from: number;
	to: number;
	preset?: string;
	live: boolean;
}

export interface ExploreRatio {
	numerator: string;
	denominator: string;
}

export interface ExploreQuery {
	metric: string;
	measure: ExploreMeasure;
	by: string[];
	filters: string[];
	step: ExploreStep;
	chart: ExploreChart;
	compare: boolean;
	ratio: ExploreRatio | null;
	range: ExploreRange;
}

export interface SeriesRequest {
	name: string;
	measure: ExploreMeasure;
	by: string[];
	filter: string[];
	from: number;
	to: number;
	stepS: number;
}

export interface StarterQuestion {
	copy: string;
	metric: string;
	measure: ExploreMeasure;
	by: string[];
}

export const STARTER_QUESTIONS: readonly StarterQuestion[] = [
	{ copy: 'explore.starter.requests', metric: 'callfs_http_requests_total', measure: 'rate', by: ['method'] },
	{ copy: 'explore.starter.routes', metric: 'callfs_http_request_duration_seconds', measure: 'p95', by: ['path'] },
	{ copy: 'explore.starter.status', metric: 'callfs_http_requests_total', measure: 'rate', by: ['status_code'] },
	{ copy: 'explore.starter.storage', metric: 'callfs_usage_bytes', measure: 'value', by: ['storage'] },
	{ copy: 'explore.starter.locks', metric: 'callfs_active_locks', measure: 'value', by: [] },
	{ copy: 'explore.starter.signins', metric: 'callfs_auth_failures_total', measure: 'rate', by: ['reason'] }
];

export function isFilter(entry: string): boolean {
	const at = entry.indexOf('=');
	return 0 < at && at < entry.length - 1;
}

export function measuresFor(type: string | undefined): readonly ExploreMeasure[] {
	return undefined === type ? EXPLORE_MEASURES : (MEASURES_BY_TYPE[type] ?? EXPLORE_MEASURES);
}

export function resolveMeasure(measure: ExploreMeasure, type: string | undefined): ExploreMeasure {
	if (measuresFor(type).includes(measure)) {
		return measure;
	}
	return undefined === type ? 'value' : (DEFAULT_MEASURE_BY_TYPE[type] ?? 'value');
}

export function unitOf(value: string): Unit {
	return UNITS.find((unit) => unit === value) ?? 'none';
}

export function resolveRange(range: ExploreRange, now: number): { from: number; to: number } {
	const span = undefined === range.preset ? undefined : EXPLORE_PRESETS[range.preset];
	if (undefined === span) {
		return { from: range.from, to: range.to };
	}
	return { from: now - span, to: now };
}

export function stepSeconds(step: ExploreStep, spanSeconds: number): number {
	if ('auto' !== step) {
		return STEP_SECONDS[step];
	}
	return AUTO_STEP_SECONDS.find((candidate) => MAX_AUTO_POINTS >= spanSeconds / candidate) ?? 3600;
}

export function defaultRange(now: number): ExploreRange {
	return { from: now - (EXPLORE_PRESETS[DEFAULT_PRESET] ?? 0), to: now, preset: DEFAULT_PRESET, live: false };
}

function readRange(params: URLSearchParams, now: number): ExploreRange {
	const live = '1' === params.get('live');
	const presetRaw = params.get('preset');
	if (null !== presetRaw && Object.hasOwn(EXPLORE_PRESETS, presetRaw)) {
		return { from: now - EXPLORE_PRESETS[presetRaw], to: now, preset: presetRaw, live };
	}
	const fromRaw = params.get('from');
	const toRaw = params.get('to');
	const from = null === fromRaw ? Number.NaN : Number(fromRaw);
	const to = null === toRaw ? Number.NaN : Number(toRaw);
	if (Number.isFinite(from) && Number.isFinite(to) && from < to) {
		return { from, to, live };
	}
	return { ...defaultRange(now), live };
}

export function oneOf<T extends string>(options: readonly T[], value: string | null, fallback: T): T {
	return options.find((option) => value === option) ?? fallback;
}

export function parseExplore(params: URLSearchParams, now: number): ExploreQuery {
	const numerator = params.get('of') ?? '';
	const denominator = params.get('over') ?? '';
	return {
		metric: params.get('metric') ?? '',
		measure: oneOf(EXPLORE_MEASURES, params.get('measure'), 'value'),
		by: (params.get('by') ?? '').split(',').filter((key) => '' !== key),
		filters: params.getAll('filter').filter(isFilter),
		step: oneOf(EXPLORE_STEPS, params.get('step'), 'auto'),
		chart: oneOf(EXPLORE_CHARTS, params.get('chart'), 'line'),
		compare: '1' === params.get('compare'),
		ratio: '' === numerator || '' === denominator ? null : { numerator, denominator },
		range: readRange(params, now)
	};
}

export function writeExplore(query: ExploreQuery): URLSearchParams {
	const params = new URLSearchParams();
	if ('' !== query.metric) {
		params.set('metric', query.metric);
	}
	params.set('measure', query.measure);
	if (0 < query.by.length) {
		params.set('by', query.by.join(','));
	}
	for (const filter of query.filters) {
		params.append('filter', filter);
	}
	params.set('step', query.step);
	params.set('chart', query.chart);
	if (query.compare) {
		params.set('compare', '1');
	}
	if (null !== query.ratio) {
		params.set('of', query.ratio.numerator);
		params.set('over', query.ratio.denominator);
	}
	if (undefined === query.range.preset) {
		params.set('from', String(query.range.from));
		params.set('to', String(query.range.to));
	} else {
		params.set('preset', query.range.preset);
	}
	if (query.range.live) {
		params.set('live', '1');
	}
	return params;
}

export function starterQuery(starter: StarterQuestion, range: ExploreRange): ExploreQuery {
	return {
		metric: starter.metric,
		measure: starter.measure,
		by: starter.by,
		filters: [],
		step: 'auto',
		chart: 'line',
		compare: false,
		ratio: null,
		range
	};
}

export function seriesRequest(query: ExploreQuery, family: MetricFamily | undefined, now: number): SeriesRequest {
	const heatmap = 'heatmap' === query.chart;
	const bounds = resolveRange(query.range, now);
	const spanSeconds = (bounds.to - bounds.from) / 1000;
	const by = heatmap && !query.by.includes('le') ? ['le', ...query.by] : query.by;
	return {
		name: query.metric,
		measure: heatmap ? 'rate' : resolveMeasure(query.measure, family?.type),
		by,
		filter: query.filters,
		from: Math.floor(bounds.from / 1000),
		to: Math.floor(bounds.to / 1000),
		stepS: stepSeconds(query.step, spanSeconds)
	};
}

export function seriesIdOf(labels: Record<string, string>): string {
	const pairs = Object.keys(labels)
		.sort()
		.map((key) => `${key}=${labels[key] ?? ''}`);
	return 0 === pairs.length ? ALL_SERIES : pairs.join(',');
}

export function labelsOfId(id: string): Record<string, string> {
	if (ALL_SERIES === id) {
		return {};
	}
	const labels: Record<string, string> = {};
	for (const pair of id.split(',')) {
		const at = pair.indexOf('=');
		if (0 < at) {
			labels[pair.slice(0, at)] = pair.slice(at + 1);
		}
	}
	return labels;
}

export function seriesLabelOf(labels: Record<string, string>, fallback: string): string {
	const values = Object.keys(labels)
		.sort()
		.map((key) => labels[key] ?? '');
	return 0 === values.length ? fallback : values.join(' ');
}

export function chartSeries(result: SeriesResult, metric: string): Series[] {
	const unit = unitOf(result.unit);
	return result.series.map((entry, index) => ({
		id: seriesIdOf(entry.labels),
		label: seriesLabelOf(entry.labels, metric),
		unit,
		points: entry.points,
		colorIndex: index
	}));
}

export function shiftedSeries(series: readonly Series[], seconds: number, suffix: string): Series[] {
	return series.map((entry) => ({
		...entry,
		id: `${entry.id}:previous`,
		label: `${entry.label}${suffix}`,
		points: entry.points.map(([time, value]): [number, number | null] => [time + seconds, value]),
		dashed: true
	}));
}

export function ratioSeries(numerator: Series, denominator: Series, label: string): Series {
	const wholes = new Map(denominator.points);
	return {
		id: 'ratio',
		label,
		unit: 'percent',
		points: numerator.points.map(([time, value]): [number, number | null] => {
			const whole = wholes.get(time) ?? null;
			return [time, null === value || null === whole || 0 === whole ? null : (value / whole) * 100];
		})
	};
}

export function heatCells(result: SeriesResult): { x: number; y: number; value: number }[] {
	return result.series.flatMap((entry) => {
		const bound = Number(entry.labels.le);
		if (undefined === entry.labels.le || !Number.isFinite(bound)) {
			return [];
		}
		return entry.points.flatMap(([time, value]) => (null === value ? [] : [{ x: time, y: bound, value }]));
	});
}

export function hasBuckets(result: SeriesResult): boolean {
	return result.series.some((entry) => undefined !== entry.labels.le);
}

export function barValue(points: readonly [number, number | null][], measure: ExploreMeasure): number | null {
	const values = points.map(([, value]) => value).filter((value): value is number => null !== value);
	if (0 === values.length) {
		return null;
	}
	if ('total' === measure || 'value' === measure) {
		return values.at(-1) ?? null;
	}
	return values.reduce((sum, value) => sum + value, 0) / values.length;
}

export function barData(series: readonly Series[], measure: ExploreMeasure): { label: string; value: number; colorIndex?: number }[] {
	return series.flatMap((entry) => {
		const value = barValue(entry.points, measure);
		return null === value ? [] : [{ label: entry.label, value, colorIndex: entry.colorIndex }];
	});
}

export function promqlMatcher(filter: string): string {
	const at = filter.indexOf('=');
	return `${filter.slice(0, at)}="${filter.slice(at + 1)}"`;
}

function selectorFor(filters: readonly string[], extra: Record<string, string>): string {
	const matchers = [...filters.map(promqlMatcher), ...Object.keys(extra).sort().map((key) => `${key}="${extra[key] ?? ''}"`)];
	return 0 === matchers.length ? '' : `{${matchers.join(', ')}}`;
}

function aggregate(fn: string, by: readonly string[], inner: string): string {
	return 0 === by.length ? `${fn}(${inner})` : `${fn} by (${by.join(', ')}) (${inner})`;
}

function promqlExpression(
	name: string,
	type: string | undefined,
	measure: ExploreMeasure,
	by: readonly string[],
	filters: readonly string[],
	extra: Record<string, string>
): string {
	const selector = selectorFor(filters, extra);
	const series = `${name}${selector}`;
	const histogram = 'histogram' === type;
	const quantile = QUANTILES[measure];
	if (undefined !== quantile) {
		return `histogram_quantile(${quantile}, ${aggregate('sum', ['le', ...by], `rate(${name}_bucket${selector}[${RATE_WINDOW}])`)})`;
	}
	if ('avg' === measure && histogram) {
		return `${aggregate('sum', by, `rate(${name}_sum${selector}[${RATE_WINDOW}])`)} / ${aggregate('sum', by, `rate(${name}_count${selector}[${RATE_WINDOW}])`)}`;
	}
	if ('avg' === measure) {
		return aggregate('avg', by, series);
	}
	if ('max' === measure) {
		return aggregate('max', by, series);
	}
	if ('rate' === measure && histogram) {
		return aggregate('sum', by, `rate(${name}_bucket${selector}[${RATE_WINDOW}])`);
	}
	if ('rate' === measure) {
		return aggregate('sum', by, `rate(${series}[${RATE_WINDOW}])`);
	}
	return aggregate('sum', by, series);
}

export function promqlFor(query: ExploreQuery, family: MetricFamily | undefined): string {
	if ('' === query.metric) {
		return '';
	}
	const measure = resolveMeasure(query.measure, family?.type);
	const expression = (extra: Record<string, string>): string =>
		promqlExpression(query.metric, family?.type, measure, query.by, query.filters, extra);
	if (null === query.ratio) {
		return expression({});
	}
	return `(${expression(labelsOfId(query.ratio.numerator))}) / (${expression(labelsOfId(query.ratio.denominator))})`;
}
