<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { toUiError } from '$lib/api/errors';
	import { metrics, series } from '$lib/api/admin/telemetry';
	import type { MetricFamily, Result, SeriesResult } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatDuration } from '$lib/format/duration';
	import { formatAbsolute, formatRelative } from '$lib/format/time';
	import { preferences, type DashboardPanel } from '$lib/stores/preferences.svelte';
	import type { ColumnDef, QueryStatus, Series, SortState, UiError, Unit } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import Checkbox from '../../atoms/Checkbox.svelte';
	import CopyButton from '../../atoms/CopyButton.svelte';
	import Select from '../../atoms/Select.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import BytesCell from '../../cells/BytesCell.svelte';
	import NumberCell from '../../cells/NumberCell.svelte';
	import TextCell from '../../cells/TextCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import Combobox from '../../molecules/Combobox.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import FormField from '../../molecules/FormField.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import SegmentedControl from '../../molecules/SegmentedControl.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import BarChart from '../../organisms/chart/BarChart.svelte';
	import Heatmap from '../../organisms/chart/Heatmap.svelte';
	import TimeSeriesChart from '../../organisms/chart/TimeSeriesChart.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import { download, toCSV, toJSON } from '../../organisms/table/export';
	import Icon from '../../quarks/Icon.svelte';
	import RangeBar, { insightsRange } from './RangeBar.svelte';
	import { processStartedAt } from './dashboards';
	import {
		EXPLORE_CHARTS,
		EXPLORE_MEASURES,
		EXPLORE_STEPS,
		LIVE_REFRESH_MS,
		STARTER_QUESTIONS,
		barData,
		chartSeries,
		heatCells,
		hasBuckets,
		isFilter,
		measuresFor,
		oneOf,
		parseExplore,
		promqlFor,
		ratioSeries,
		resolveMeasure,
			seriesRequest,
		shiftedSeries,
		starterQuery,
		unitOf,
		writeExplore,
		type ExploreQuery,
		type ExploreRatio,
		type StarterQuestion
	} from './explore-state.svelte';

	interface Props {
		class?: string;
	}

	interface ExploreData {
		current: SeriesResult;
		previous: SeriesResult | null;
		shiftSeconds: number;
		window: readonly [number, number];
	}

	interface ExploreRow {
		key: string;
		time: number;
		series: string;
		value: number;
		unit: Unit;
	}

	const COMMANDS_CONTEXT = 'callfs.commands';
	const IDS = {
		metric: 'explore-metric',
		measure: 'explore-measure',
		group: 'explore-group',
		filter: 'explore-filter',
		step: 'explore-step',
		chart: 'explore-chart',
		numerator: 'explore-numerator',
		denominator: 'explore-denominator'
	};

	let { class: className }: Props = $props();

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	const familiesQuery = createQuery<Result<MetricFamily[], UiError>>({
		key: ['metrics'],
		fetch: loadFamilies
	});

	async function loadFamilies(): Promise<Result<MetricFamily[], UiError>> {
		const result = await metrics();
		if (!result.ok) {
			return { ok: false, error: toUiError(result.error, { action: 'explore.action.families' }) };
		}
		return { ok: true, value: result.value.families };
	}

	const familyResult = $derived(familiesQuery.data);
	const families = $derived<MetricFamily[]>(undefined !== familyResult && familyResult.ok ? familyResult.value : []);
	const familiesError = $derived(undefined !== familyResult && !familyResult.ok ? familyResult.error : null);

	const store = insightsRange();
	const explore = $derived<ExploreQuery>({
		...parseExplore(page.url.searchParams, Date.now()),
		range: { from: store.value.from, to: store.value.to, preset: store.value.preset, live: store.value.live }
	});
	const family = $derived(families.find((entry) => entry.name === explore.metric));
	const measure = $derived(resolveMeasure(explore.measure, family?.type));
	const measureOptions = $derived(measuresFor(family?.type).map((option) => ({ value: option, label: t(`explore.measure.${option}`) })));
	const labelKeys = $derived(
		[...new Set((family?.series ?? []).flatMap((entry) => Object.keys(entry.labels)))].filter((key) => 'le' !== key).sort()
	);
	const filterItems = $derived(
		[...new Set((family?.series ?? []).flatMap((entry) => Object.entries(entry.labels).map(([key, value]) => `${key}=${value}`)))].map(
			(value) => ({ value, label: value })
		)
	);

	const exploreQuery = createQuery<Result<ExploreData, UiError>>(() => ({
		key: ['explore', seriesRequest(explore, family, Date.now()), explore.compare],
		fetch: () => fetchExplore(explore, family),
		enabled: () => '' !== explore.metric,
		refetchMs: explore.range.live ? LIVE_REFRESH_MS : undefined,
		keepPrevious: true
	}));

	async function fetchExplore(query: ExploreQuery, metricFamily: MetricFamily | undefined): Promise<Result<ExploreData, UiError>> {
		const request = seriesRequest(query, metricFamily, Date.now());
		const current = await series(request);
		if (!current.ok) {
			return { ok: false, error: toUiError(current.error, { action: 'explore.action.load' }) };
		}
		const shiftSeconds = request.to - request.from;
		if (!query.compare) {
			return { ok: true, value: { current: current.value, previous: null, shiftSeconds, window: [request.from, request.to] } };
		}
		const previous = await series({ ...request, from: request.from - shiftSeconds, to: request.from });
		if (!previous.ok) {
			return { ok: false, error: toUiError(previous.error, { action: 'explore.action.load' }) };
		}
		return { ok: true, value: { current: current.value, previous: previous.value, shiftSeconds, window: [request.from, request.to] } };
	}

	const loaded = $derived(exploreQuery.data);
	const payload = $derived(undefined !== loaded && loaded.ok ? loaded.value : undefined);
	const loadError: UiError | null = $derived(undefined !== loaded && !loaded.ok ? loaded.error : undefined === loaded ? exploreQuery.error : null);
	const partialSince = $derived(payload?.current.sampled_since ?? null);
	const unit = $derived<Unit>(payload ? unitOf(payload.current.unit) : 'none');
	const baseSeries = $derived<Series[]>(payload ? chartSeries(payload.current, explore.metric) : []);
	const shareSeries = $derived.by((): Series | null => {
		if (null === explore.ratio) {
			return null;
		}
		const numerator = baseSeries.find((entry) => explore.ratio?.numerator === entry.id);
		const denominator = baseSeries.find((entry) => explore.ratio?.denominator === entry.id);
		if (undefined === numerator || undefined === denominator) {
			return null;
		}
		return ratioSeries(numerator, denominator, t('explore.ratio.title', { numerator: numerator.label, denominator: denominator.label }));
	});
	const shown = $derived<Series[]>(null === shareSeries ? baseSeries : [shareSeries]);
	const overlay = $derived<Series[]>(
		undefined !== payload && null !== payload.previous && null === shareSeries
			? shiftedSeries(chartSeries(payload.previous, explore.metric), payload.shiftSeconds, t('explore.compare.suffix'))
			: []
	);
	const chartList = $derived<Series[]>([...shown, ...overlay]);
	const seriesOptions = $derived(baseSeries.map((entry) => ({ value: entry.id, label: entry.label })));
	const bars = $derived(barData(shown, explore.measure));
	const cells = $derived(undefined !== payload ? heatCells(payload.current) : []);
	const heatmapReady = $derived(undefined !== payload && hasBuckets(payload.current));
	const rows = $derived<ExploreRow[]>(
		shown.flatMap((entry) =>
			entry.points.flatMap(([time, value]) =>
				null === value ? [] : [{ key: `${entry.id}@${time}`, time, series: entry.label, value, unit: entry.unit }]
			)
		)
	);

	const chartStatus: QueryStatus = $derived(
		null !== loadError
			? 'error'
			: undefined === loaded
				? 'loading'
				: 0 === shown.length
					? 'empty'
					: 'success'
	);
	const tableStatus: QueryStatus = $derived(
		null !== loadError ? 'error' : undefined === loaded ? 'loading' : 0 === rows.length ? 'empty' : 'success'
	);
	const emptyReason = $derived(false === family?.emitted ? 'not_reported' : 'no_samples');
	const startedAt = $derived(processStartedAt(families));
	const emptyCopy = $derived(
		'not_reported' === emptyReason
			? {
					title: t('dashboards.empty.since_start.title'),
					body:
						null === startedAt
							? t('dashboards.empty.since_start.body_unknown')
							: t('dashboards.empty.since_start.body', { ago: formatRelative(startedAt * 1000) })
				}
			: undefined
	);

	const promql = $derived(promqlFor(explore, family));
	const shareUrl = $derived(page.url.href);
	const summary = $derived(
		t('explore.summary', {
			measure: t(`explore.measure.${measure}`),
			metric: explore.metric,
			scope: 0 < explore.by.length ? t('explore.summary.scope', { labels: explore.by.join(', ') }) : '',
			range: t('explore.summary.range', { duration: formatDuration(explore.range.to - explore.range.from, 'long') }),
			step: t('explore.summary.step', { step: formatDuration(seriesRequest(explore, family, Date.now()).stepS * 1000, 'long') })
		})
	);
	const dashboardChart = $derived<DashboardPanel['chart'] | null>(
		'line' === explore.chart ? 'line' : 'stacked' === explore.chart ? 'area' : 'bars' === explore.chart ? 'bar' : null
	);
	const columns: ColumnDef<ExploreRow>[] = [
		{ id: 'time', header: t('explore.table.time'), cell: timeCell, sortKey: 'time' },
		{ id: 'series', header: t('explore.table.series'), cell: seriesCell, sortKey: 'series' },
		{ id: 'value', header: t('explore.table.value'), cell: valueCell, sortKey: 'value', align: 'end' }
	];

	let sort = $state<SortState | null>(null);

	const starters = STARTER_QUESTIONS;

	function commit(next: ExploreQuery): void {
		const url = new URL(page.url);
		url.search = writeExplore(next).toString();
		void goto(url, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function run(): void {
		void exploreQuery.refetch();
	}

	function start(starter: StarterQuestion): void {
		commit(starterQuery(starter, explore.range));
	}

	function chooseMetric(values: string[]): void {
		commit({ ...explore, metric: values[0] ?? '', by: [], filters: [], ratio: null });
	}

	function chooseRatio(numerator: string, denominator: string): ExploreRatio | null {
		return '' === numerator || '' === denominator ? null : { numerator, denominator };
	}

	function exportRows(format: 'csv' | 'json'): void {
		const body = 'csv' === format ? toCSV(rows, columns) : toJSON(rows, columns);
		const type = 'csv' === format ? 'text/csv' : 'application/json';
		download(`callfs-explore.${format}`, new Blob([body], { type }));
	}

	function addToDashboard(): void {
		if (null === dashboardChart || '' === explore.metric) {
			return;
		}
		const panel: DashboardPanel = {
			id: `explore-${Date.now().toString(36)}`,
			title: summary,
			metric: explore.metric,
			measure: explore.measure,
			group: explore.by.join(','),
			unit,
			chart: dashboardChart,
			thresholds: []
		};
		preferences.update({ dashboards: { custom: [...preferences.value.dashboards.custom, panel] } });
		announce(t('explore.dashboard.added'), 'polite');
	}

	function unitLabel(value: Unit): string | undefined {
		return 'none' === value ? undefined : t(`chart.unit.${value}`);
	}

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'insights.explore.run',
				title: t('explore.command.run'),
				group: 'Insights',
				keywords: [t('explore.title')],
				icon: 'search',
				run: run
			},
			{
				id: 'insights.explore.dashboard',
				title: t('explore.command.dashboard'),
				group: 'Insights',
				keywords: [t('explore.title')],
				icon: 'plus',
				when: () => null !== dashboardChart,
				run: addToDashboard
			}
		]);
	});
</script>

{#snippet timeCell(row: ExploreRow)}
	<TimeCell value={new Date(row.time * 1000).toISOString()} />
{/snippet}

{#snippet seriesCell(row: ExploreRow)}
	<TextCell value={row.series} />
{/snippet}

{#snippet valueCell(row: ExploreRow)}
	{#if 'bytes' === row.unit}
		<BytesCell value={row.value} />
	{:else}
		<NumberCell value={row.value} unit={unitLabel(row.unit)} />
	{/if}
{/snippet}

{#snippet emptyTable()}
	<EmptyState
		variant="no-results"
		icon="search"
		title={emptyCopy?.title ?? t('explore.table.empty.title')}
		body={emptyCopy?.body ?? t('explore.table.empty.body')}
	/>
{/snippet}

<PageFrame title={t('explore.title')} job={t('explore.job')} class={className}>
	{#snippet status()}
		{#if '' !== explore.metric}
			<StaleIndicator
				updatedAt={exploreQuery.updatedAt}
				intervalMs={LIVE_REFRESH_MS}
				state={explore.range.live ? 'live' : 'paused'}
				onrefresh={run}
			/>
		{/if}
	{/snippet}

	{#snippet primary()}
		<Button variant="primary" iconStart="play" disabledReason={'' === explore.metric ? t('explore.run.disabled') : undefined} onclick={run}>
			{t('explore.run')}
		</Button>
	{/snippet}

	{#snippet toolbar()}
		<RangeBar {store} />
	{/snippet}

	<div class="explore">
		<section class="card query" aria-labelledby="explore-query-title">
			<h2 class="card-title" id="explore-query-title">{t('insights.explore.query')}</h2>
			{#if null !== familiesError}
				<InlineAlert tone="danger" title={familiesError.title} body={familiesError.body} />
			{/if}

			<div class="field">
				<Combobox
					id={IDS.metric}
					aria-describedby={`${IDS.metric}-help`}
					label={t('explore.metric.label')}
					placeholder={t('explore.metric.placeholder')}
					items={families.map((entry) => ({ value: entry.name, label: entry.name }))}
					bind:value={() => ('' === explore.metric ? [] : [explore.metric]), chooseMetric}
				/>
				<p class="help" id={`${IDS.metric}-help`}>{family?.help ?? t('explore.metric.help')}</p>
			</div>

			<FormField label={t('explore.measure.label')} for={IDS.measure}>
				{#snippet control({ describedBy })}
					<Select
						id={IDS.measure}
						aria-describedby={describedBy}
						options={measureOptions}
						bind:value={() => explore.measure, (next) => commit({ ...explore, measure: oneOf(EXPLORE_MEASURES, next, explore.measure) })}
					/>
				{/snippet}
			</FormField>

			<div class="field">
				<Combobox
					id={IDS.group}
					aria-describedby={`${IDS.group}-help`}
					label={t('explore.group.label')}
					placeholder={t('explore.group.placeholder')}
					multiple
					items={labelKeys.map((key) => ({ value: key, label: key }))}
					bind:value={() => explore.by, (next) => commit({ ...explore, by: next })}
				/>
				<p class="help" id={`${IDS.group}-help`}>{t('explore.group.help')}</p>
			</div>

			<div class="field">
				<Combobox
					id={IDS.filter}
					aria-describedby={`${IDS.filter}-help`}
					label={t('explore.filter.label')}
					placeholder={t('explore.filter.placeholder')}
					multiple
					creatable
					items={filterItems}
					bind:value={() => explore.filters, (next) => commit({ ...explore, filters: next.filter(isFilter) })}
				/>
				<p class="help" id={`${IDS.filter}-help`}>{t('explore.filter.help')}</p>
			</div>

			<FormField label={t('explore.step.label')} for={IDS.step} help={t('explore.step.help')}>
				{#snippet control({ describedBy })}
					<Select
						id={IDS.step}
						aria-describedby={describedBy}
						options={EXPLORE_STEPS.map((step) => ({ value: step, label: t(`explore.step.${step}`) }))}
						bind:value={() => explore.step, (next) => commit({ ...explore, step: oneOf(EXPLORE_STEPS, next, explore.step) })}
					/>
				{/snippet}
			</FormField>

			<details class="more">
				<summary>{t('insights.explore.more')}</summary>
				<div class="more-body">
					<Checkbox
						label={t('explore.compare.label')}
						bind:checked={() => explore.compare, (next) => commit({ ...explore, compare: next })}
					/>
					{#if 1 < baseSeries.length}
						<FormField label={t('explore.ratio.numerator')} for={IDS.numerator} help={t('explore.ratio.help')}>
							{#snippet control({ describedBy })}
								<Select
									id={IDS.numerator}
									aria-describedby={describedBy}
									options={[{ value: '', label: t('explore.ratio.off') }, ...seriesOptions]}
									bind:value={
										() => explore.ratio?.numerator ?? '',
										(next) => commit({ ...explore, ratio: chooseRatio(next, explore.ratio?.denominator ?? '') })
									}
								/>
							{/snippet}
						</FormField>
						<FormField label={t('explore.ratio.denominator')} for={IDS.denominator}>
							{#snippet control({ describedBy })}
								<Select
									id={IDS.denominator}
									aria-describedby={describedBy}
									options={[{ value: '', label: t('explore.ratio.off') }, ...seriesOptions]}
									bind:value={
										() => explore.ratio?.denominator ?? '',
										(next) => commit({ ...explore, ratio: chooseRatio(explore.ratio?.numerator ?? '', next) })
									}
								/>
							{/snippet}
						</FormField>
					{/if}
				</div>
			</details>
		</section>

		<div class="results">
			{#if '' === explore.metric}
				<section class="card starters" aria-labelledby="explore-starters-title">
					<div class="starter-head">
						<span class="starter-icon" aria-hidden="true"><Icon name="line-chart" /></span>
						<h2 class="card-title" id="explore-starters-title">{t('explore.empty.title')}</h2>
						<p class="muted">{t('insights.explore.empty_body')}</p>
					</div>
					<ul class="starter-list" aria-label={t('insights.explore.starters')}>
						{#each starters as starter (starter.copy)}
							<li>
								<button type="button" class="starter" onclick={() => start(starter)}>
									<span>{t(starter.copy)}</span>
									<Icon name="arrow-right" />
								</button>
							</li>
						{/each}
					</ul>
				</section>
			{:else}
				<section class="card" aria-labelledby="explore-result-title">
					<header class="result-head">
						<div class="titles">
							<h2 class="card-title" id="explore-result-title">{t('insights.explore.result')}</h2>
							<p class="muted">{summary}</p>
						</div>
						<SegmentedControl
							label={t('explore.chart.label')}
							options={EXPLORE_CHARTS.map((chart) => ({ value: chart, label: t(`explore.chart.${chart}`) }))}
							bind:value={() => explore.chart, (next) => commit({ ...explore, chart: oneOf(EXPLORE_CHARTS, next, explore.chart) })}
						/>
					</header>

					{#if null !== partialSince && null === loadError}
						<InlineAlert tone="warn" title={t('explore.partial.title')} body={t('explore.partial.body', { since: formatAbsolute(partialSince) })} />
					{/if}

					{#if null !== loadError}
						<ErrorState error={loadError} onretry={run} />
					{:else if 'table' !== explore.chart}
						<div class="chart">
							{#if 'bars' === explore.chart}
								{#if undefined === payload}
									<Skeleton shape="rect" height="100%" />
								{:else}
									<BarChart data={bars} {unit} label={t('explore.chart.bars_label')} />
								{/if}
							{:else if 'heatmap' === explore.chart}
								{#if heatmapReady}
									<Heatmap {cells} xUnit="s" yUnit={unit} label={t('explore.chart.heatmap_label')} scale="seq" />
								{:else if undefined !== payload}
									<InlineAlert
										tone="info"
										title={t('explore.chart.heatmap_unavailable.title')}
										body={t('explore.chart.heatmap_unavailable.body')}
									/>
								{:else}
									<Skeleton shape="rect" height="100%" />
								{/if}
							{:else}
								<TimeSeriesChart
									series={chartList}
									domain={payload?.window}
									{unit}
									stacked={'stacked' === explore.chart}
									status={chartStatus}
									error={loadError ?? undefined}
									{emptyReason}
									{emptyCopy}
									onretry={run}
								/>
							{/if}
						</div>
					{/if}

					<footer class="result-actions" aria-label={t('insights.explore.share')}>
						<CopyButton variant="button" value={shareUrl} label={t('explore.copy_link')} />
						<CopyButton variant="button" value={promql} label={t('explore.copy_promql')} />
						<Button
							variant="secondary"
							size="sm"
							iconStart="plus"
							disabledReason={null === dashboardChart ? t('explore.dashboard.unavailable') : undefined}
							onclick={addToDashboard}
						>
							{t('explore.dashboard.add')}
						</Button>
					</footer>
				</section>

				<DataTable
					{rows}
					{columns}
					rowKey={(row: ExploreRow) => row.key}
					label={t('explore.table.label')}
					caption={t('explore.table.label')}
					status={tableStatus}
					error={loadError ?? undefined}
					bind:sort
					onexport={exportRows}
					onretry={run}
					empty={emptyTable}
				/>
			{/if}
		</div>
	</div>
</PageFrame>

<style>
	.explore {
		display: grid;
		grid-template-columns: minmax(0, calc(var(--space-4) * 20)) minmax(0, 1fr);
		align-items: start;
		gap: var(--space-6);
	}

	.results {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
		padding: var(--card-pad);
		background: var(--surface-raised);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xs);
	}

	.query {
		position: sticky;
		inset-block-start: var(--space-6);
	}

	.card-title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
	}

	.muted {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.help {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
	}

	.more {
		border-block-start: var(--hairline) solid var(--border-subtle);
		padding-block-start: var(--space-3);
	}

	.more summary {
		cursor: pointer;
		inline-size: fit-content;
		border-radius: var(--radius-sm);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: 500;
		color: var(--text-link);
	}

	.more summary:focus-visible {
		outline: var(--focus-ring);
		outline-offset: var(--space-0-5);
	}

	.more-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding-block-start: var(--space-4);
	}

	.result-head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-3) var(--space-4);
	}

	.titles {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
		flex: 1 1 calc(var(--space-4) * 16);
	}

	.chart {
		display: flex;
		flex-direction: column;
		min-block-size: calc(var(--space-4) * 18);
		min-inline-size: 0;
	}

	.result-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding-block-start: var(--space-4);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.starters {
		gap: var(--space-6);
		padding: var(--space-8) var(--card-pad);
	}

	.starter-head {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-2);
		text-align: center;
	}

	.starter-icon {
		display: grid;
		place-items: center;
		inline-size: var(--space-10);
		block-size: var(--space-10);
		border-radius: var(--radius-md);
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	.starter-list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, calc(var(--space-4) * 16)), 1fr));
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.starter {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		inline-size: 100%;
		min-block-size: var(--touch-target);
		padding: var(--space-3) var(--space-4);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface);
		color: var(--text);
		font: inherit;
		text-align: start;
		cursor: pointer;
		transition:
			background var(--dur-micro) var(--ease-standard),
			border-color var(--dur-micro) var(--ease-standard);
	}

	.starter :global(svg) {
		flex: none;
		color: var(--text-muted);
	}

	@media (hover: hover) {
		.starter:hover {
			background: var(--surface-hover);
			border-color: var(--border);
		}
	}

	.starter:focus-visible {
		outline: var(--focus-ring);
		outline-offset: var(--space-0-5);
	}

	@media (width < 1024px) {
		.explore {
			grid-template-columns: minmax(0, 1fr);
		}

		.query {
			position: static;
		}

		.results:has(.starters) {
			order: -1;
		}
	}
</style>
