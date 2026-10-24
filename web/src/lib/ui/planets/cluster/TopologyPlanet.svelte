<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { cluster } from '$lib/api/admin/cluster';
	import { instances, type InstanceList } from '$lib/api/admin/topology';
	import { toUiError } from '$lib/api/errors';
	import type { ClusterInfo, Result, ServerInfo } from '$lib/api/types';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import type { StatusKey } from '$lib/status/catalog';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import QuorumMeter from '$lib/ui/molecules/QuorumMeter.svelte';
	import ScopeChip from '$lib/ui/molecules/ScopeChip.svelte';
	import SegmentedControl, { type SegmentOption } from '$lib/ui/molecules/SegmentedControl.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import type StatTile from '$lib/ui/molecules/StatTile.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import TopologyMap, { type TopologyColorBy } from '$lib/ui/organisms/TopologyMap.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import type { UiError } from '$lib/ui/types';
	import ClusterCard from './ClusterCard.svelte';
	import FactList from './FactList.svelte';

	type StatTileProps = ComponentProps<typeof StatTile>;

	const REFRESH_MS = 5_000;
	const COLOR_BY_VALUES: readonly TopologyColorBy[] = ['state', 'latency', 'lag', 'disk'];

	const clusterQuery = createQuery<Result<ClusterInfo, UiError>>({
		key: ['cluster'],
		fetch: () => cluster(),
		refetchMs: REFRESH_MS,
		liveTopics: ['cluster.*']
	});

	const instancesQuery = createQuery<Result<InstanceList, UiError>>({
		key: ['instances'],
		fetch: async () => {
			const response = await instances();
			if (!response.ok) {
				return { ok: false, error: toUiError(response.error, { action: 'cluster.load' }) };
			}
			return { ok: true, value: response.value };
		},
		refetchMs: REFRESH_MS,
		liveTopics: ['cluster.*']
	});

	const colorOptions: SegmentOption[] = [
		{ value: 'state', label: t('topology.color.state') },
		{ value: 'latency', label: t('topology.color.latency') },
		{ value: 'lag', label: t('topology.color.lag') },
		{ value: 'disk', label: t('topology.color.disk') }
	];

	let colorValue = $state('state');
	let selected = $state<string | null>(null);

	const colorBy = $derived<TopologyColorBy>(COLOR_BY_VALUES.find((option) => option === colorValue) ?? 'state');
	const clusterResult = $derived(clusterQuery.data);
	const instancesResult = $derived(instancesQuery.data);
	const info = $derived(clusterResult?.ok ? clusterResult.value : undefined);
	const clusterError = $derived(clusterResult && !clusterResult.ok ? clusterResult.error : null);
	const instancesError = $derived(instancesResult && !instancesResult.ok ? instancesResult.error : null);
	const servers = $derived(instancesResult?.ok ? instancesResult.value.items : []);
	const members = $derived(info?.members ?? []);
	const upMembers = $derived(members.filter((member) => member.online).length);
	const reachableServers = $derived(servers.filter((server) => 'server.up' === reachStatus(server)).length);
	const totalFiles = $derived(servers.reduce((sum, server) => sum + server.files, 0));
	const totalBytes = $derived(servers.reduce((sum, server) => sum + server.bytes, 0));
	const isNats = $derived('nats' === info?.mode);
	const isSingle = $derived('single' === info?.mode);
	const runsAlone = $derived(isSingle && 1 >= servers.length);

	const tiles = $derived<StatTileProps[]>(
		info
			? [
					{
						label: t('topology.kpi.members'),
						value: upMembers,
						unit: 'count',
						base: { num: upMembers, den: info.replicas },
						status: quorumStatus(info)
					},
					{ label: t('topology.kpi.log'), value: info.log.messages, unit: 'count' },
					{
						label: t('topology.kpi.servers'),
						value: reachableServers,
						unit: 'count',
						base: { num: reachableServers, den: servers.length }
					}
				]
			: []
	);

	function quorumStatus(current: ClusterInfo): StatusKey {
		if (!current.has_quorum) {
			return 'quorum.lost';
		}
		return 0 < current.can_lose ? 'quorum.ok' : 'quorum.at_risk';
	}

	function reachStatus(server: ServerInfo): StatusKey {
		if (true === server.reachable || (server.is_self && null === server.reachable)) {
			return 'server.up';
		}
		return false === server.reachable ? 'server.down' : 'server.unknown';
	}

	function retry(): void {
		void clusterQuery.refetch();
		void instancesQuery.refetch();
	}

	function addMember(): void {
		void goto(`${base}/cluster/nodes?add=member`);
	}

	function addServer(): void {
		void goto(`${base}/cluster/servers?add=server`);
	}

	function openNode(id: string): void {
		const member = members.find((candidate) => candidate.node_id === id);
		void goto(member ? `${base}/cluster/nodes/${encodeURIComponent(id)}` : `${base}/cluster/servers`);
	}
</script>

{#snippet scope()}
	{#if info}
		<ScopeChip server={info.self_id} leader={info.leader_id === info.self_id} />
	{/if}
{/snippet}

{#snippet status()}
	<StaleIndicator updatedAt={clusterQuery.updatedAt} intervalMs={REFRESH_MS} state="live" onrefresh={retry} />
{/snippet}

{#snippet primary()}
	{#if isNats}
		<Button variant="primary" iconStart="plus" onclick={addMember}>{t('topology.primary.add_member')}</Button>
	{:else if isSingle && !runsAlone}
		<Button variant="primary" iconStart="plus" onclick={addServer}>{t('topology.primary.add_server')}</Button>
	{/if}
{/snippet}

<PageFrame title={t('topology.title')} job={t('topology.job')} {scope} {status} {primary}>
	<Stack gap="6">
		{#if undefined === clusterResult}
			<Skeleton shape="rect" height="var(--space-16)" />
			<Skeleton shape="rect" height="16rem" />
		{:else if clusterError}
			<ErrorState error={clusterError} onretry={retry} />
		{:else if info}
			{#if instancesError}
				<InlineAlert tone="warn" title={t('topology.partial.title')} body={t('topology.partial.body')} />
			{/if}
			{#if runsAlone}
				<KpiStrip
					tiles={[
						{ label: t('topology.kpi.servers'), value: reachableServers, unit: 'count', base: { num: reachableServers, den: servers.length } },
						{ label: t('cluster.topology.kpi.files'), value: totalFiles, unit: 'count' },
						{ label: t('cluster.topology.kpi.bytes'), value: totalBytes, unit: 'bytes' }
					]}
				/>
				<div class="cards">
					{#each servers as server (server.instance_id)}
						{#snippet reachValue()}
							<StatusPill status={reachStatus(server)} size="sm" />
						{/snippet}
						<ClusterCard
							title={server.instance_id}
							description={t('cluster.topology.self.body')}
							icon="server"
							tone={false === server.reachable ? 'danger' : 'ok'}
						>
							{#snippet meta()}
								<StatusPill status="server.self" size="sm" />
							{/snippet}
							<FactList
								items={[
									{ term: t('topology.member.server_address'), value: server.endpoint, mono: true },
									{ term: t('cluster.topology.fact.mode'), value: t('cluster.topology.mode.single') },
									{ term: t('topology.table.state'), value: reachValue }
								]}
							/>
						</ClusterCard>
					{/each}
					<ClusterCard title={t('topology.single.title')} description={t('topology.single.body')} icon="network">
						<ol class="steps">
							<li>{t('cluster.topology.single.step1')}</li>
							<li>{t('cluster.topology.single.step2')}</li>
							<li>{t('cluster.topology.single.step3')}</li>
						</ol>
						{#snippet footer()}
							<Link href={`${base}/settings/index`}>{t('cluster.topology.single.link')}</Link>
						{/snippet}
					</ClusterCard>
				</div>
			{:else}
				<KpiStrip {tiles} />
				<ClusterCard
					title={t('topology.map.label')}
					description={info.leader_id ? t('topology.leader.line', { leader: info.leader_id }) : t('topology.leader.none')}
					icon="network"
					tone={info.has_quorum ? 'neutral' : 'danger'}
				>
					{#snippet meta()}
						<SegmentedControl bind:value={colorValue} options={colorOptions} label={t('topology.color.label')} />
					{/snippet}
					<TopologyMap {members} {servers} {colorBy} bind:selected onopen={openNode} />
				</ClusterCard>
				{#if isNats}
					<ClusterCard title={t('cluster.topology.quorum.title')} description={t('cluster.topology.quorum.body')} icon="shield">
						<QuorumMeter voters={info.replicas} healthy={upMembers} />
					</ClusterCard>
				{/if}
			{/if}
		{/if}
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 20)), 1fr));
		gap: var(--space-4);
	}

	.steps {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding-inline-start: var(--space-5);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text);
	}
</style>
