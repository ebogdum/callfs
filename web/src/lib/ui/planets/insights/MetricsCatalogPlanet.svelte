<script lang="ts">
	import { base } from '$app/paths';
	import { metrics as metricsCatalog, type MetricsCatalog } from '$lib/api/admin/telemetry';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import type { MetricFamily, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { DASH } from '$lib/format/number';
	import { formatRelative } from '$lib/format/time';
	import type { StatusKey } from '$lib/status/catalog';
	import NumberCell from '$lib/ui/cells/NumberCell.svelte';
	import StatusCell from '$lib/ui/cells/StatusCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import DescriptionList from '$lib/ui/molecules/DescriptionList.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Icon from '$lib/ui/quarks/Icon.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import SearchInput from '$lib/ui/molecules/SearchInput.svelte';
	import SegmentedControl from '$lib/ui/molecules/SegmentedControl.svelte';
	import type { ColumnDef } from '$lib/ui/types';
	import { processStartedAt } from './dashboards';

	interface MetricDefinition {
		name: string;
		matches: string[];
		type: string;
		labels: string;
		meaning: string;
	}

	interface CatalogRow {
		key: string;
		name: string;
		type: string;
		labels: string;
		meaning: string;
		status: StatusKey;
		value: number | null;
		explore: string | null;
	}

	const CATALOG_REFRESH_MS = 30000;

	const DEFINITIONS: MetricDefinition[] = [
		{ name: 'callfs_http_requests_total', matches: ['callfs_http_requests_total'], type: 'counter', labels: 'method, path, status_code', meaning: 'storage-insights.metric.http_requests.meaning' },
		{ name: 'callfs_http_request_duration_seconds', matches: ['callfs_http_request_duration_seconds'], type: 'histogram', labels: 'method, path', meaning: 'storage-insights.metric.http_duration.meaning' },
		{ name: 'callfs_backend_ops_total', matches: ['callfs_backend_ops_total'], type: 'counter', labels: 'backend_type, operation', meaning: 'storage-insights.metric.backend_ops.meaning' },
		{ name: 'callfs_backend_op_duration_seconds', matches: ['callfs_backend_op_duration_seconds'], type: 'histogram', labels: 'backend_type, operation', meaning: 'storage-insights.metric.backend_op_duration.meaning' },
		{ name: 'callfs_metadata_db_queries_total', matches: ['callfs_metadata_db_queries_total'], type: 'counter', labels: 'operation', meaning: 'storage-insights.metric.metadata_queries.meaning' },
		{ name: 'callfs_metadata_db_query_duration_seconds', matches: ['callfs_metadata_db_query_duration_seconds'], type: 'histogram', labels: 'operation', meaning: 'storage-insights.metric.metadata_query_duration.meaning' },
		{ name: 'callfs_single_use_link_generations_total', matches: ['callfs_single_use_link_generations_total'], type: 'counter', labels: '', meaning: 'storage-insights.metric.link_generations.meaning' },
		{ name: 'callfs_single_use_link_consumptions_total', matches: ['callfs_single_use_link_consumptions_total'], type: 'counter', labels: 'status', meaning: 'storage-insights.metric.link_consumptions.meaning' },
		{ name: 'callfs_lock_operations_total', matches: ['callfs_lock_operations_total'], type: 'counter', labels: 'operation, status', meaning: 'storage-insights.metric.lock_operations.meaning' },
		{ name: 'callfs_lock_operation_duration_seconds', matches: ['callfs_lock_operation_duration_seconds'], type: 'histogram', labels: 'operation', meaning: 'storage-insights.metric.lock_duration.meaning' },
		{ name: 'callfs_active_locks', matches: ['callfs_active_locks'], type: 'gauge', labels: '', meaning: 'storage-insights.metric.active_locks.meaning' },
		{ name: 'callfs_file_operations_total', matches: ['callfs_file_operations_total'], type: 'counter', labels: 'operation, backend_type', meaning: 'storage-insights.metric.file_operations.meaning' },
		{ name: 'callfs_errors_total', matches: ['callfs_errors_total'], type: 'counter', labels: 'component, error_type', meaning: 'storage-insights.metric.errors.meaning' },
		{ name: 'callfs_cache_lookups_total', matches: ['callfs_cache_lookups_total'], type: 'counter', labels: 'result', meaning: 'storage-insights.metric.cache_lookups.meaning' },
		{ name: 'callfs_cache_evictions_total', matches: ['callfs_cache_evictions_total'], type: 'counter', labels: '', meaning: 'storage-insights.metric.cache_evictions.meaning' },
		{ name: 'callfs_cache_entries', matches: ['callfs_cache_entries'], type: 'gauge', labels: '', meaning: 'storage-insights.metric.cache_entries.meaning' },
		{ name: 'callfs_rate_limit_rejections_total', matches: ['callfs_rate_limit_rejections_total'], type: 'counter', labels: 'endpoint', meaning: 'storage-insights.metric.rate_limit_rejections.meaning' },
		{ name: 'callfs_replication_total', matches: ['callfs_replication_total'], type: 'counter', labels: 'result', meaning: 'storage-insights.metric.replication.meaning' },
		{ name: 'callfs_transfers_active', matches: ['callfs_transfers_active'], type: 'gauge', labels: 'kind', meaning: 'storage-insights.metric.transfers_active.meaning' },
		{ name: 'callfs_auth_failures_total', matches: ['callfs_auth_failures_total'], type: 'counter', labels: 'reason', meaning: 'storage-insights.metric.auth_failures.meaning' },
		{ name: 'callfs_nats_connected', matches: ['callfs_nats_connected'], type: 'gauge', labels: '', meaning: 'storage-insights.metric.nats_connected.meaning' },
		{ name: 'callfs_nats_routes', matches: ['callfs_nats_routes'], type: 'gauge', labels: '', meaning: 'storage-insights.metric.nats_routes.meaning' },
		{ name: 'callfs_nats_slow_consumers', matches: ['callfs_nats_slow_consumers'], type: 'counter', labels: '', meaning: 'storage-insights.metric.nats_slow_consumers.meaning' },
		{ name: 'callfs_nats_in_msgs', matches: ['callfs_nats_in_msgs'], type: 'counter', labels: '', meaning: 'storage-insights.metric.nats_in_msgs.meaning' },
		{ name: 'callfs_nats_out_bytes', matches: ['callfs_nats_out_bytes'], type: 'counter', labels: '', meaning: 'storage-insights.metric.nats_out_bytes.meaning' },
		{ name: 'callfs_nats_js_api_errors', matches: ['callfs_nats_js_api_errors'], type: 'counter', labels: '', meaning: 'storage-insights.metric.nats_js_api_errors.meaning' },
		{ name: 'callfs_nats_stream_messages', matches: ['callfs_nats_stream_messages'], type: 'gauge', labels: 'stream', meaning: 'storage-insights.metric.nats_stream_messages.meaning' },
		{ name: 'callfs_nats_stream_bytes', matches: ['callfs_nats_stream_bytes'], type: 'gauge', labels: 'stream', meaning: 'storage-insights.metric.nats_stream_bytes.meaning' },
		{ name: 'callfs_jobs_total', matches: ['callfs_jobs_total'], type: 'counter', labels: 'kind, result', meaning: 'storage-insights.metric.jobs.meaning' },
		{ name: 'callfs_erasure_pieces_repaired_total', matches: ['callfs_erasure_pieces_repaired_total'], type: 'counter', labels: '', meaning: 'storage-insights.metric.erasure_repaired.meaning' },
		{ name: 'callfs_usage_bytes', matches: ['callfs_usage_bytes'], type: 'gauge', labels: 'storage', meaning: 'storage-insights.metric.usage_bytes.meaning' },
		{ name: 'callfs_usage_files', matches: ['callfs_usage_files'], type: 'gauge', labels: 'storage', meaning: 'storage-insights.metric.usage_files.meaning' },
		{ name: 'go_*, process_*', matches: ['go_', 'process_'], type: '', labels: '', meaning: 'storage-insights.metric.runtime.meaning' }
	];

	async function loadCatalog(): Promise<MetricsCatalog> {
		const outcome: Result<MetricsCatalog, ApiFailure> = await metricsCatalog();
		return outcome.ok ? outcome.value : Promise.reject(toUiError(outcome.error, { action: 'explore.action.families', read: true }));
	}

	const catalogQuery = createQuery({
		key: ['metrics-catalog'],
		fetch: loadCatalog,
		staleMs: CATALOG_REFRESH_MS,
		refetchMs: CATALOG_REFRESH_MS
	});

	const familiesFor = (definition: MetricDefinition, families: MetricFamily[]): MetricFamily[] =>
		families.filter((family) =>
			definition.matches.some((match) => (match.endsWith('_') ? family.name.startsWith(match) : family.name === match))
		);

	const sumOf = (families: MetricFamily[]): number =>
		families.reduce(
			(total, family) =>
				total +
				family.series.reduce((sum, series) => sum + (series.value ?? series.count ?? 0), 0),
			0
		);

	const buildRow = (definition: MetricDefinition, families: MetricFamily[]): CatalogRow => {
		const matched = familiesFor(definition, families);
		const reported = matched.some((family) => true === family.emitted) && 0 < sumOf(matched);
		const isExact = definition.matches.every((match) => !match.endsWith('_'));
		const type = definition.type.length > 0 ? definition.type : DASH;
		return {
			key: definition.name,
			name: definition.name,
			type,
			labels: definition.labels.length > 0 ? definition.labels : t('storage-insights.metrics.none'),
			meaning: t(definition.meaning),
			status: reported ? 'metric.reported' : 'metric.not_reported',
			value: reported ? sumOf(matched) : undefined === catalogQuery.data ? null : 0,
			explore: isExact ? `/insights/explore?metric=${encodeURIComponent(definition.name)}` : null
		};
	};

	const catalogRows = $derived(DEFINITIONS.map((definition) => buildRow(definition, catalogQuery.data?.families ?? [])));

	let search = $state('');
	let show = $state<string>('all');

	const startedAt = $derived(processStartedAt(catalogQuery.data?.families ?? []));
	const reportedCount = $derived(catalogRows.filter((row) => 'metric.reported' === row.status).length);
	const visibleRows = $derived(
		catalogRows.filter((row) => {
			const needle = search.trim().toLowerCase();
			const matchesText = '' === needle || row.name.toLowerCase().includes(needle) || row.meaning.toLowerCase().includes(needle);
			const matchesShow =
				'all' === show || ('reported' === show) === ('metric.reported' === row.status);
			return matchesText && matchesShow;
		})
	);
	const showOptions = [
		{ value: 'all', label: t('insights.metrics.filter.all') },
		{ value: 'reported', label: t('insights.metrics.filter.reported') },
		{ value: 'missing', label: t('insights.metrics.filter.missing') }
	];

	const columns: ColumnDef<CatalogRow>[] = [
		{ id: 'name', header: t('storage-insights.metrics.column.name'), cell: nameCell, sortKey: 'name' },
		{ id: 'meaning', header: t('storage-insights.metrics.column.meaning'), cell: meaningCell, width: 360 },
		{ id: 'reported', header: t('storage-insights.metrics.column.reported'), cell: reportedCell, sortKey: 'status' },
		{ id: 'value', header: t('storage-insights.metrics.column.value'), cell: valueCell, align: 'end', sortKey: 'value' },
		{ id: 'type', header: t('storage-insights.metrics.column.type'), cell: typeCell, sortKey: 'type', priority: 2, hidden: true },
		{ id: 'labels', header: t('storage-insights.metrics.column.labels'), cell: labelsCell, priority: 3, hidden: true }
	];

	const rowKeyOf = (row: CatalogRow): string => row.key;
</script>

{#snippet nameCell(row: CatalogRow)}
	{#if null === row.explore}
		<TextCell value={row.name} mono />
	{:else}
		<Link href={`${base}${row.explore}`} title={t('storage-insights.metrics.column.explore')}><span class="mono">{row.name}</span></Link>
	{/if}
{/snippet}

{#snippet typeCell(row: CatalogRow)}
	<TextCell value={row.type} />
{/snippet}

{#snippet labelsCell(row: CatalogRow)}
	<TextCell value={row.labels} muted />
{/snippet}

{#snippet meaningCell(row: CatalogRow)}
	<TextCell value={row.meaning} wrap />
{/snippet}

{#snippet reportedCell(row: CatalogRow)}
	<StatusCell status={row.status} />
{/snippet}

{#snippet valueCell(row: CatalogRow)}
	<NumberCell value={row.value} />
{/snippet}

<PageFrame title={t('storage-insights.metrics.title')} job={t('storage-insights.metrics.job')}>
	{#snippet status()}
		<StaleIndicator
			updatedAt={catalogQuery.updatedAt}
			intervalMs={CATALOG_REFRESH_MS}
			state={'error' === catalogQuery.status ? 'offline' : 'live'}
			onrefresh={() => {
				void catalogQuery.refetch();
			}}
		/>
	{/snippet}

	{#snippet toolbar()}
		<div class="toolbar">
			<div class="search">
				<SearchInput label={t('insights.metrics.search')} placeholder={t('insights.metrics.placeholder')} bind:value={search} resultCount={'' === search ? null : visibleRows.length} />
			</div>
			<SegmentedControl label={t('insights.metrics.filter.label')} options={showOptions} bind:value={show} />
			<span class="count">
				{t('insights.metrics.count', { reported: reportedCount, total: catalogRows.length })}
				{#if null !== startedAt}
					{t('insights.metrics.since', { ago: formatRelative(startedAt * 1000) })}
				{/if}
			</span>
		</div>
	{/snippet}

	<div class="page">
		<section aria-label={t('storage-insights.metrics.table')}>
			<DataTable
				rows={visibleRows}
				{columns}
				rowKey={rowKeyOf}
				label={t('storage-insights.metrics.table')}
				status={0 === visibleRows.length && 'success' === catalogQuery.status ? 'empty' : catalogQuery.status}
				error={catalogQuery.error ?? undefined}
			>
				{#snippet empty()}
					<EmptyState variant="no-results" icon="search" title={t('insights.metrics.empty.title')} body={t('insights.metrics.empty.body')} />
				{/snippet}
			</DataTable>
		</section>

		<section class="card" aria-labelledby="metrics-scrape">
			<div class="card-intro">
				<span class="card-icon" aria-hidden="true"><Icon name="plug" /></span>
				<div class="card-text">
					<h2 class="card-title" id="metrics-scrape">{t('insights.metrics.scrape.title')}</h2>
					<p class="muted">{t('insights.metrics.scrape.body')}</p>
				</div>
			</div>
			<DescriptionList
				columns={2}
				items={[
					{
						term: t('storage-insights.metrics.scrape.main_term'),
						value: '/metrics',
						copy: '/metrics',
						help: t('storage-insights.metrics.scrape.main_help')
					},
					{
						term: t('storage-insights.metrics.scrape.port_term'),
						value: t('storage-insights.metrics.scrape.port_value'),
						help: t('storage-insights.metrics.scrape.port_help')
					}
				]}
			/>
			<InlineAlert
				tone="warn"
				title={t('storage-insights.metrics.exposure.title')}
				body={t('storage-insights.metrics.exposure.body')}
			/>
		</section>
	</div>
</PageFrame>

<style>
	.page {
		display: flex;
		min-inline-size: 0;
		max-inline-size: 100%;
		flex-direction: column;
		gap: var(--space-8);
		min-inline-size: 0;
	}

	.toolbar {
		display: flex;
		flex: 1;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: var(--space-2) var(--space-3);
		min-inline-size: 0;
	}

	.search {
		flex: 1 1 calc(var(--space-4) * 16);
		max-inline-size: calc(var(--space-4) * 24);
		min-inline-size: 0;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
	}

	.count {
		flex: 1 0 auto;
		text-align: end;
		padding-block: var(--space-1-5);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
		font-variant-numeric: tabular-nums;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--card-pad);
		background: var(--surface-raised);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xs);
	}

	.card-intro {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
	}

	.card-icon {
		display: grid;
		flex: none;
		place-items: center;
		inline-size: var(--space-8);
		block-size: var(--space-8);
		border-radius: var(--radius-md);
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	.card-text {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
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
	}
</style>
