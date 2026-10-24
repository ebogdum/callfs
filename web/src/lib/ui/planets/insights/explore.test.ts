import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { metrics, series, type MetricsCatalog } from '$lib/api/admin/telemetry';
import type { MetricFamily, SeriesResult } from '$lib/api/types';
import { clearCache } from '$lib/data/cache';
import type { Series } from '$lib/ui/types';
import ExplorePlanet from './ExplorePlanet.svelte';
import {
	heatCells,
	parseExplore,
	promqlFor,
	ratioSeries,
	resolveMeasure,
	seriesRequest,
	stepSeconds,
	writeExplore,
	type ExploreQuery
} from './explore-state.svelte';

const NOW = 1_800_000_000_000;

const route = vi.hoisted(() => ({ url: new URL('http://127.0.0.1/ui/insights/explore') }));

vi.mock('$app/state', () => ({ page: route }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));
vi.mock('$lib/api/admin/telemetry', () => ({ metrics: vi.fn(), series: vi.fn() }));

const counter: MetricFamily = {
	name: 'callfs_http_requests_total',
	type: 'counter',
	help: 'Requests served.',
	emitted: true,
	series: [{ labels: { method: 'GET', status_code: '200' }, value: 12 }]
};

const histogram: MetricFamily = {
	name: 'callfs_http_request_duration_seconds',
	type: 'histogram',
	help: 'Response time.',
	emitted: true,
	series: [{ labels: { path: '/v1/files' }, count: 3, sum: 0.3, buckets: [] }]
};

const catalog = (families: MetricFamily[]): MetricsCatalog => ({ generated_at: '2026-10-23T00:00:00Z', families });

const seriesResult = (overrides: Partial<SeriesResult> = {}): SeriesResult => ({
	name: 'callfs_http_requests_total',
	measure: 'rate',
	unit: 'per_s',
	step_s: 60,
	series: [{ labels: { method: 'GET' }, points: [[1_800_000_000, 2], [1_800_000_060, null], [1_800_000_120, 3]] }],
	sampled_since: null,
	...overrides
});

const okResult = <T,>(value: T) => ({ ok: true as const, value });

const query = (overrides: Partial<ExploreQuery> = {}): ExploreQuery => ({
	metric: 'callfs_http_requests_total',
	measure: 'rate',
	by: [],
	filters: [],
	step: 'auto',
	chart: 'line',
	compare: false,
	ratio: null,
	range: { from: NOW - 3_600_000, to: NOW, live: false },
	...overrides
});

const useRoute = (search: string) => {
	route.url = new URL(`http://127.0.0.1/ui/insights/explore${search}`);
};

describe('explore query model', () => {
	it('defaults to the six hour range, line chart and automatic step', () => {
		const parsed = parseExplore(new URLSearchParams(''), NOW);
		expect(parsed.metric).toBe('');
		expect(parsed.measure).toBe('value');
		expect(parsed.chart).toBe('line');
		expect(parsed.step).toBe('auto');
		expect(parsed.range).toEqual({ from: NOW - 21_600_000, to: NOW, preset: '6h', live: false });
	});

	it('drops filters that are not label=value pairs', () => {
		const parsed = parseExplore(new URLSearchParams('filter=method%3DGET&filter=broken&filter==x&filter=path%3D'), NOW);
		expect(parsed.filters).toEqual(['method=GET']);
	});

	it('falls back to the default measure for a counter when the URL asks for a histogram measure', () => {
		expect(resolveMeasure('p95', 'counter')).toBe('rate');
		expect(resolveMeasure('p95', 'histogram')).toBe('p95');
	});

	it('round-trips every field through the URL', () => {
		const original = query({
			by: ['method', 'status_code'],
			filters: ['path=/v1/files'],
			step: '5m',
			chart: 'stacked',
			compare: true,
			ratio: { numerator: 'status_code=500', denominator: 'all' },
			range: { from: NOW - 7_200_000, to: NOW, live: true }
		});
		const restored = parseExplore(writeExplore(original), NOW);
		expect(restored).toEqual(original);
	});

	it('writes the preset instead of absolute bounds when a preset is active', () => {
		const params = writeExplore(query({ range: { from: 1, to: 2, preset: '1h', live: false } }));
		expect(params.get('preset')).toBe('1h');
		expect(params.has('from')).toBe(false);
	});

	it('picks an automatic step that keeps the chart at or under 360 points', () => {
		expect(stepSeconds('auto', 21_600)).toBe(60);
		expect(stepSeconds('auto', 86_400)).toBe(300);
		expect(stepSeconds('5m', 21_600)).toBe(300);
	});

	it('requests bucket counts by le for a heatmap', () => {
		const request = seriesRequest(query({ chart: 'heatmap', by: ['path'], metric: 'callfs_http_request_duration_seconds' }), histogram, NOW);
		expect(request.measure).toBe('rate');
		expect(request.by).toEqual(['le', 'path']);
	});

	it('builds a rate PromQL expression with the filter and grouping', () => {
		const promql = promqlFor(query({ by: ['method'], filters: ['status_code=500'] }), counter);
		expect(promql).toBe('sum by (method) (rate(callfs_http_requests_total{status_code="500"}[5m]))');
	});

	it('builds a p95 PromQL expression from histogram buckets', () => {
		const promql = promqlFor(
			query({ metric: 'callfs_http_request_duration_seconds', measure: 'p95', by: ['path'] }),
			histogram
		);
		expect(promql).toBe('histogram_quantile(0.95, sum by (le, path) (rate(callfs_http_request_duration_seconds_bucket[5m])))');
	});

	it('builds a ratio PromQL expression with one side per series', () => {
		const promql = promqlFor(
			query({ by: ['method'], ratio: { numerator: 'method=GET,status_code=500', denominator: 'method=GET' } }),
			counter
		);
		expect(promql).toBe(
			'(sum by (method) (rate(callfs_http_requests_total{method="GET", status_code="500"}[5m]))) / (sum by (method) (rate(callfs_http_requests_total{method="GET"}[5m])))'
		);
	});

	it('returns no PromQL when no metric is chosen', () => {
		expect(promqlFor(query({ metric: '' }), undefined)).toBe('');
	});

	it('turns a share query into percentages of the whole at each time', () => {
		const numerator: Series = { id: 'status_code=500', label: '500', unit: 'count', points: [[10, 1], [20, 4]] };
		const denominator: Series = { id: 'status_code=200', label: '200', unit: 'count', points: [[10, 4], [20, null]] };
		const share = ratioSeries(numerator, denominator, 'share');
		expect(share.points).toEqual([
			[10, 25],
			[20, null]
		]);
	});

	it('flattens histogram bucket series into heatmap cells and skips the +Inf bucket', () => {
		const cells = heatCells(
			seriesResult({
				series: [
					{ labels: { le: '0.5' }, points: [[10, 3]] },
					{ labels: { le: '+Inf' }, points: [[10, 7]] }
				]
			})
		);
		expect(cells).toEqual([{ x: 10, y: 0.5, value: 3 }]);
	});
});

describe('ExplorePlanet', () => {
	beforeEach(() => {
		clearCache();
		vi.mocked(metrics).mockReset();
		vi.mocked(series).mockReset();
		vi.mocked(metrics).mockResolvedValue(okResult(catalog([counter, histogram])) as never);
	});

	it('offers six starter questions before a metric is chosen', async () => {
		useRoute('');
		render(ExplorePlanet);
		expect(await screen.findByText('Pick a metric to see its chart.')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'How many requests come in?' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'How often do sign-ins fail?' })).toBeInTheDocument();
		expect(screen.getAllByRole('button', { name: /\?$/ })).toHaveLength(6);
	});

	it('states what the query measures in one sentence', async () => {
		useRoute('?metric=callfs_http_requests_total&measure=rate&by=method');
		vi.mocked(series).mockResolvedValue(okResult(seriesResult()) as never);
		render(ExplorePlanet);
		expect(await screen.findByText(/Rate per second of callfs_http_requests_total by method, last 1 day/)).toBeInTheDocument();
	});

	it('shows the network error title and keeps the page usable when the series request fails', async () => {
		useRoute('?metric=callfs_http_requests_total&measure=rate');
		vi.mocked(series).mockResolvedValue({ ok: false, error: { kind: 'network' } } as never);
		render(ExplorePlanet);
		expect((await screen.findAllByText('We cannot reach callfs.')).length).toBeGreaterThan(0);
	});

	it('explains that recording started partway through the range', async () => {
		useRoute('?metric=callfs_http_requests_total&measure=rate');
		vi.mocked(series).mockResolvedValue(okResult(seriesResult({ sampled_since: '2026-10-23T10:00:00Z' })) as never);
		render(ExplorePlanet);
		expect(await screen.findByText('Recording started partway through this range.')).toBeInTheDocument();
	});
});
