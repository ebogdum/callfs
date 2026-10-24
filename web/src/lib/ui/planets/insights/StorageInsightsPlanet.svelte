<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { series as metricSeries } from '$lib/api/admin/telemetry';
	import { owners as listOwners, type OwnerPage } from '$lib/api/admin/users';
	import type { ApiFailure } from '$lib/api/errors';
	import { usage as measureUsage } from '$lib/api/search';
	import type { OwnerStat, Result, Usage, UsageBucket } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatBytes } from '$lib/format/bytes';
	import { formatRelative } from '$lib/format/time';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import PathInput from '$lib/ui/molecules/PathInput.svelte';
	import type StatTile from '$lib/ui/molecules/StatTile.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import BarChart, { type BarDatum } from '$lib/ui/organisms/chart/BarChart.svelte';
	import Histogram from '$lib/ui/organisms/chart/Histogram.svelte';
	import TimeSeriesChart from '$lib/ui/organisms/chart/TimeSeriesChart.svelte';
	import type { QueryStatus, Series } from '$lib/ui/types';
	import type { ComponentProps, Snippet } from 'svelte';

	type StatTileProps = ComponentProps<typeof StatTile>;
	type UsageOutcome = { kind: 'ready'; usage: Usage } | { kind: 'job'; jobId: string };

	interface TwinRow {
		key: string;
		label: string;
		bytes: number | null;
		files: number;
	}

	const FOLDER_CHART_LIMIT = 10;
	const DAY_SECONDS = 86400;
	const GROWTH_WINDOW_DAYS = 7;
	const GROWTH_STEP_SECONDS = 3600;
	const GROWTH_MIN_STEP_SECONDS = 60;
	const GROWTH_POINTS = 60;
	const CACHE_STALE_MS = 60000;
	const GROWTH_METRIC = 'callfs_usage_bytes';
	const STORAGE_LABEL_KEYS: Record<string, string> = {
		localfs: 'storage-insights.storage.localfs',
		s3: 'storage-insights.storage.s3',
		erasure: 'storage-insights.storage.erasure',
		internalproxy: 'storage-insights.storage.internalproxy'
	};

	let growthReported = $state(true);
	const initialScope = page.url.searchParams.get('path') ?? '/';
	let scopeDraft = $state(initialScope);
	let scope = $state(initialScope);

	function valueOrReject<T>(outcome: Result<T, ApiFailure>): Promise<T> {
		return outcome.ok ? Promise.resolve(outcome.value) : Promise.reject(outcome.error);
	}

	function jobIdOf(data: unknown): string | null {
		return 'object' === typeof data && null !== data && 'job_id' in data && 'string' === typeof data.job_id
			? data.job_id
			: null;
	}

	async function loadUsage(path: string): Promise<UsageOutcome> {
		const outcome = await measureUsage(path, { depth: 1, histograms: true });
		if (!outcome.ok) {
			return Promise.reject(outcome.error);
		}
		const jobId = jobIdOf(outcome.value.data);
		if (null !== jobId) {
			return { kind: 'job', jobId };
		}
		return { kind: 'ready', usage: outcome.value.data };
	}

	async function loadOwners(): Promise<OwnerStat[]> {
		const owners: OwnerStat[] = [];
		let cursor: string | null = null;
		do {
			const ownerPage: OwnerPage = await valueOrReject(await listOwners(cursor));
			owners.push(...ownerPage.items);
			cursor = ownerPage.next_cursor;
		} while (null !== cursor);
		return owners;
	}

	const storageLabel = (key: string): string =>
		Object.hasOwn(STORAGE_LABEL_KEYS, key) ? t(STORAGE_LABEL_KEYS[key]) : t('storage-insights.storage.other');

	const bySizeDesc = (a: TwinRow, b: TwinRow): number => (b.bytes ?? 0) - (a.bytes ?? 0);

	const toBar = (row: TwinRow): BarDatum => ({ label: row.label, value: row.bytes ?? 0 });

	const bucketRows = (buckets: Record<string, UsageBucket> | undefined, labelOf: (key: string) => string): TwinRow[] =>
		Object.entries(buckets ?? {})
			.map(([key, bucket]): TwinRow => ({ key, label: labelOf(key), bytes: bucket.bytes, files: bucket.files }))
			.sort(bySizeDesc);


	const usageQuery = createQuery(() => {
		const path = scope;
		return {
			key: ['usage', path],
			fetch: () => loadUsage(path),
			staleMs: CACHE_STALE_MS,
			isEmpty: (outcome: UsageOutcome) =>
				'ready' === outcome.kind && 0 === outcome.usage.files && 0 === outcome.usage.directories,
			isPartial: (outcome: UsageOutcome) => 'ready' === outcome.kind && outcome.usage.scan_truncated
		};
	});

	const growthQuery = createQuery({
		key: ['series', GROWTH_METRIC, GROWTH_WINDOW_DAYS],
		fetch: async () => {
			const nowSeconds = Math.floor(Date.now() / 1000);
			const outcome = await metricSeries({
				name: GROWTH_METRIC,
				measure: 'value',
				from: nowSeconds - GROWTH_WINDOW_DAYS * DAY_SECONDS,
				to: nowSeconds,
				stepS: GROWTH_STEP_SECONDS
			});
			if (!outcome.ok && 'http' === outcome.error.kind && 404 === outcome.error.status) {
				growthReported = false;
				return { series: [] };
			}
			growthReported = true;
			if (!outcome.ok || null === outcome.value.sampled_since) {
				return valueOrReject(outcome);
			}
			const started = Math.ceil(Date.parse(outcome.value.sampled_since) / 1000);
			if (started <= nowSeconds - GROWTH_WINDOW_DAYS * DAY_SECONDS + GROWTH_STEP_SECONDS || started >= nowSeconds) {
				return valueOrReject(outcome);
			}
			const stepS = Math.min(GROWTH_STEP_SECONDS, Math.max(GROWTH_MIN_STEP_SECONDS, Math.ceil((nowSeconds - started) / GROWTH_POINTS)));
			return valueOrReject(await metricSeries({ name: GROWTH_METRIC, measure: 'value', from: started, to: nowSeconds, stepS }));
		},
		staleMs: CACHE_STALE_MS
	});

	const ownersQuery = createQuery({ key: ['owners'], fetch: loadOwners, staleMs: CACHE_STALE_MS });

	const usageOutcome = $derived(usageQuery.data);
	const readyUsage = $derived(undefined !== usageOutcome && 'ready' === usageOutcome.kind ? usageOutcome.usage : null);
	const jobId = $derived(undefined !== usageOutcome && 'job' === usageOutcome.kind ? usageOutcome.jobId : null);
	const usageBlocking = $derived(null !== usageQuery.error && undefined === usageQuery.data ? usageQuery.error : null);

	const usageStatus = $derived<QueryStatus>(null !== jobId ? 'loading' : usageQuery.status);
	const usageLoading = $derived('loading' === usageStatus);
	const usageReason = $derived(usageQuery.error?.title);

	const kpiTiles = $derived<StatTileProps[]>([
		{ label: t('storage-insights.kpi.bytes'), value: readyUsage?.bytes ?? null, unit: 'bytes', loading: usageLoading, help: usageReason },
		{ label: t('storage-insights.kpi.files'), value: readyUsage?.files ?? null, unit: 'count', loading: usageLoading, help: usageReason },
		{ label: t('storage-insights.kpi.folders'), value: readyUsage?.directories ?? null, unit: 'count', loading: usageLoading, help: usageReason },
		{ label: t('storage-insights.kpi.split'), value: readyUsage?.erasure_files ?? null, unit: 'count', loading: usageLoading, help: usageReason }
	]);

	const growthSeries = $derived<Series[]>(
		(growthQuery.data?.series ?? []).map(
			(line, index): Series => ({
				id: `usage-${index}`,
				label: Object.hasOwn(line.labels, 'storage') ? storageLabel(line.labels.storage) : t('storage-insights.growth.total'),
				unit: 'bytes',
				points: line.points
			})
		)
	);

	const folderRows = $derived(
		(readyUsage?.children ?? [])
			.map((child): TwinRow => ({ key: child.path, label: child.path, bytes: child.bytes, files: child.files }))
			.sort(bySizeDesc)
	);
	const storageRows = $derived(bucketRows(readyUsage?.by_storage, storageLabel));
	const serverRows = $derived(bucketRows(readyUsage?.by_instance, (key) => key));
	const ownerRows = $derived(
		(ownersQuery.data ?? [])
			.map(
				(owner): TwinRow => ({
					key: owner.owner,
					label: '' === owner.owner ? t('storage-insights.owner.empty') : owner.owner,
					bytes: owner.bytes,
					files: owner.files
				})
			)
			.sort(bySizeDesc)
	);

	const folderBars = $derived<BarDatum[]>(folderRows.slice(0, FOLDER_CHART_LIMIT).map(toBar));
	const storageBars = $derived<BarDatum[]>(storageRows.map(toBar));
	const serverBars = $derived<BarDatum[]>(serverRows.map(toBar));
	const ownerBars = $derived<BarDatum[]>(ownerRows.map(toBar));

	const sizeBuckets = $derived(readyUsage?.size_histogram ?? []);
	const ageBuckets = $derived((readyUsage?.age_histogram ?? []).map((bucket) => ({ le: bucket.le_days, count: bucket.count })));
	const sizeRows = $derived(
		sizeBuckets.map(
			(bucket): TwinRow => ({
				key: String(bucket.le),
				label: t('storage-insights.sizes.up_to', { size: formatBytes(bucket.le).text }),
				bytes: null,
				files: bucket.count
			})
		)
	);
	const ageRows = $derived(
		ageBuckets.map(
			(bucket): TwinRow => ({
				key: String(bucket.le),
				label: t('storage-insights.age.up_to', { days: bucket.le }),
				bytes: null,
				files: bucket.count
			})
		)
	);


	const commitScope = (path: string) => {
		scope = path;
		void goto(`?path=${encodeURIComponent(path)}`, { replaceState: true, keepFocus: true, noScroll: true });
	};
</script>

{#snippet panel(
	id: string,
	title: string,
	chart: Snippet,
	rows: TwinRow[],
	status: QueryStatus,
	empty: string
)}
	<section class="card" aria-labelledby={id}>
		<header class="card-head">
			<h3 class="card-title" {id}>{title}</h3>
		</header>
		<div class="card-chart">
			{#if 'loading' === status}
				<Skeleton shape="rect" height="var(--space-16)" />
			{:else if 'error' === status && 0 === rows.length}
				<div class="card-empty">{t('storage-insights.panel.failed')}</div>
			{:else if 0 === rows.length}
				<div class="card-empty">{empty}</div>
			{:else}
				{@render chart()}
			{/if}
		</div>
	</section>
{/snippet}

{#snippet folderChart()}
	<BarChart data={folderBars} unit="bytes" label={t('storage-insights.folders.label', { path: scope })} />
{/snippet}

{#snippet storageChart()}
	<BarChart data={storageBars} unit="bytes" label={t('storage-insights.storage.label')} />
{/snippet}

{#snippet serverChart()}
	<BarChart data={serverBars} unit="bytes" label={t('storage-insights.server.label')} />
{/snippet}

{#snippet ownerChart()}
	<BarChart data={ownerBars} unit="bytes" label={t('storage-insights.owner.label')} />
{/snippet}

{#snippet sizeChart()}
	<Histogram buckets={sizeBuckets} unit="bytes" label={t('storage-insights.sizes.label')} />
{/snippet}

{#snippet ageChart()}
	<Histogram buckets={ageBuckets} unit="none" label={t('storage-insights.age.label')} />
{/snippet}

<PageFrame title={t('storage-insights.title')} job={t('storage-insights.job')}>
	{#snippet status()}
		{#if null !== readyUsage}
			<span class="calculated">{t('storage-insights.calculated', { when: formatRelative(readyUsage.computed_at) })}</span>
		{/if}
	{/snippet}

	{#snippet primary()}
		<Button
			variant="primary"
			iconStart="refresh"
			loading={usageQuery.fetching}
			onclick={() => {
				void usageQuery.refetch();
			}}
		>
			{t('storage-insights.recalculate')}
		</Button>
	{/snippet}

	{#snippet toolbar()}
		<div class="scope">
			<PathInput label={t('storage-insights.scope.label')} bind:value={scopeDraft} onsubmit={commitScope} />
		</div>
	{/snippet}

	<div class="page">
		{#if null !== jobId}
			<InlineAlert tone="info" title={t('storage-insights.job_running.title')} body={t('storage-insights.job_running.body')}>
				{#snippet actions()}
					<Link href={`${base}/activity/jobs/${encodeURIComponent(jobId)}`}>{t('storage-insights.job_running.open')}</Link>
				{/snippet}
			</InlineAlert>
		{/if}
		{#if 'partial' === usageQuery.status}
			<InlineAlert tone="warn" title={t('storage-insights.partial')} />
		{/if}
		{#if null !== usageQuery.error && undefined !== usageQuery.data}
			<InlineAlert tone="danger" title={usageQuery.error.title} body={usageQuery.error.body} />
		{/if}

		{#if null !== usageBlocking}
			<ErrorState
				error={usageBlocking}
				variant="region"
				onretry={() => {
					void usageQuery.refetch();
				}}
			/>
		{:else if 'empty' === usageQuery.status}
			<EmptyState
				variant="first-use"
				icon="hard-drive"
				title={t('storage-insights.empty.title')}
				body={t('storage-insights.empty.body')}
				primary={{ label: t('storage-insights.empty.upload'), href: `${base}/files` }}
			/>
		{:else}
			<section aria-label={t('storage-insights.region.kpis')}>
				<KpiStrip tiles={kpiTiles} />
			</section>

			<section class="card" aria-labelledby="storage-growth">
				<header class="card-head">
					<h2 class="card-title" id="storage-growth">{t('storage-insights.growth.title')}</h2>
				</header>
				<TimeSeriesChart
					series={growthSeries}
					unit="bytes"
					height="md"
					status={'success' === growthQuery.status && 0 === growthSeries.length ? 'empty' : growthQuery.status}
					error={growthQuery.error ?? undefined}
					emptyReason={growthReported ? 'no_samples' : 'not_reported'}
					emptyCopy={growthReported
						? { title: t('storage-insights.growth.waiting.title'), body: t('storage-insights.growth.waiting.body') }
						: undefined}
					onretry={() => {
						void growthQuery.refetch();
					}}
				/>
			</section>

			<section class="group" aria-labelledby="storage-where">
				<h2 class="group-title" id="storage-where">{t('insights.storage.charts')}</h2>
				<div class="grid">
					{@render panel(
						'storage-folders',
						t('storage-insights.folders.title'),
						folderChart,
						folderRows,
						usageStatus,
						t('storage-insights.folders.empty')
					)}
					{@render panel(
						'storage-by-owner',
						t('storage-insights.owner.title'),
						ownerChart,
						ownerRows,
						ownersQuery.status,
						t('storage-insights.owner.empty_list')
					)}
					{@render panel(
						'storage-by-storage',
						t('storage-insights.storage.title'),
						storageChart,
						storageRows,
						usageStatus,
						t('storage-insights.storage.empty')
					)}
					{@render panel(
						'storage-by-server',
						t('storage-insights.server.title'),
						serverChart,
						serverRows,
						usageStatus,
						t('storage-insights.server.empty')
					)}
				</div>
			</section>

			<section class="group" aria-labelledby="storage-files">
				<h2 class="group-title" id="storage-files">{t('insights.storage.files')}</h2>
				<div class="grid">
					{@render panel(
						'storage-sizes',
						t('storage-insights.sizes.title'),
						sizeChart,
						sizeRows,
						usageStatus,
						t('storage-insights.sizes.empty')
					)}
					{@render panel(
						'storage-age',
						t('storage-insights.age.title'),
						ageChart,
						ageRows,
						usageStatus,
						t('storage-insights.age.empty')
					)}
				</div>
			</section>
		{/if}
	</div>
</PageFrame>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: var(--space-8);
		min-inline-size: 0;
	}

	.scope {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		inline-size: min(100%, var(--form-max));
	}

	.calculated {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.group-title {
		margin: 0;
		font-size: var(--text-title-size);
		line-height: var(--text-title-line);
		font-weight: var(--text-title-weight);
		letter-spacing: var(--text-title-tracking);
		color: var(--text);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 26)), 1fr));
		gap: var(--space-4);
		align-items: start;
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

	.card-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-block-size: var(--control-h);
	}

	.card-title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
	}

	.card-chart {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-inline-size: 0;
	}

	.card-empty {
		display: grid;
		flex: 1;
		place-items: center;
		padding: var(--space-6);
		border: var(--hairline) dashed var(--border-subtle);
		border-radius: var(--radius-md);
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}
</style>
