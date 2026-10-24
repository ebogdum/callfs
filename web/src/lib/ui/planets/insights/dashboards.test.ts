import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { eventHistory, metrics, series } from '$lib/api/admin/telemetry';
import type { Event as EventRecord, SeriesResult } from '$lib/api/types';
import {
	DEFAULT_PRESET,
	aggregate,
	annotationsOf,
	barData,
	buildDashboards,
	crossedThreshold,
	failureLines,
	heatCells,
	matchesFailure,
	measuredValue,
	missingExpected,
	panelFromCustom,
	panelLines,
	rangeFromParams,
	statReading,
	stepFor,
	type PanelDefinition,
	type PanelLine,
	type PanelQuery,
	type PanelSample
} from './dashboards';
import DashboardsPlanet from './DashboardsPlanet.svelte';

vi.mock('$app/state', () => ({ page: { url: new URL('http://127.0.0.1/ui/insights') } }));
vi.mock('$lib/api/admin/telemetry', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/telemetry')>()),
	series: vi.fn(),
	metrics: vi.fn(),
	eventHistory: vi.fn()
}));

const NOW = 1_800_000_000;

const emptyResult = (name: string): SeriesResult => ({ name, measure: 'total', unit: 'count', step_s: 60, series: [], sampled_since: null });

const totalByStatus: PanelQuery = { measure: 'total', group: ['status_code'], filter: [] };

const statDefinition: PanelDefinition = {
	id: 'errors',
	title: 'Server errors',
	metric: 'callfs_http_requests_total',
	queries: [totalByStatus],
	unit: 'count',
	thresholds: [
		{ value: 1, tone: 'warn', label: 'Warning above 1 %' },
		{ value: 5, tone: 'danger', label: 'Critical above 5 %' }
	],
	chart: 'stat',
	span: 4,
	failure: { label: 'status_code', mode: 'prefix', value: '5' }
};

const statSamples: PanelSample[] = [
	{
		query: totalByStatus,
		result: {
			name: 'callfs_http_requests_total',
			measure: 'total',
			unit: 'count',
			step_s: 60,
			sampled_since: '2026-10-23T00:00:00Z',
			series: [
				{ labels: { status_code: '200' }, points: [[1, 90], [2, 100]] },
				{ labels: { status_code: '500' }, points: [[1, 3], [2, null]] }
			]
		}
	}
];

describe('rangeFromParams', () => {
	it('uses the default preset and live refresh when no parameters are set', () => {
		expect(DEFAULT_PRESET).toBe('1h');
		expect(rangeFromParams(new URLSearchParams(), NOW)).toEqual({ from: NOW - 3_600, to: NOW, live: true });
	});

	it('applies a named preset and turns live off when asked', () => {
		expect(rangeFromParams(new URLSearchParams('preset=7d&live=false'), NOW)).toEqual({
			from: NOW - 604_800,
			to: NOW,
			live: false
		});
	});

	it('uses absolute bounds when they are ordered whole seconds', () => {
		expect(rangeFromParams(new URLSearchParams('from=100&to=200'), NOW)).toEqual({ from: 100, to: 200, live: true });
	});

	it('falls back to the default preset for bounds that are not usable', () => {
		expect(rangeFromParams(new URLSearchParams('from=200&to=100'), NOW)).toEqual({ from: NOW - 3_600, to: NOW, live: true });
		expect(rangeFromParams(new URLSearchParams('from=abc&to=300'), NOW)).toEqual({ from: NOW - 3_600, to: NOW, live: true });
	});
});

describe('stepFor', () => {
	it('keeps steps at five seconds or more and under the point budget', () => {
		expect(stepFor({ from: 0, to: 60, live: true })).toBe(5);
		expect(stepFor({ from: 0, to: 604_800, live: true })).toBe(2_520);
	});
});

describe('buildDashboards', () => {
	it('defines the six built-in dashboards in order with at most eight panels each', () => {
		const dashboards = buildDashboards();
		expect(dashboards.map((dashboard) => dashboard.id)).toEqual([
			'traffic',
			'file-work',
			'download-links',
			'edit-locks',
			'file-index',
			'errors'
		]);
		for (const dashboard of dashboards) {
			expect(dashboard.panels.length).toBeGreaterThan(0);
			expect(dashboard.panels.length).toBeLessThanOrEqual(8);
		}
	});

	it('gives every panel a metric, at least one query and a unique id', () => {
		const panels = buildDashboards().flatMap((dashboard) => dashboard.panels);
		const ids = new Set(panels.map((panel) => panel.id));
		expect(ids.size).toBe(panels.length);
		for (const panel of panels) {
			expect(panel.metric.startsWith('callfs_')).toBe(true);
			expect(panel.queries.length).toBeGreaterThan(0);
		}
	});

	it('marks the traffic error ratio with the 1 % warning and 5 % critical thresholds', () => {
		const errors = buildDashboards()
			.flatMap((dashboard) => dashboard.panels)
			.find((panel) => 'traffic-errors' === panel.id);
		expect(errors?.thresholds.map((entry) => [entry.tone, entry.value])).toEqual([
			['warn', 1],
			['danger', 5]
		]);
	});
});

describe('panelFromCustom', () => {
	it('maps a saved panel to a single query with its group', () => {
		const definition = panelFromCustom({
			id: 'saved-1',
			title: 'Saved',
			metric: 'callfs_http_requests_total',
			measure: 'rate',
			group: 'path',
			unit: 'per_s',
			chart: 'area',
			thresholds: []
		});
		expect(definition).toMatchObject({
			id: 'saved-1',
			metric: 'callfs_http_requests_total',
			queries: [{ measure: 'rate', group: ['path'], filter: [] }],
			unit: 'per_s',
			chart: 'area'
		});
	});

	it('falls back to the value measure and no unit for unknown saved values', () => {
		const definition = panelFromCustom({
			id: 'saved-2',
			title: 'Saved',
			metric: 'callfs_active_locks',
			measure: 'median',
			group: '',
			unit: 'furlongs',
			chart: 'line',
			thresholds: []
		});
		expect(definition.queries).toEqual([{ measure: 'value', group: [], filter: [] }]);
		expect(definition.unit).toBe('none');
	});
});

describe('aggregate and readings', () => {
	it('sums totals, averages rates, takes the peak for percentiles and ignores gaps', () => {
		const points: [number, number | null][] = [
			[1, 2],
			[2, null],
			[3, 4]
		];
		expect(aggregate(points, 'total')).toBe(6);
		expect(aggregate(points, 'rate')).toBe(3);
		expect(aggregate(points, 'p95')).toBe(4);
		expect(aggregate([[1, null]], 'total')).toBeNull();
	});

	it('reads failures of all series and expresses them as a share of the total', () => {
		const lines = panelLines(statDefinition, statSamples);
		const reading = statReading(statDefinition, lines);
		expect(reading).toEqual({ value: 3, base: { num: 3, den: 193 } });
		expect(measuredValue(reading)).toBeCloseTo((3 / 193) * 100);
	});

	it('reports the plain total when the panel has no failure rule', () => {
		const plain: PanelDefinition = { ...statDefinition, failure: undefined };
		expect(statReading(plain, panelLines(plain, statSamples))).toEqual({ value: 193 });
	});

	it('returns the highest threshold the measured value is above', () => {
		expect(crossedThreshold(statDefinition.thresholds, 6)?.label).toBe('Critical above 5 %');
		expect(crossedThreshold(statDefinition.thresholds, 1.5)?.label).toBe('Warning above 1 %');
		expect(crossedThreshold(statDefinition.thresholds, 0.5)).toBeNull();
		expect(crossedThreshold(statDefinition.thresholds, null)).toBeNull();
	});
});

describe('panel lines', () => {
	it('labels lines by their group values and prefixes the measure when a panel has several queries', () => {
		const single = panelLines(statDefinition, statSamples);
		expect(single.map((line) => line.label)).toEqual(['200', '500']);

		const multi: PanelDefinition = {
			...statDefinition,
			queries: [totalByStatus, { measure: 'max', group: ['status_code'], filter: [] }]
		};
		const samples: PanelSample[] = [...statSamples, { query: multi.queries[1] ?? totalByStatus, result: statSamples[0]?.result ?? emptyResult('x') }];
		expect(panelLines(multi, samples).map((line) => line.label)).toContain('max · 200');
	});

	it('keeps only failing lines for failure-scoped panels', () => {
		const lines = panelLines(statDefinition, statSamples);
		expect(failureLines(statDefinition, lines).map((line) => line.label)).toEqual(['500']);
		expect(matchesFailure({ status: 'success' }, { label: 'status', mode: 'exclude', value: 'success' })).toBe(false);
		expect(matchesFailure({ status: 'expired' }, { label: 'status', mode: 'exclude', value: 'success' })).toBe(true);
	});

	it('builds bars from aggregates and heat cells from non-null points only', () => {
		const lines: PanelLine[] = panelLines(statDefinition, statSamples);
		expect(barData(lines)).toEqual([
			{ label: '200', value: 190 },
			{ label: '500', value: 3 }
		]);
		expect(heatCells(lines)).toEqual([
			{ x: 0, y: 0, value: 90, yLabel: '200' },
			{ x: 1, y: 0, value: 100, yLabel: '200' },
			{ x: 0, y: 1, value: 3, yLabel: '500' }
		]);
	});

	it('lists expected groups that have no series as not reported', () => {
		const definition: PanelDefinition = {
			...statDefinition,
			queries: [{ measure: 'p95', group: ['operation'], filter: [] }],
			chart: 'line',
			expected: ['create', 'read']
		};
		const samples: PanelSample[] = [
			{
				query: definition.queries[0] ?? totalByStatus,
				result: { ...emptyResult('callfs_backend_op_duration_seconds'), series: [{ labels: { operation: 'create' }, points: [] }] }
			}
		];
		expect(missingExpected(definition, panelLines(definition, samples))).toEqual(['read']);
	});
});

describe('annotationsOf', () => {
	it('maps event levels to tones and drops events without a valid time', () => {
		const events: EventRecord[] = [
			{ id: '1', at: '2026-10-23T10:00:00Z', type: 'job.failed', level: 'error', actor: null, instance_id: 'a', message: 'Job failed', entities: [], fields: {} },
			{ id: '2', at: 'not a time', type: 'server.started', level: 'info', actor: null, instance_id: 'a', message: 'Started', entities: [], fields: {} },
			{ id: '3', at: '2026-10-23T10:05:00Z', type: 'config.changed', level: 'warn', actor: null, instance_id: 'a', message: 'Config changed', entities: [], fields: {} }
		];
		expect(annotationsOf(events)).toEqual([
			{ at: Date.parse('2026-10-23T10:00:00Z') / 1000, label: 'Job failed', tone: 'danger' },
			{ at: Date.parse('2026-10-23T10:05:00Z') / 1000, label: 'Config changed', tone: 'warn' }
		]);
	});
});

describe('DashboardsPlanet', () => {
	beforeEach(() => {
		vi.mocked(series).mockResolvedValue({ ok: true, value: emptyResult('callfs_http_requests_total') });
		vi.mocked(metrics).mockResolvedValue({ ok: true, value: { generated_at: '', families: [] } });
		vi.mocked(eventHistory).mockResolvedValue({ ok: true, value: { items: [] } });
	});

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it('renders the page heading, one tab per dashboard and a labelled region for each built-in panel', async () => {
		render(DashboardsPlanet);
		expect(await screen.findByRole('heading', { name: 'Dashboards', level: 1 })).toBeTruthy();
		for (const name of ['Traffic', 'File work', 'Download links', 'Edit locks', 'File index', 'Errors', 'Custom']) {
			expect(screen.getByRole('tab', { name })).toBeTruthy();
		}
		expect(screen.getByRole('region', { name: 'Server errors' })).toBeTruthy();
		expect(screen.getByRole('region', { name: 'Slowest routes' })).toBeTruthy();
	});

	it('shows the custom tab empty state when no panels are saved', async () => {
		render(DashboardsPlanet);
		await fireEvent.click(await screen.findByRole('tab', { name: 'Custom' }));
		expect(await screen.findByText('No custom panels yet.')).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Open Explore' }).getAttribute('href')).toBe('/ui/insights/explore');
	});

	it('shows the query error with a retry action when a panel fails to load', async () => {
		vi.mocked(series).mockResolvedValue({ ok: false, error: { kind: 'network' } });
		render(DashboardsPlanet);
		const region = await screen.findByRole('region', { name: 'Server errors' });
		expect((await screen.findAllByRole('button', { name: 'Retry' })).length).toBeGreaterThan(0);
		expect(region.getAttribute('data-state')).toBe('collapsed');
	});
});
