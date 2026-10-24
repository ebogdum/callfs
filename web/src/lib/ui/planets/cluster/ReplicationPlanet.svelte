<script lang="ts">
	import { getContext, untrack } from 'svelte';
	import { base } from '$app/paths';
	import {
		cluster,
		snapshot as createSnapshot,
		snapshots,
		stateStats,
		transferLeadership,
		type SnapshotPage
	} from '$lib/api/admin/cluster';
	import type { ClusterInfo, ClusterMember, Result, Snapshot, StateStats } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatAbsolute } from '$lib/format/time';
	import { formatNumber } from '$lib/format/number';
	import type { ColumnDef, QueryStatus, UiError } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import Heading from '../../atoms/Heading.svelte';
	import Link from '../../atoms/Link.svelte';
	import Select, { type SelectOption } from '../../atoms/Select.svelte';
	import Text from '../../atoms/Text.svelte';
	import ToggleButton from '../../atoms/ToggleButton.svelte';
	import BytesCell from '../../cells/BytesCell.svelte';
	import DurationCell from '../../cells/DurationCell.svelte';
	import NumberCell from '../../cells/NumberCell.svelte';
	import TextCell from '../../cells/TextCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList, { type Fact as DescriptionItem } from './FactList.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import FormField from '../../molecules/FormField.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import Drawer from '../../organisms/Drawer.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { CLUSTER_REFRESH_MS, COMMANDS_CONTEXT } from './MembersPlanet.svelte';

	const AUTOMATIC = '';
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

	const replicas = $derived(known?.members ?? []);
	const replicaStatus = $derived.by<QueryStatus>(() => (0 === replicas.length ? 'empty' : 'success'));

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
	let moveOpen = $state(false);
	let moveTarget = $state(AUTOMATIC);
	let moving = $state(false);
	let moveFailure = $state<UiError | null>(null);

	const moveOptions = $derived<SelectOption[]>([
		{ value: AUTOMATIC, label: t('members.replication.move.automatic') },
		...replicas
			.filter((member) => member.online && 'leader' !== member.state)
			.map((member) => ({ value: member.node_id, label: member.node_id }))
	]);

	const leaderWord = $derived.by(() => {
		if (null === known || null === known.leader_id) {
			return t('members.replication.leader.none');
		}
		if (known.leader_id === known.self_id) {
			return t('members.replication.leader.self', { leader: known.leader_id });
		}
		return known.leader_id;
	});

	const logItems = $derived<DescriptionItem[]>(
		null === known
			? []
			: [
					{ term: t('members.replication.leader'), value: leaderWord },
					{ term: t('members.replication.messages'), value: formatNumber(known.log.messages) },
					{ term: t('members.replication.size'), value: formatBytes(known.log.bytes).text },
					{ term: t('members.replication.first_seq'), value: formatNumber(known.log.first_seq) },
					{ term: t('members.replication.last_seq'), value: formatNumber(known.log.last_seq) },
					{ term: t('members.replication.applied'), value: formatNumber(known.log.applied_seq) },
					{
						term: t('members.replication.last_snapshot'),
						value:
							null === known.log.last_snapshot
								? t('members.replication.snapshot.none')
								: t('members.replication.snapshot.value', {
										seq: formatNumber(known.log.last_snapshot.seq),
										time: formatAbsolute(known.log.last_snapshot.created_at)
									})
					},
					{ term: t('members.replication.retain'), value: formatNumber(snapshotPage?.retain ?? null) }
				]
	);

	const statsItems = $derived<DescriptionItem[]>([
		{ term: t('members.replication.stats.files'), value: formatNumber(stats?.files ?? null) },
		{ term: t('members.replication.stats.directories'), value: formatNumber(stats?.directories ?? null) },
		{ term: t('members.replication.stats.links_active'), value: formatNumber(stats?.links.active ?? null) },
		{ term: t('members.replication.stats.links_used'), value: formatNumber(stats?.links.used ?? null) },
		{ term: t('members.replication.stats.links_expired'), value: formatNumber(stats?.links.expired ?? null) },
		{ term: t('members.replication.stats.links_revoked'), value: formatNumber(stats?.links.revoked ?? null) },
		{ term: t('members.replication.stats.erasure'), value: formatNumber(stats?.erasure_entries ?? null) },
		{ term: t('members.replication.stats.index'), value: formatNumber(stats?.index_entries ?? null) }
	]);

	const replicaColumns: ColumnDef<ClusterMember>[] = [
		{ id: 'server', header: t('members.replication.replica.server'), cell: replicaServerCell },
		{ id: 'current', header: t('members.replication.replica.current'), cell: replicaCurrentCell },
		{ id: 'lag', header: t('members.replication.replica.lag'), cell: replicaLagCell, align: 'end' },
		{ id: 'seen', header: t('members.replication.replica.last_seen'), cell: replicaSeenCell, align: 'end' }
	];

	const snapshotColumns: ColumnDef<Snapshot>[] = [
		{ id: 'id', header: t('members.replication.snap.id'), cell: snapshotIdCell },
		{ id: 'seq', header: t('members.replication.snap.seq'), cell: snapshotSeqCell, align: 'end' },
		{ id: 'size', header: t('members.replication.snap.size'), cell: snapshotSizeCell, align: 'end' },
		{ id: 'created', header: t('members.replication.snap.created'), cell: snapshotCreatedCell }
	];

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'cluster.snapshot',
				title: t('members.replication.snapshot'),
				group: 'Cluster',
				keywords: [t('members.replication.title')],
				icon: 'save',
				when: (ctx) => 'admin' === ctx.role,
				run: takeSnapshot
			},
			{
				id: 'cluster.move-leader',
				title: t('members.replication.move'),
				group: 'Cluster',
				keywords: [t('members.replication.title')],
				icon: 'leader',
				when: (ctx) => 'admin' === ctx.role,
				run: openMove
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
		announce(t('members.replication.snapshot.done', { seq: formatNumber(outcome.value.seq) }), 'polite');
		invalidate(['cluster']);
	}

	function openMove(): void {
		moveTarget = AUTOMATIC;
		moveFailure = null;
		moveOpen = true;
	}

	async function moveLeader(): Promise<void> {
		if (true === moving) {
			return;
		}
		moving = true;
		moveFailure = null;
		const outcome = await transferLeadership('' === moveTarget ? null : moveTarget);
		moving = false;
		if (false === outcome.ok) {
			moveFailure = outcome.error;
			return;
		}
		announce(t('members.transfer.done', { leader: outcome.value.leader_id ?? t('common.unknown') }), 'polite');
		moveOpen = false;
		invalidate(['cluster']);
	}
</script>

{#snippet replicaServerCell(row: ClusterMember)}
	<TextCell value={row.node_id} mono />
{/snippet}

{#snippet replicaCurrentCell(row: ClusterMember)}
	<TextCell value={row.online && 0 === row.behind_by ? t('members.replication.replica.yes') : t('members.replication.replica.no')} />
{/snippet}

{#snippet replicaLagCell(row: ClusterMember)}
	<NumberCell value={row.behind_by} />
{/snippet}

{#snippet replicaSeenCell(row: ClusterMember)}
	<DurationCell value={row.last_seen_ms ?? t('common.unknown')} />
{/snippet}

{#snippet snapshotIdCell(row: Snapshot)}
	<TextCell value={row.id} mono />
{/snippet}

{#snippet snapshotSeqCell(row: Snapshot)}
	<NumberCell value={row.seq} />
{/snippet}

{#snippet snapshotSizeCell(row: Snapshot)}
	<BytesCell value={row.bytes} />
{/snippet}

{#snippet snapshotCreatedCell(row: Snapshot)}
	<TimeCell value={row.created_at} />
{/snippet}

{#snippet moveFooter()}
	<Cluster justify="end" gap="2">
		<Button variant="ghost" onclick={() => (moveOpen = false)}>{t('common.cancel')}</Button>
		<Button variant="primary" loading={moving} onclick={moveLeader}>{t('members.replication.move.confirm')}</Button>
	</Cluster>
{/snippet}

<PageFrame title={t('members.replication.title')} job={t('members.replication.job')}>
	{#snippet primary()}
		{#if null !== known && 'nats' === known.mode}
			<Button variant="primary" loading={snapshotting} onclick={takeSnapshot}>{t('members.replication.snapshot')}</Button>
		{/if}
	{/snippet}
	{#snippet secondary()}
		{#if null !== known && 'nats' === known.mode}
			<Button variant="secondary" iconStart="leader" onclick={openMove}>{t('members.replication.move')}</Button>
		{/if}
	{/snippet}
	{#snippet toolbar()}
		{#if null !== known && 'nats' === known.mode}
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
				title={t('members.replication.single.title')}
				body={t('members.replication.single.body')}
				primary={{ label: t('members.replication.settings.link'), href: fileIndexHref }}
			/>
		{:else}
			{#if null !== clusterFailure}
				<InlineAlert tone="warn" title={clusterFailure.title} body={clusterFailure.body} />
			{/if}
			<div class="cards">
				<ClusterCard title={t('members.replication.section.log')} icon="logs">
					<FactList items={showTechnical ? logItems : logItems.slice(0, 2)} />
				</ClusterCard>
				{#if showTechnical}
					<ClusterCard title={t('members.replication.section.stored')} icon="database">
						{#if null !== statsFailure}
							<InlineAlert tone="warn" title={statsFailure.title} body={statsFailure.body} />
						{/if}
						<FactList items={statsItems} />
					</ClusterCard>
				{/if}
			</div>
			<section class="region" aria-labelledby="replication-replicas">
				<Heading level={2} size="title-sm" id="replication-replicas">{t('members.replication.section.replicas')}</Heading>
				<DataTable
					rows={replicas}
					columns={replicaColumns}
					rowKey={(row) => row.node_id}
					label={t('members.replication.section.replicas')}
					status={replicaStatus}
					onretry={refresh}
				>
					{#snippet empty()}
						<EmptyState
							variant="first-use"
							icon="server"
							title={t('members.replication.replicas.empty.title')}
							body={t('members.replication.replicas.empty.body')}
						/>
					{/snippet}
				</DataTable>
			</section>
			{#if showTechnical}
				<section class="region" aria-labelledby="replication-snapshots">
					<Heading level={2} size="title-sm" id="replication-snapshots">{t('members.replication.section.snapshots')}</Heading>
					{#if null !== snapshotFailure}
						<InlineAlert tone="warn" title={snapshotFailure.title} body={snapshotFailure.body} />
					{/if}
					<DataTable
						rows={snapshotRows}
						columns={snapshotColumns}
						rowKey={(row) => row.id}
						label={t('members.replication.section.snapshots')}
						status={snapshotStatus}
						error={snapshotFailure ?? undefined}
						onretry={refresh}
					>
						{#snippet empty()}
							<EmptyState
								variant="first-use"
								icon="save"
								title={t('cluster.replication.snapshots.empty.title')}
								body={t('cluster.replication.snapshots.empty.body')}
							/>
						{/snippet}
					</DataTable>
				</section>
			{/if}
			<ClusterCard
				title={t('members.replication.section.settings')}
				description={t('members.replication.settings.summary', { count: known.replicas })}
				icon="settings"
			>
				{#snippet footer()}
					<Link href={fileIndexHref}>{t('members.replication.settings.link')}</Link>
				{/snippet}
			</ClusterCard>
		{/if}
	</Stack>
</PageFrame>

<Drawer bind:open={moveOpen} title={t('members.replication.move.title')} width="form" modal footer={moveFooter}>
	<Stack gap="4">
		<Text tone="muted">{t('members.replication.move.help')}</Text>
		{#if null !== moveFailure}
			<ErrorState error={moveFailure} />
		{/if}
		<FormField label={t('members.replication.move.label')} for="replication-move-target">
			{#snippet control({ id, describedBy })}
				<Select {id} aria-describedby={describedBy} options={moveOptions} bind:value={moveTarget} />
			{/snippet}
		</FormField>
	</Stack>
</Drawer>

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
