<script lang="ts">
	import { getContext, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { cluster, transferLeadership } from '$lib/api/admin/cluster';
	import type { ClusterInfo, Result } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import type { UiError } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Breadcrumbs from '../../molecules/Breadcrumbs.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import MemberCard from '../../organisms/MemberCard.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { CLUSTER_REFRESH_MS, COMMANDS_CONTEXT } from './MembersPlanet.svelte';

	let { id }: { id: string } = $props();

	const membersHref = `${base}/cluster/nodes`;

	const clusterQuery = createQuery<Result<ClusterInfo, UiError>>({
		key: ['cluster'],
		fetch: () => cluster(),
		staleMs: CLUSTER_REFRESH_MS,
		refetchMs: CLUSTER_REFRESH_MS,
		revalidateOnFocus: true
	});

	let known = $state<ClusterInfo | null>(null);
	$effect(() => {
		const latest = clusterQuery.data;
		if (undefined !== latest && true === latest.ok) {
			known = latest.value;
		}
	});

	const failure = $derived.by<UiError | null>(() => {
		const latest = clusterQuery.data;
		return undefined !== latest && false === latest.ok ? latest.error : null;
	});

	const member = $derived(known?.members.find((candidate) => candidate.node_id === id));
	const viewStatus = $derived.by<'loading' | 'error' | 'missing' | 'success'>(() => {
		if (null !== failure && null === known) {
			return 'error';
		}
		if (null === known) {
			return 'loading';
		}
		if (undefined === member) {
			return 'missing';
		}
		return 'success';
	});
	const indicatorState = $derived(null === failure ? 'live' : 'offline');

	let actionFailure = $state<UiError | null>(null);
	let transferring = $state(false);

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'cluster.member.transfer',
				title: t('members.detail.transfer'),
				group: 'Cluster',
				keywords: [id],
				icon: 'leader',
				when: (ctx) => 'admin' === ctx.role,
				run: transferHere
			},
			{
				id: 'cluster.member.remove',
				title: t('members.menu.remove'),
				group: 'Cluster',
				keywords: [id],
				icon: 'trash',
				when: (ctx) => 'admin' === ctx.role,
				run: removeMember
			}
		]));
	});

	function refresh(): void {
		void clusterQuery.refetch();
	}

	async function runTransfer(nodeId: string): Promise<void> {
		if (true === transferring) {
			return;
		}
		transferring = true;
		actionFailure = null;
		const outcome = await transferLeadership(nodeId);
		transferring = false;
		if (false === outcome.ok) {
			actionFailure = outcome.error;
			return;
		}
		announce(t('members.transfer.done', { leader: outcome.value.leader_id ?? t('common.unknown') }), 'polite');
		invalidate(['cluster']);
	}

	function transferHere(): void {
		void runTransfer(id);
	}

	function removeMember(): void {
		void goto(`${membersHref}?remove=${encodeURIComponent(id)}`);
	}
</script>

<PageFrame title={t('members.detail.title', { node: id })} job={t('members.detail.job')}>
	{#snippet breadcrumbs()}
		<Breadcrumbs
			label={t('members.breadcrumbs')}
			segments={[
				{ label: t('members.title'), href: membersHref },
				{ label: id, href: `${membersHref}/${encodeURIComponent(id)}` }
			]}
		/>
	{/snippet}
	{#snippet primary()}
		{#if undefined !== member && 'leader' !== member.state}
			<Button
				variant="primary"
				loading={transferring}
				disabledReason={'voter' === member.suffrage ? undefined : t('members.detail.transfer.nonvoter')}
				onclick={transferHere}
			>
				{t('members.detail.transfer')}
			</Button>
		{/if}
	{/snippet}
	{#snippet secondary()}
		{#if undefined !== member}
			<Button variant="secondary" onclick={removeMember}>{t('members.menu.remove')}</Button>
		{/if}
	{/snippet}
	{#snippet status()}
		<StaleIndicator updatedAt={clusterQuery.updatedAt} intervalMs={CLUSTER_REFRESH_MS} state={indicatorState} onrefresh={refresh} />
	{/snippet}

	<Stack gap="4">
		{#if null !== actionFailure}
			<ErrorState error={actionFailure} />
		{/if}
		{#if 'loading' === viewStatus}
			<Skeleton lines={3} />
		{:else if 'error' === viewStatus && null !== failure}
			<ErrorState error={failure} onretry={refresh} />
		{:else if 'missing' === viewStatus}
			<EmptyState
				variant="not-available"
				icon="users"
				title={t('members.detail.missing.title', { node: id })}
				body={t('members.detail.missing.body')}
				secondary={{ label: t('members.detail.back'), href: membersHref }}
			/>
		{:else if undefined !== member}
			{#if null !== failure}
				<ErrorState error={failure} variant="region" onretry={refresh} />
			{/if}
			<MemberCard {member} />
		{/if}
	</Stack>
</PageFrame>
