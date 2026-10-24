<script lang="ts" module>
	export const TOKENS = ['--panel-inset', '--panel-body-min'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { series as loadSeries } from '$lib/api/admin/telemetry';
	import { toUiError } from '$lib/api/errors';
	import type { Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { formatDuration } from '$lib/format/duration';
	import { formatRelative, formatTimestamp } from '$lib/format/time';
	import type { Annotation, UiError } from '$lib/ui/types';
	import IconButton from '../../atoms/IconButton.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import BarChart, { formatChartValue } from '../../organisms/chart/BarChart.svelte';
	import Heatmap from '../../organisms/chart/Heatmap.svelte';
	import TimeSeriesChart, { type EmptyReason } from '../../organisms/chart/TimeSeriesChart.svelte';
	import {
		aggregate,
		barData,
		chartSeries,
		crossedThreshold,
		failureLines,
		hasBucket,
		heatCells,
		measuredValue,
		missingExpected,
		panelLines,
		stepFor,
		statReading,
		type PanelDefinition,
		type PanelLine,
		type PanelSample,
		type TimeRange
	} from './dashboards';

	type Outcome = Result<PanelSample[], UiError> | null;
	type PanelRequest = { definition: PanelDefinition; range: TimeRange; generation: number };

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'children'> {
		definition: PanelDefinition;
		range: TimeRange;
		reported: boolean;
		annotations: Annotation[];
		syncKey?: string;
		revision: number;
		startedAt?: number | null;
	}

	type EmptyCopy = { title: string; body: string };

	const REASON_COPY: Record<EmptyReason, string> = {
		not_reported: 'chart.not_reported',
		no_samples: 'chart.no_samples',
		empty_range: 'chart.empty_range'
	};
	const MAX_LINES = 8;
	const UNIT_NOTES: readonly string[] = ['s', 'ms', 'per_s', 'bytes', 'bytes_per_s', 'percent'];
	const MAX_BARS = 10;
	const RECENT_S = 120;

	let { definition, range, reported, annotations, syncKey, revision, startedAt = null, class: className, ...rest }: Props = $props();

	let outcome = $state<Outcome>(null);
	let inspecting = $state(false);
	let expanded = $state(false);
	let localRevision = $state(0);
	let latestGeneration = 0;

	const headingId = $derived(`${definition.id}-title`);
	const samples = $derived(null !== outcome && true === outcome.ok ? outcome.value : null);
	const failure = $derived(null !== outcome && false === outcome.ok ? outcome.error : null);
	const lines = $derived(null === samples ? [] : panelLines(definition, samples));
	const scoped = $derived(failureLines(definition, lines));
	const ranked = $derived(rankLines(scoped));
	const capped = $derived('bar' === definition.chart ? MAX_BARS : MAX_LINES);
	const shown = $derived('heatmap' === definition.chart ? scoped : ranked.slice(0, capped));
	const heatReady = $derived(shown.some(hasBucket));
	const hasData = $derived(lines.some((line) => line.points.some(([, value]) => null !== value)));
	const hasBars = $derived(shown.some((line) => 0 !== (aggregate(line.points, line.measure) ?? 0)));
	const sampledSince = $derived(samples?.[0]?.result.sampled_since ?? null);
	const reason = $derived<EmptyReason>(false === reported ? 'not_reported' : null === sampledSince ? 'no_samples' : 'empty_range');
	const recent = $derived(range.live || RECENT_S > Math.abs(Date.now() / 1000 - range.to));
	const span = $derived(formatDuration((range.to - range.from) * 1000).replace(/^1 (?=\p{L}+$)/u, ''));
	const rangeFrom = $derived(formatTimestamp(range.from * 1000));
	const rangeTo = $derived(formatTimestamp(range.to * 1000));
	const sampledLate = $derived(null !== sampledSince && Date.parse(sampledSince) / 1000 > range.from);
	const emptyCopy = $derived.by((): EmptyCopy | undefined => {
		if ('not_reported' === reason) {
			return {
				title: t('dashboards.empty.since_start.title'),
				body:
					null === startedAt
						? t('dashboards.empty.since_start.body_unknown')
						: t('dashboards.empty.since_start.body', { ago: formatRelative(startedAt * 1000) })
			};
		}
		if ('empty_range' !== reason) {
			return undefined;
		}
		return {
			title: recent ? t('dashboards.empty.last.title', { span }) : t('dashboards.empty.between.title', { from: rangeFrom, to: rangeTo }),
			body:
				null !== sampledSince && sampledLate
					? t('dashboards.empty.range.body_started', { ago: formatRelative(sampledSince) })
					: t('dashboards.empty.range.body')
		};
	});
	const statNone = $derived(
		recent ? t('dashboards.stat.none_last', { span }) : t('dashboards.stat.none_between', { from: rangeFrom, to: rangeTo })
	);
	const missing = $derived(missingExpected(definition, lines));
	const reading = $derived(statReading(definition, lines));
	const crossed = $derived(crossedThreshold(definition.thresholds, measuredValue(reading)));
	const isSeriesChart = $derived('line' === definition.chart || 'area' === definition.chart || 'stacked' === definition.chart);
	const subtitle = $derived(
		'stat' !== definition.chart && capped < ranked.length && 'heatmap' !== definition.chart
			? t('insights.panel.top', { shown: capped, total: ranked.length })
			: UNIT_NOTES.includes(definition.unit)
				? t(`insights.unit.${definition.unit}`)
				: ''
	);
	const share = $derived(
		undefined === reading.base || 0 === reading.base.den ? '' : `${Math.round((reading.base.num / reading.base.den) * 100)}%`
	);

	function rankLines(source: PanelLine[]): PanelLine[] {
		const peak = (line: PanelLine): number => aggregate(line.points, line.measure) ?? 0;
		return [...source].sort((left, right) => peak(right) - peak(left));
	}

	async function load(request: PanelRequest): Promise<void> {
		latestGeneration = request.generation;
		const responses = await Promise.all(
			request.definition.queries.map(async (entry) => ({
				query: entry,
				response: await loadSeries({
					name: request.definition.metric,
					measure: entry.measure,
					by: entry.group,
					filter: entry.filter,
					from: request.range.from,
					to: request.range.to,
					stepS: stepFor(request.range)
				})
			}))
		);
		if (request.generation !== latestGeneration) {
			return;
		}
		const collected: PanelSample[] = [];
		for (const { query, response } of responses) {
			if (false === response.ok) {
				outcome = { ok: false, error: toUiError(response.error, { action: 'dashboards.load' }) };
				return;
			}
			collected.push({ query, result: response.value });
		}
		outcome = { ok: true, value: collected };
	}

	$effect(() => {
		void load({ definition, range, generation: revision + localRevision });
	});

	function reloadNow(): void {
		localRevision += 1;
	}

	function handleKey(event: KeyboardEvent): void {
		if ('Escape' === event.key && expanded) {
			expanded = false;
		}
	}
</script>

<section
	class={['panel', className]}
	aria-labelledby={headingId}
	data-state={expanded ? 'expanded' : 'collapsed'}
	onkeydown={handleKey}
	{...rest}
>
	<header class="head">
		<div class="titles">
			<h2 class="title" id={headingId}>{definition.title}</h2>
			{#if '' !== subtitle}
				<p class="subtitle">{subtitle}</p>
			{/if}
		</div>
		<div class="tools">
			{#if 'stat' === definition.chart || 'heatmap' === definition.chart}
			<IconButton
				icon="table"
				size="sm"
				pressed={inspecting}
				label={inspecting ? t('insights.panel.inspect_hide', { title: definition.title }) : t('insights.panel.inspect', { title: definition.title })}
				onclick={() => (inspecting = !inspecting)}
			/>
			{/if}
			<IconButton
				icon={expanded ? 'minimize' : 'maximize'}
				size="sm"
				pressed={expanded}
				label={expanded ? t('insights.panel.collapse', { title: definition.title }) : t('insights.panel.expand', { title: definition.title })}
				onclick={() => (expanded = !expanded)}
			/>
		</div>
	</header>

	<div class="body" data-chart={definition.chart}>
		{#if null !== failure}
			<ErrorState error={failure} onretry={reloadNow} variant="region" />
		{:else if null === samples}
			<Skeleton shape="rect" height="100%" />
		{:else if 'stat' === definition.chart}
			<div class="stat">
				{#if null === reading.value}
					<p class="stat-empty">{statNone}</p>
				{:else}
					<p class="stat-value" data-tone={crossed?.tone ?? 'default'}>{formatChartValue(reading.value, definition.unit)}</p>
					{#if undefined !== reading.base}
						<p class="stat-base">{t('insights.stat.of', { total: formatChartValue(reading.base.den, definition.unit), share })}</p>
					{/if}
				{/if}
				{#if null !== crossed}
					<p class="stat-flag" data-tone={crossed.tone}>{crossed.label}</p>
				{/if}
			</div>
		{:else if isSeriesChart}
			<TimeSeriesChart
				series={chartSeries(definition, shown)}
				unit={definition.unit}
				thresholds={definition.thresholds}
				{annotations}
				stacked={'stacked' === definition.chart}
				area={'area' === definition.chart}
				{syncKey}
				height="md"
				status={hasData ? 'success' : 'empty'}
				emptyReason={reason}
				{emptyCopy}
				onretry={reloadNow}
			/>
		{:else if hasBars && 'bar' === definition.chart}
			<BarChart data={barData(shown)} unit={definition.unit} label={definition.title} />
		{:else if heatReady}
			<Heatmap cells={heatCells(shown)} xUnit="s" yUnit={definition.unit} label={definition.title} scale="seq" />
		{:else if 'heatmap' === definition.chart && hasData}
			<div class="empty">
				<p class="empty-title">{t('insights.heatmap.none.title')}</p>
				<p class="empty-body">{t('insights.heatmap.none.body')}</p>
			</div>
		{:else}
			<div class="empty">
				<p class="empty-title">{emptyCopy?.title ?? t(`${REASON_COPY[reason]}.title`)}</p>
				<p class="empty-body">{emptyCopy?.body ?? t(`${REASON_COPY[reason]}.body`)}</p>
			</div>
		{/if}
	</div>

	{#if 0 < missing.length && hasData}
		<p class="note">{t('dashboards.not_reported.list', { names: missing.join(', ') })}</p>
	{/if}

	{#if inspecting && null !== samples}
		<div class="inspect" role="region" aria-label={t('dashboards.inspect.region', { title: definition.title })}>
			<table>
				<thead>
					<tr>
						<th scope="col">{t('insights.panel.series')}</th>
						<th scope="col" class="num">{t('insights.panel.value')}</th>
					</tr>
				</thead>
				<tbody>
					{#each ranked as line (line.id)}
						<tr>
							<td>{line.label}</td>
							<td class="num">{formatChartValue(aggregate(line.points, line.measure), definition.unit)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		block-size: 100%;
		min-inline-size: 0;
		padding: var(--card-pad);
		background: var(--surface-raised);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xs);
	}

	.panel[data-state='expanded'] {
		position: fixed;
		inset: var(--panel-inset, var(--space-6));
		z-index: var(--z-drawer);
		overflow: auto;
		box-shadow: var(--shadow-xl);
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.titles {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
		text-wrap: balance;
	}

	.subtitle,
	.note,
	.stat-base,
	.empty-body {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
	}

	.tools {
		display: flex;
		flex: none;
		gap: var(--space-1);
	}

	.body {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-block-size: var(--panel-body-min, calc(var(--space-4) * 15));
		min-inline-size: 0;
	}

	.body:has(> .empty) {
		min-block-size: auto;
	}

	.body[data-chart='stat'] {
		min-block-size: auto;
		justify-content: center;
	}

	.stat {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.stat-value {
		margin: 0;
		font-size: var(--text-display-size);
		line-height: var(--text-display-line);
		font-weight: var(--text-display-weight);
		letter-spacing: var(--text-display-tracking);
		font-variant-numeric: tabular-nums;
		color: var(--text);
	}

	.stat-value[data-tone='warn'] {
		color: var(--warn-text);
	}

	.stat-value[data-tone='danger'] {
		color: var(--danger-text);
	}

	.stat-empty,
	.empty-title {
		margin: 0;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: 500;
		color: var(--text);
	}

	.stat-flag {
		align-self: flex-start;
		margin: 0;
		padding: var(--space-0-5) var(--space-2);
		border-radius: var(--radius-full);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		background: var(--warn-soft);
		color: var(--warn-text);
	}

	.stat-flag[data-tone='danger'] {
		background: var(--danger-soft);
		color: var(--danger-text);
	}

	.empty {
		display: flex;
		flex: 1;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		padding: var(--space-6);
		text-align: center;
		border: var(--hairline) dashed var(--border-subtle);
		border-radius: var(--radius-md);
	}

	.inspect {
		max-block-size: calc(var(--space-4) * 16);
		overflow: auto;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
	}

	table {
		inline-size: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	th,
	td {
		padding: var(--space-1-5) var(--space-3);
		text-align: start;
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	th {
		position: sticky;
		inset-block-start: 0;
		background: var(--surface-sunken);
		font-weight: 500;
		color: var(--text-muted);
	}

	tr:last-child td {
		border-block-end: 0;
	}

	td {
		color: var(--text);
		word-break: break-all;
	}

	.num {
		text-align: end;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
