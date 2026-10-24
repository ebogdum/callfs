<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { erasurePlan, erasureSummary, verifyAll } from '$lib/api/admin/topology';
	import { toUiError } from '$lib/api/errors';
	import { listSplitFiles, repair, type SplitFilePage, type SplitHealth } from '$lib/api/erasure';
	import type { ErasureSummary, PlacementPlan, Result, SplitFile } from '$lib/api/types';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import { notifications } from '$lib/stores/notifications.svelte';
	import { jobs } from '$lib/stores/jobs.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { StatusKey } from '$lib/status/catalog';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import BytesCell from '$lib/ui/cells/BytesCell.svelte';
	import CountCell from '$lib/ui/cells/CountCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import DurabilityMeter from '$lib/ui/molecules/DurabilityMeter.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StatTile from '$lib/ui/molecules/StatTile.svelte';
	import BarChart from '$lib/ui/organisms/chart/BarChart.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import PlacementPlanner from '$lib/ui/organisms/PlacementPlanner.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import type { ColumnDef, QueryStatus, UiError } from '$lib/ui/types';
	import type { ComponentProps } from 'svelte';

	type StatTileProps = ComponentProps<typeof StatTile>;

	interface ExtraPages {
		key: string;
		items: SplitFile[];
		cursor: string | null | undefined;
	}

	const PAGE_SIZE = 200;
	const FILTERABLE_HEALTH: readonly SplitHealth[] = ['ok', 'reduced', 'critical', 'unreadable'];
	const DAMAGED_HEALTH: readonly SplitHealth[] = ['reduced', 'critical', 'unreadable'];

	let extra = $state<ExtraPages>({ key: 'all', items: [], cursor: undefined });
	let moreError = $state<UiError | null>(null);
	let loadingMore = $state(false);
	let verifying = $state(false);
	let repairing = $state(false);
	let actionError = $state<UiError | null>(null);
	let planData = $state(0);
	let planSpare = $state(0);
	let planSeeded = $state(false);

	const summaryQuery = createQuery<Result<ErasureSummary, UiError>>({
		key: ['erasure', 'summary'],
		fetch: async () => {
			const response = await erasureSummary();
			if (!response.ok) {
				return { ok: false, error: toUiError(response.error, { action: 'durability.load' }) };
			}
			return { ok: true, value: response.value };
		},
		liveTopics: ['job.*']
	});

	const summaryResult = $derived(summaryQuery.data);
	const summary = $derived(summaryResult?.ok ? summaryResult.value : undefined);
	const summaryError = $derived(summaryResult && !summaryResult.ok ? summaryResult.error : undefined === summaryResult ? summaryQuery.error : null);
	const instances = $derived(summary?.instances ?? []);
	const damagedCount = $derived(
		summary ? summary.health.reduced + summary.health.critical + summary.health.unreadable : 0
	);

	const requestedHealth = $derived(page.url.searchParams.get('health'));
	const healthFilter = $derived(FILTERABLE_HEALTH.find((candidate) => candidate === requestedHealth) ?? null);
	const filterKey = $derived(healthFilter ?? 'all');

	const filesQuery = createQuery<Result<SplitFilePage, UiError>>(() => {
		const health = healthFilter;
		return {
			key: ['erasure', 'files', health ?? 'all'],
			fetch: () => listSplitFiles({ health: health ?? undefined, limit: PAGE_SIZE }, null),
			liveTopics: ['job.*']
		};
	});

	const filesResult = $derived(filesQuery.data);
	const filesError = $derived(filesResult && !filesResult.ok ? filesResult.error : undefined === filesResult ? filesQuery.error : null);
	const firstItems = $derived(filesResult?.ok ? filesResult.value.items : []);
	const firstNextCursor = $derived(filesResult?.ok ? filesResult.value.next_cursor : null);
	const pages = $derived<ExtraPages>(extra.key === filterKey ? extra : { key: filterKey, items: [], cursor: undefined });
	const rows = $derived([...firstItems, ...pages.items]);
	const nextCursor = $derived(undefined === pages.cursor ? firstNextCursor : pages.cursor);
	const partialFiles = $derived(rows.some((row) => null === row.healthy_shards));
	const tableStatus = $derived(filesStatusOf(filesResult, filesQuery.status, rows.length, null !== moreError));

	const planQuery = createQuery<Result<PlacementPlan, UiError>>(() => {
		const data = planData;
		const spare = planSpare;
		const servers = instances;
		return {
			key: ['erasure', 'plan', data, spare, servers],
			fetch: () => loadPlan(data, spare, servers),
			enabled: () => 0 < data && 0 < servers.length
		};
	});

	const planResult = $derived(planQuery.data);
	const planValue = $derived(planResult?.ok ? planResult.value : null);
	const planStatus = $derived(statusOf(planResult, planQuery.status));

	const tiles = $derived<StatTileProps[]>(
		summary
			? [
					{ label: t('durability.kpi.files'), value: summary.files, unit: 'count' },
					{
						label: t('durability.kpi.full'),
						value: summary.health.ok,
						unit: 'count',
						base: { num: summary.health.ok, den: summary.files },
						status: 0 < summary.files && summary.health.ok === summary.files ? 'durability.full' : undefined,
						href: `${base}/cluster/durability?health=ok`
					},
					{
						label: t('durability.kpi.less'),
						value: summary.health.reduced + summary.health.critical,
						unit: 'count',
						status: lessProtectedStatus(summary),
						href: `${base}/cluster/durability?health=reduced`
					},
					{
						label: t('durability.kpi.unreadable'),
						value: summary.health.unreadable,
						unit: 'count',
						status: 0 < summary.health.unreadable ? 'durability.unreadable' : undefined,
						href: `${base}/cluster/durability?health=unreadable`
					}
				]
			: []
	);

	const pieceBars = $derived.by(() => {
		if (!summary) {
			return [];
		}
		const counts: Record<string, number> = summary.pieces_by_instance ?? {};
		const servers = [...new Set([...instances, ...Object.keys(counts)])];
		return servers.map((label) => ({ label, value: counts[label] ?? 0 }));
	});

	const columns = $derived<ColumnDef<SplitFile>[]>([
		{ id: 'path', header: t('durability.col.path'), cell: pathCell, width: 240 },
		{ id: 'size', header: t('durability.col.size'), cell: sizeCell, align: 'end' },
		{ id: 'layout', header: t('durability.col.layout'), cell: layoutCell },
		{ id: 'pieces', header: t('durability.col.pieces'), cell: piecesCell, align: 'end' },
		{ id: 'protection', header: t('durability.col.protection'), cell: protectionCell },
		{ id: 'checked', header: t('durability.col.checked'), cell: checkedCell, width: 140 }
	]);

	$effect(() => {
		if (!planSeeded && summary) {
			planData = summary.data_shards;
			planSpare = summary.parity_shards;
			planSeeded = true;
		}
	});

	function statusOf<T>(result: Result<T, UiError> | undefined, pending: QueryStatus): QueryStatus {
		if (undefined === result) {
			return pending;
		}
		return result.ok ? 'success' : 'error';
	}

	function filesStatusOf(
		result: Result<SplitFilePage, UiError> | undefined,
		pending: QueryStatus,
		count: number,
		partial: boolean
	): QueryStatus {
		if (undefined === result) {
			return pending;
		}
		if (!result.ok) {
			return 'error';
		}
		if (0 === count) {
			return 'empty';
		}
		return partial ? 'partial' : 'success';
	}

	function lessProtectedStatus(current: ErasureSummary): StatusKey | undefined {
		if (0 < current.health.critical) {
			return 'durability.critical';
		}
		return 0 < current.health.reduced ? 'durability.reduced' : undefined;
	}

	async function loadPlan(
		dataShards: number,
		spareShards: number,
		servers: string[]
	): Promise<Result<PlacementPlan, UiError>> {
		const response = await erasurePlan({ data_shards: dataShards, parity_shards: spareShards, instances: servers });
		if (!response.ok) {
			return { ok: false, error: toUiError(response.error, { action: 'durability.plan' }) };
		}
		return { ok: true, value: response.value };
	}

	async function loadMore(): Promise<void> {
		const cursor = nextCursor;
		if (null === cursor || loadingMore) {
			return;
		}
		loadingMore = true;
		const requestedKey = filterKey;
		const loadedItems = pages.items;
		const response = await listSplitFiles({ health: healthFilter ?? undefined, limit: PAGE_SIZE }, cursor);
		loadingMore = false;
		if (requestedKey !== filterKey) {
			return;
		}
		if (!response.ok) {
			moreError = response.error;
			return;
		}
		moreError = null;
		extra = {
			key: requestedKey,
			items: [...loadedItems, ...response.value.items],
			cursor: response.value.next_cursor
		};
	}

	function retry(): void {
		void summaryQuery.refetch();
		void filesQuery.refetch();
	}

	function detailHref(path: string): string {
		return `${base}/cluster/durability/file${path
			.split('/')
			.map((segment) => encodeURIComponent(segment))
			.join('/')}`;
	}

	function openFile(row: SplitFile): void {
		void goto(detailHref(row.path));
	}

	function reportFailure(error: UiError): void {
		actionError = error;
		notifications.add({
			tone: 'danger',
			category: 'error',
			title: error.title,
			body: error.body,
			requestId: error.requestId ?? null
		});
	}

	async function verifyEverything(): Promise<void> {
		verifying = true;
		const response = await verifyAll('/');
		verifying = false;
		if (!response.ok) {
			reportFailure(toUiError(response.error, { action: 'durability.verify_all' }));
			return;
		}
		actionError = null;
		jobs.watch(response.value.job_id);
		toasts.push({ tone: 'info', title: t('durability.verify_all.started') });
	}

	async function repairDamaged(): Promise<void> {
		const damagedRows = rows.filter((row) => DAMAGED_HEALTH.includes(row.health));
		repairing = true;
		let accepted = 0;
		for (const row of damagedRows) {
			const response = await repair(row.path);
			if (!response.ok) {
				repairing = false;
				reportFailure(response.error);
				return;
			}
			jobs.watch(response.value.job_id);
			accepted += 1;
		}
		repairing = false;
		if (0 < accepted) {
			actionError = null;
			toasts.push({ tone: 'info', title: t('durability.repair.started', { count: accepted }) });
		}
	}
</script>

{#snippet pathCell(row: SplitFile)}
	<Link href={detailHref(row.path)}>
		<span class="path-wrap" title={row.path}>{#each row.path.split('/') as part, index (index)}{#if 0 < index}/<wbr />{/if}{part}{/each}</span>
	</Link>
{/snippet}

{#snippet sizeCell(row: SplitFile)}
	<BytesCell value={row.original_size} />
{/snippet}

{#snippet layoutCell(row: SplitFile)}
	<TextCell value={t('durability.layout', { data: formatNumber(row.data_shards), spare: formatNumber(row.parity_shards) })} />
{/snippet}

{#snippet piecesCell(row: SplitFile)}
	{#if null === row.healthy_shards}
		<TextCell value={t('durability.pieces.unchecked')} muted />
	{:else}
		<CountCell num={row.healthy_shards} den={row.shards_total} />
	{/if}
{/snippet}

{#snippet protectionCell(row: SplitFile)}
	{#if null === row.healthy_shards}
		<TextCell value={t('durability.pieces.unchecked')} muted />
	{:else}
		<DurabilityMeter data={row.data_shards} spare={row.parity_shards} healthy={row.healthy_shards} />
	{/if}
{/snippet}

{#snippet checkedCell(row: SplitFile)}
	<TimeCell value={row.last_checked_at} />
{/snippet}

{#snippet emptyFiles()}
	<EmptyState
		variant="first-use"
		icon="split"
		title={t('durability.table.empty.title')}
		body={t('durability.table.empty.body')}
	/>
{/snippet}

{#snippet primary()}
	{#if summary?.enabled}
		{#if 0 < damagedCount}
			<Button variant="primary" loading={repairing} onclick={repairDamaged}>{t('durability.primary.repair')}</Button>
		{:else}
			<Button variant="primary" loading={verifying} onclick={verifyEverything}>{t('durability.primary.verify')}</Button>
		{/if}
	{/if}
{/snippet}

{#snippet secondary()}
	<Button variant="ghost" iconStart="refresh" onclick={retry}>{t('durability.refresh')}</Button>
{/snippet}

<PageFrame title={t('durability.title')} job={t('durability.job')} {primary} {secondary}>
	{#if undefined === summaryResult && null === summaryError}
		<Stack gap="4">
			<Skeleton shape="rect" height="var(--space-16)" />
			<Skeleton shape="rect" height="16rem" />
		</Stack>
	{:else if summaryError}
		<ErrorState error={summaryError} onretry={retry} />
	{:else if summary && !summary.enabled}
		<EmptyState
			variant="not-available"
			icon="split"
			title={t('durability.off.title')}
			body={t('durability.off.body')}
			primary={{ label: t('durability.off.link'), href: `${base}/settings/storage` }}
		/>
	{:else if summary}
		<Stack gap="8">
			{#if moreError}
				<InlineAlert tone="warn" title={moreError.title} body={moreError.body} />
			{:else if partialFiles}
				<InlineAlert tone="warn" title={t('durability.partial.title')} body={t('durability.partial.body')} />
			{/if}
			{#if actionError}
				<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
			{/if}
			<KpiStrip tiles={tiles} />
			<div class="cards">
				<ClusterCard title={t('durability.chart.label')} icon="bar-chart">
					<BarChart data={pieceBars} unit="count" label={t('durability.chart.label')} />
				</ClusterCard>
				<ClusterCard title={t('cluster.durability.plan.title')} description={t('cluster.durability.plan.body')} icon="split">
					<PlacementPlanner bind:data={planData} bind:spare={planSpare} servers={instances} plan={planValue} status={planStatus} />
					{#snippet footer()}
						<Link href={`${base}/settings/storage`}>{t('durability.plan.settings')}</Link>
					{/snippet}
				</ClusterCard>
			</div>
			<DataTable
				rows={rows}
				columns={columns}
				rowKey={(row: SplitFile) => row.path}
				label={t('durability.table.label')}
				status={tableStatus}
				error={filesError ?? undefined}
				hasMore={null !== nextCursor}
				onloadmore={loadMore}
				onopen={openFile}
				onretry={retry}
				empty={emptyFiles}
			/>
		</Stack>
	{/if}
</PageFrame>

<style>
	.path-wrap {
		font-family: var(--font-mono);
		font-size: var(--table-text);
		overflow-wrap: anywhere;
	}

	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 22)), 1fr));
		align-items: start;
		gap: var(--space-4);
	}
</style>
