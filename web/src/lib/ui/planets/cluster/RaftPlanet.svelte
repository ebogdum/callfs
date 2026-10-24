<script lang="ts">
	import { getContext, untrack } from 'svelte';
	import { base } from '$app/paths';
	import {
		cluster,
		snapshot as createSnapshot,
		snapshots,
		stateStats,
		type SnapshotPage
	} from '$lib/api/admin/cluster';
	import type { ClusterInfo, Result, Snapshot, StateStats } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatAbsolute } from '$lib/format/time';
	import { formatNumber } from '$lib/format/number';
	import type { ColumnDef, QueryStatus, UiError } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import Heading from '../../atoms/Heading.svelte';
	import Link from '../../atoms/Link.svelte';
	import Text from '../../atoms/Text.svelte';
	import ToggleButton from '../../atoms/ToggleButton.svelte';
	import BytesCell from '../../cells/BytesCell.svelte';
	import NumberCell from '../../cells/NumberCell.svelte';
	import TextCell from '../../cells/TextCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList, { type Fact as DescriptionItem } from './FactList.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { CLUSTER_REFRESH_MS, COMMANDS_CONTEXT } from './MembersPlanet.svelte';

	const fileIndexHref = `${base}/settings/index`;

	const clusterQuery = createQuery<Result<ClusterInfo, UiError>>({
		key: ['cluster'],
		fetch: () => cluster(),
		staleMs: CLUSTER_REFRESH_MS,
		refetchMs: CLUSTER_REFRESH_MS,
		revalidateOnFocus: true
	});
	const snapshotsQuery = createQuery<Result<SnapshotPage, UiError>>({
		key: ['cluster', 'snapshots'],
		fetch: () => snapshots(),
		staleMs: CLUSTER_REFRESH_MS,
		revalidateOnFocus: true
	});
	const statsQuery = createQuery<Result<StateStats, UiError>>({
		key: ['cluster', 'state-stats'],
		fetch: () => stateStats(),
		staleMs: CLUSTER_REFRESH_MS,
		revalidateOnFocus: true
	});

	let known = $state<ClusterInfo | null>(null);
	$effect(() => {
		const latest = clusterQuery.data;
		if (undefined !== latest && true === latest.ok) {
			known = latest.value;
		}
	});

	const clusterFailure = $derived.by<UiError | null>(() => {
		const latest = clusterQuery.data;
		return undefined !== latest && false === latest.ok ? latest.error : null;
	});

	const snapshotResult = $derived(snapshotsQuery.data);
	const snapshotPage = $derived(
		undefined !== snapshotResult && true === snapshotResult.ok ? snapshotResult.value : null
	);
	const snapshotRows = $derived(snapshotPage?.items ?? []);
	const snapshotFailure = $derived(
		undefined !== snapshotResult && false === snapshotResult.ok ? snapshotResult.error : null
	);
	const snapshotStatus = $derived.by<QueryStatus>(() => {
		if (undefined === snapshotResult) {
			return 'loading';
		}
		if (null !== snapshotFailure) {
			return 'error';
		}
		if (0 === snapshotRows.length) {
			return 'empty';
		}
		return 'success';
	});

	const statsResult = $derived(statsQuery.data);
	const stats = $derived(undefined !== statsResult && true === statsResult.ok ? statsResult.value : null);
	const statsFailure = $derived(undefined !== statsResult && false === statsResult.ok ? statsResult.error : null);

	const indicatorState = $derived(null === clusterFailure ? 'live' : 'offline');
	let showTechnical = $state(true);

	let actionFailure = $state<UiError | null>(null);
	let snapshotting = $state(false);

	const leadershipWord = $derived.by(() => {
		if (null === known) {
			return '';
		}
		if (known.leader_id === known.self_id) {
			return t('members.raft.state.leading');
		}
		if (null !== known.leader_id) {
			return t('members.raft.state.following', { leader: known.leader_id });
		}
		return t('members.raft.state.none');
	});

	const summaryItems = $derived<DescriptionItem[]>(
		null === known
			? []
			: [
					{ term: t('members.raft.state.label'), value: leadershipWord },
					{ term: t('members.raft.term'), value: formatNumber(known.term) },
					{
						term: t('members.raft.last_snapshot'),
						value:
							null === known.last_snapshot
								? t('members.raft.snapshot.none')
								: t('members.raft.snapshot.value', {
										index: formatNumber(known.last_snapshot.index),
										time: formatAbsolute(known.last_snapshot.at)
									})
					}
				]
	);

	const technicalItems = $derived<DescriptionItem[]>(
		null === known
			? []
			: [
					{ term: t('members.raft.last_log'), value: formatNumber(known.last_log_index) },
					{ term: t('members.raft.commit'), value: formatNumber(known.commit_index) },
					{ term: t('members.raft.applied'), value: formatNumber(known.applied_index) },
					{ term: t('members.raft.retain'), value: formatNumber(snapshotPage?.retain ?? null) }
				]
	);

	const statsItems = $derived<DescriptionItem[]>([
		{ term: t('members.raft.stats.files'), value: formatNumber(stats?.files ?? null) },
		{ term: t('members.raft.stats.directories'), value: formatNumber(stats?.directories ?? null) },
		{ term: t('members.raft.stats.links_active'), value: formatNumber(stats?.links.active ?? null) },
		{ term: t('members.raft.stats.links_used'), value: formatNumber(stats?.links.used ?? null) },
		{ term: t('members.raft.stats.links_expired'), value: formatNumber(stats?.links.expired ?? null) },
		{ term: t('members.raft.stats.links_revoked'), value: formatNumber(stats?.links.revoked ?? null) },
		{ term: t('members.raft.stats.erasure'), value: formatNumber(stats?.erasure_entries ?? null) },
		{ term: t('members.raft.stats.index'), value: formatNumber(stats?.index_entries ?? null) }
	]);

	const snapshotColumns: ColumnDef<Snapshot>[] = [
		{ id: 'id', header: t('members.raft.snap.id'), cell: snapshotIdCell },
		{ id: 'index', header: t('members.raft.snap.index'), cell: snapshotIndexCell, align: 'end' },
		{ id: 'term', header: t('members.raft.snap.term'), cell: snapshotTermCell, align: 'end' },
		{ id: 'size', header: t('members.raft.snap.size'), cell: snapshotSizeCell, align: 'end' },
		{ id: 'created', header: t('members.raft.snap.created'), cell: snapshotCreatedCell }
	];

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'cluster.snapshot',
				title: t('members.raft.snapshot'),
				group: 'Cluster',
				keywords: [t('members.raft.title')],
				icon: 'save',
				when: (ctx) => 'admin' === ctx.role,
				run: takeSnapshot
			}
		]));
	});

	function refresh(): void {
		void clusterQuery.refetch();
		void snapshotsQuery.refetch();
		void statsQuery.refetch();
	}

	async function takeSnapshot(): Promise<void> {
		if (true === snapshotting) {
			return;
		}
		snapshotting = true;
		actionFailure = null;
		const outcome = await createSnapshot();
		snapshotting = false;
		if (false === outcome.ok) {
			actionFailure = outcome.error;
			return;
		}
		announce(t('members.raft.snapshot.done', { index: formatNumber(outcome.value.index) }), 'polite');
		invalidate(['cluster']);
	}
</script>

{#snippet snapshotIdCell(row: Snapshot)}
	<TextCell value={row.id} mono />
{/snippet}

{#snippet snapshotIndexCell(row: Snapshot)}
	<NumberCell value={row.index} />
{/snippet}

{#snippet snapshotTermCell(row: Snapshot)}
	<NumberCell value={row.term} />
{/snippet}

{#snippet snapshotSizeCell(row: Snapshot)}
	<BytesCell value={row.bytes} />
{/snippet}

{#snippet snapshotCreatedCell(row: Snapshot)}
	<TimeCell value={row.created_at} />
{/snippet}

<PageFrame title={t('members.raft.title')} job={t('members.raft.job')}>
	{#snippet primary()}
		{#if null !== known && 'raft' === known.mode}
			<Button variant="primary" loading={snapshotting} onclick={takeSnapshot}>{t('members.raft.snapshot')}</Button>
		{/if}
	{/snippet}
	{#snippet toolbar()}
		{#if null !== known && 'raft' === known.mode}
			<ToggleButton bind:pressed={showTechnical} label={t('common.show_technical')} />
		{/if}
	{/snippet}
	{#snippet status()}
		<StaleIndicator updatedAt={clusterQuery.updatedAt} intervalMs={CLUSTER_REFRESH_MS} state={indicatorState} onrefresh={refresh} />
	{/snippet}

	<Stack gap="6">
		{#if null !== actionFailure}
			<ErrorState error={actionFailure} variant="region" />
		{/if}
		{#if null !== clusterFailure && null === known}
			<ErrorState error={clusterFailure} onretry={refresh} />
		{:else if null === known}
			<Skeleton lines={4} />
		{:else if 'single' === known.mode}
			<EmptyState
				variant="not-available"
				icon="database"
				title={t('members.raft.single.title')}
				body={t('members.raft.single.body')}
					primary={{ label: t('members.raft.settings.link'), href: fileIndexHref }}
			/>
		{:else}
			{#if null !== clusterFailure}
				<InlineAlert tone="warn" title={clusterFailure.title} body={clusterFailure.body} />
			{/if}
			<div class="cards">
				<ClusterCard title={t('members.raft.section.log')} icon="logs">
					<FactList items={showTechnical ? [...summaryItems, ...technicalItems] : summaryItems} />
				</ClusterCard>
				{#if showTechnical}
					<ClusterCard title={t('members.raft.section.stored')} icon="database">
						{#if null !== statsFailure}
							<InlineAlert tone="warn" title={statsFailure.title} body={statsFailure.body} />
						{/if}
						<FactList items={statsItems} />
					</ClusterCard>
				{/if}
			</div>
			{#if showTechnical}
				<section class="region" aria-labelledby="raft-snapshots">
					<Heading level={2} size="title-sm" id="raft-snapshots">{t('members.raft.section.snapshots')}</Heading>
					{#if null !== snapshotFailure}
						<InlineAlert tone="warn" title={snapshotFailure.title} body={snapshotFailure.body} />
					{/if}
					<DataTable
						rows={snapshotRows}
						columns={snapshotColumns}
						rowKey={(row) => row.id}
						label={t('members.raft.section.snapshots')}
						status={snapshotStatus}
						error={snapshotFailure ?? undefined}
						onretry={refresh}
					>
						{#snippet empty()}
							<EmptyState
								variant="first-use"
								icon="save"
								title={t('cluster.raft.snapshots.empty.title')}
								body={t('cluster.raft.snapshots.empty.body')}
							/>
						{/snippet}
					</DataTable>
				</section>
			{/if}
			<ClusterCard
				title={t('members.raft.section.settings')}
				description={t('members.raft.settings.summary', { count: known.voters })}
				icon="settings"
			>
				{#snippet footer()}
					<Link href={fileIndexHref}>{t('members.raft.settings.link')}</Link>
				{/snippet}
			</ClusterCard>
		{/if}
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 20)), 1fr));
		gap: var(--space-4);
	}

	.region {
		display: grid;
		gap: var(--space-3);
	}
</style>
