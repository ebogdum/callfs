import type { SeriesMeasure } from '$lib/api/admin/telemetry';
import type { Event as EventRecord, MetricFamily, SeriesResult } from '$lib/api/types';
import { t } from '$lib/copy/t';
import type { DashboardPanel } from '$lib/stores/preferences.svelte';
import { formatChartValue, type BarDatum } from '$lib/ui/organisms/chart/BarChart.svelte';
import type { HeatCell } from '$lib/ui/organisms/chart/Heatmap.svelte';
import type { Annotation, Series, Threshold, Tone, Unit } from '$lib/ui/types';

export const POLL_MS = 10_000;

const POINT_BUDGET = 240;
const MIN_STEP_S = 5;

const MEASURES: readonly string[] = ['rate', 'total', 'avg', 'p50', 'p95', 'p99', 'max', 'value'];
const UNITS: readonly string[] = ['bytes', 'bytes_per_s', 'count', 'per_s', 'ms', 's', 'percent', 'ratio', 'none'];
const EVENT_TYPES = ['config.changed', 'cluster.member_joined', 'server.started', 'job.failed'];

const HTTP_REQUESTS = 'callfs_http_requests_total';
const HTTP_DURATION = 'callfs_http_request_duration_seconds';
const LINK_OPENS = 'callfs_single_use_link_consumptions_total';
const FAILURE_CLASS_5XX = '5';

export type DashboardId = 'traffic' | 'file-work' | 'download-links' | 'edit-locks' | 'file-index' | 'errors' | 'custom';
export type PanelChart = 'line' | 'area' | 'stacked' | 'bar' | 'stat' | 'heatmap';
export type PanelSpan = 3 | 4 | 6 | 8 | 12;
export type SeriesPoint = [number, number | null];

export interface TimeRange {
	from: number;
	to: number;
	live: boolean;
}

export interface PanelQuery {
	measure: SeriesMeasure;
	group: string[];
	filter: string[];
}

export interface FailureRule {
	label: string;
	mode: 'prefix' | 'exclude';
	value: string;
}

export interface PanelDefinition {
	id: string;
	title: string;
	metric: string;
	queries: PanelQuery[];
	unit: Unit;
	thresholds: Threshold[];
	chart: PanelChart;
	span: PanelSpan;
	failure?: FailureRule;
	expected?: string[];
}

export interface DashboardDefinition {
	id: DashboardId;
	label: string;
	panels: PanelDefinition[];
}

export interface PanelSample {
	query: PanelQuery;
	result: SeriesResult;
}

export interface PanelLine {
	id: string;
	label: string;
	measure: SeriesMeasure;
	labels: Record<string, string>;
	points: SeriesPoint[];
}

export interface StatReading {
	value: number | null;
	base?: { num: number; den: number };
}

export function stepFor(range: TimeRange): number {
	return Math.max(MIN_STEP_S, Math.ceil((range.to - range.from) / POINT_BUDGET));
}

const isMeasure = (value: string): value is SeriesMeasure => MEASURES.includes(value);
const isUnit = (value: string): value is Unit => UNITS.includes(value);
const measureOf = (value: string): SeriesMeasure => (isMeasure(value) ? value : 'value');
const unitOf = (value: string): Unit => (isUnit(value) ? value : 'none');

export function panelFromCustom(panel: DashboardPanel): PanelDefinition {
	return {
		id: panel.id,
		title: panel.title,
		metric: panel.metric,
		queries: [{ measure: measureOf(panel.measure), group: '' === panel.group ? [] : [panel.group], filter: [] }],
		unit: unitOf(panel.unit),
		thresholds: panel.thresholds,
		chart: panel.chart,
		span: 6
	};
}

const query = (measure: SeriesMeasure, group: string[] = [], filter: string[] = []): PanelQuery => ({ measure, group, filter });

const threshold = (value: number, tone: Threshold['tone'], unit: Unit): Threshold => ({
	value,
	tone,
	label: t(`dashboards.threshold.${tone}`, { value: formatChartValue(value, unit) })
});

export function buildDashboards(): DashboardDefinition[] {
	return [
		{
			id: 'traffic',
			label: t('dashboards.tab.traffic'),
			panels: [
				{ id: 'traffic-rate', title: t('dashboards.panel.traffic.rate'), metric: HTTP_REQUESTS, queries: [query('rate', ['status_code'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 8 },
				{
					id: 'traffic-errors',
					title: t('dashboards.panel.traffic.errors'),
					metric: HTTP_REQUESTS,
					queries: [query('total', ['status_code'])],
					unit: 'count',
					thresholds: [threshold(1, 'warn', 'percent'), threshold(5, 'danger', 'percent')],
					chart: 'stat',
					span: 4,
					failure: { label: 'status_code', mode: 'prefix', value: FAILURE_CLASS_5XX }
				},
				{ id: 'traffic-top-routes', title: t('dashboards.panel.traffic.top_routes'), metric: HTTP_REQUESTS, queries: [query('total', ['path'])], unit: 'count', thresholds: [], chart: 'bar', span: 6 },
				{
					id: 'traffic-latency',
					title: t('insights.panel.latency'),
					metric: HTTP_DURATION,
					queries: [query('p50'), query('p95'), query('p99')],
					unit: 's',
					thresholds: [],
					chart: 'line',
					span: 6
				},
				{
					id: 'traffic-slowest',
					title: t('dashboards.panel.traffic.slowest'),
					metric: HTTP_DURATION,
					queries: [query('p95', ['path'])],
					unit: 's',
					thresholds: [threshold(1, 'warn', 's'), threshold(5, 'danger', 's')],
					chart: 'bar',
					span: 6
				},
				{
					id: 'traffic-unmatched',
					title: t('dashboards.panel.traffic.unmatched'),
					metric: HTTP_REQUESTS,
					queries: [query('rate', ['path'], ['status_code=404'])],
					unit: 'per_s',
					thresholds: [threshold(1, 'warn', 'per_s')],
					chart: 'line',
					span: 6
				},
				{ id: 'traffic-heatmap', title: t('insights.panel.heatmap'), metric: HTTP_DURATION, queries: [query('rate', ['le'])], unit: 's', thresholds: [], chart: 'heatmap', span: 12 }
			]
		},
		{
			id: 'file-work',
			label: t('dashboards.tab.file_work'),
			panels: [
				{ id: 'file-operations', title: t('dashboards.panel.file_work.operations'), metric: 'callfs_file_operations_total', queries: [query('rate', ['operation'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 },
				{ id: 'storage-operations', title: t('dashboards.panel.file_work.storage_operations'), metric: 'callfs_backend_ops_total', queries: [query('rate', ['operation'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 },
				{
					id: 'storage-time',
					title: t('dashboards.panel.file_work.storage_time'),
					metric: 'callfs_backend_op_duration_seconds',
					queries: [query('p95', ['operation'])],
					unit: 's',
					thresholds: [],
					chart: 'line',
					span: 6,
					expected: ['create', 'open', 'update', 'delete', 'move']
				},
				{ id: 'transfers-active', title: t('dashboards.panel.file_work.transfers'), metric: 'callfs_transfers_active', queries: [query('value', ['kind'])], unit: 'count', thresholds: [], chart: 'line', span: 6 }
			]
		},
		{
			id: 'download-links',
			label: t('dashboards.tab.download_links'),
			panels: [
				{ id: 'links-created', title: t('dashboards.panel.links.created'), metric: 'callfs_single_use_link_generations_total', queries: [query('rate')], unit: 'per_s', thresholds: [], chart: 'line', span: 4 },
				{ id: 'links-opened', title: t('dashboards.panel.links.opened'), metric: LINK_OPENS, queries: [query('rate', ['status'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 4 },
				{
					id: 'links-failed',
					title: t('dashboards.panel.links.failed'),
					metric: LINK_OPENS,
					queries: [query('total', ['status'])],
					unit: 'count',
					thresholds: [threshold(20, 'warn', 'percent')],
					chart: 'stat',
					span: 4,
					failure: { label: 'status', mode: 'exclude', value: 'success' }
				},
				{ id: 'links-active', title: t('dashboards.panel.links.active'), metric: 'callfs_single_use_links_active', queries: [query('value')], unit: 'count', thresholds: [], chart: 'line', span: 12 }
			]
		},
		{
			id: 'edit-locks',
			label: t('dashboards.tab.edit_locks'),
			panels: [
				{ id: 'locks-active', title: t('dashboards.panel.locks.active'), metric: 'callfs_active_locks', queries: [query('value')], unit: 'count', thresholds: [], chart: 'line', span: 6 },
				{ id: 'locks-operations', title: t('dashboards.panel.locks.operations'), metric: 'callfs_lock_operations_total', queries: [query('rate', ['status'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 },
				{
					id: 'locks-wait',
					title: t('dashboards.panel.locks.wait'),
					metric: 'callfs_lock_operation_duration_seconds',
					queries: [query('p95', ['operation'])],
					unit: 's',
					thresholds: [threshold(1, 'warn', 's')],
					chart: 'line',
					span: 6
				},
				{ id: 'locks-timeouts', title: t('dashboards.panel.locks.timeouts'), metric: 'callfs_lock_operations_total', queries: [query('total', [], ['status=timeout'])], unit: 'count', thresholds: [], chart: 'line', span: 6 }
			]
		},
		{
			id: 'file-index',
			label: t('dashboards.tab.file_index'),
			panels: [
				{ id: 'index-queries', title: t('dashboards.panel.index.queries'), metric: 'callfs_metadata_db_queries_total', queries: [query('rate', ['operation'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 },
				{
					id: 'index-query-time',
					title: t('dashboards.panel.index.query_time'),
					metric: 'callfs_metadata_db_query_duration_seconds',
					queries: [query('p95', ['operation'])],
					unit: 's',
					thresholds: [threshold(0.1, 'warn', 's'), threshold(0.5, 'danger', 's')],
					chart: 'line',
					span: 6
				},
				{ id: 'index-nats-routes', title: t('storage-insights.dashboard.nats_routes'), metric: 'callfs_nats_routes', queries: [query('value')], unit: 'count', thresholds: [], chart: 'line', span: 4 },
				{ id: 'index-nats-log', title: t('storage-insights.dashboard.nats_log'), metric: 'callfs_nats_stream_messages', queries: [query('value', ['stream'])], unit: 'count', thresholds: [], chart: 'line', span: 4 },
				{ id: 'index-cache', title: t('dashboards.panel.index.cache'), metric: 'callfs_cache_lookups_total', queries: [query('rate', ['result'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 4 }
			]
		},
		{
			id: 'errors',
			label: t('dashboards.tab.errors'),
			panels: [
				{ id: 'errors-by-part', title: t('dashboards.panel.errors.by_part'), metric: 'callfs_errors_total', queries: [query('rate', ['component', 'error_type'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 },
				{
					id: 'errors-server-by-route',
					title: t('dashboards.panel.errors.server_by_route'),
					metric: HTTP_REQUESTS,
					queries: [query('total', ['path', 'status_code'])],
					unit: 'count',
					thresholds: [],
					chart: 'bar',
					span: 6,
					failure: { label: 'status_code', mode: 'prefix', value: FAILURE_CLASS_5XX }
				},
				{ id: 'errors-limits', title: t('dashboards.panel.errors.limits'), metric: 'callfs_rate_limit_rejections_total', queries: [query('rate', ['endpoint'])], unit: 'per_s', thresholds: [], chart: 'line', span: 6 },
				{ id: 'errors-sign-ins', title: t('dashboards.panel.errors.sign_ins'), metric: 'callfs_auth_failures_total', queries: [query('rate', ['reason'])], unit: 'per_s', thresholds: [], chart: 'stacked', span: 6 }
			]
		}
	];
}

export function metricNames(panels: readonly PanelDefinition[]): string[] {
	return [...new Set(panels.map((panel) => panel.metric))];
}

export const PROCESS_START_METRIC = 'process_start_time_seconds';

export function processStartedAt(families: readonly MetricFamily[]): number | null {
	const value = families.find((family) => PROCESS_START_METRIC === family.name)?.series[0]?.value;
	return undefined !== value && 0 < value ? value : null;
}

const toneOf = (level: EventRecord['level']): Tone => ('error' === level ? 'danger' : 'warn' === level ? 'warn' : 'info');

export function annotationsOf(events: readonly EventRecord[]): Annotation[] {
	return events.flatMap((event) => {
		const at = Date.parse(event.at) / 1000;
		return Number.isFinite(at) ? [{ at, label: event.message, tone: toneOf(event.level) }] : [];
	});
}

export const ANNOTATION_EVENT_TYPES: readonly string[] = EVENT_TYPES;

export function panelLines(definition: PanelDefinition, samples: readonly PanelSample[]): PanelLine[] {
	const prefixed = 1 < definition.queries.length;
	return samples.flatMap(({ query: sampleQuery, result }) =>
		result.series.map((line): PanelLine => {
			const group = sampleQuery.group
				.map((key) => line.labels[key] ?? '')
				.filter((value) => '' !== value)
				.join(' · ');
			const name = '' === group ? definition.title : group;
			return {
				id: `${sampleQuery.measure}|${JSON.stringify(line.labels)}`,
				label: prefixed ? ('' === group ? sampleQuery.measure : `${sampleQuery.measure} · ${name}`) : name,
				measure: sampleQuery.measure,
				labels: line.labels,
				points: line.points
			};
		})
	);
}

export function matchesFailure(labels: Record<string, string>, rule: FailureRule): boolean {
	const value = labels[rule.label] ?? '';
	return 'prefix' === rule.mode ? value.startsWith(rule.value) : rule.value !== value;
}

export function failureLines(definition: PanelDefinition, lines: PanelLine[]): PanelLine[] {
	const rule = definition.failure;
	return undefined === rule ? lines : lines.filter((line) => matchesFailure(line.labels, rule));
}

export function aggregate(points: readonly SeriesPoint[], measure: SeriesMeasure): number | null {
	const values = points.flatMap(([, value]) => (null === value ? [] : [value]));
	if (0 === values.length) {
		return null;
	}
	const sum = values.reduce((total, value) => total + value, 0);
	if ('total' === measure) {
		return sum;
	}
	if ('rate' === measure) {
		return sum / values.length;
	}
	return values.reduce((highest, value) => Math.max(highest, value));
}

const sumDefined = (values: readonly (number | null)[]): number | null => {
	const defined = values.flatMap((value) => (null === value ? [] : [value]));
	return 0 === defined.length ? null : defined.reduce((total, value) => total + value, 0);
};

export function statReading(definition: PanelDefinition, lines: PanelLine[]): StatReading {
	const total = sumDefined(lines.map((line) => aggregate(line.points, line.measure)));
	if (undefined === definition.failure) {
		return { value: total };
	}
	const failed = sumDefined(failureLines(definition, lines).map((line) => aggregate(line.points, line.measure)));
	return { value: failed, base: { num: failed ?? 0, den: total ?? 0 } };
}

export function measuredValue(reading: StatReading): number | null {
	const base = reading.base;
	if (undefined === base) {
		return reading.value;
	}
	return 0 < base.den ? (base.num / base.den) * 100 : null;
}

export function crossedThreshold(thresholds: readonly Threshold[], measured: number | null): Threshold | null {
	if (null === measured) {
		return null;
	}
	return thresholds
		.filter((candidate) => measured > candidate.value)
		.reduce<Threshold | null>((highest, candidate) => (null === highest || candidate.value > highest.value ? candidate : highest), null);
}

export function chartSeries(definition: PanelDefinition, lines: PanelLine[]): Series[] {
	return lines.map((line, index) => ({ id: line.id, label: line.label, unit: definition.unit, points: line.points, colorIndex: index }));
}

export function barData(lines: PanelLine[]): BarDatum[] {
	return lines.flatMap((line) => {
		const value = aggregate(line.points, line.measure);
		return null === value ? [] : [{ label: line.label, value }];
	});
}

export function hasBucket(line: PanelLine): boolean {
	const raw = line.labels.le;
	return undefined !== raw && '' !== raw && Number.isFinite(Number(raw));
}

export function heatCells(lines: PanelLine[]): HeatCell[] {
	return lines.flatMap((line, y) => {
		if (!hasBucket(line)) {
			const yLabel = line.labels.route ?? line.label;
			return line.points.flatMap(([, value], x) => (null === value ? [] : [{ x, y, value, yLabel }]));
		}
		const bound = Number(line.labels.le);
		return line.points.flatMap(([time, value]) => (null === value ? [] : [{ x: time, y: bound, value }]));
	});
}

export function missingExpected(definition: PanelDefinition, lines: PanelLine[]): string[] {
	const expected = definition.expected ?? [];
	const [key] = definition.queries[0]?.group ?? [];
	if (undefined === key) {
		return [];
	}
	const present = new Set(lines.map((line) => line.labels[key]));
	return expected.filter((name) => !present.has(name));
}
