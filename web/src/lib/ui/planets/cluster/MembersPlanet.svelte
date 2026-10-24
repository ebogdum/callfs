<script lang="ts" module>
	import type { ClusterMember } from '$lib/api/types';

	export const COMMANDS_CONTEXT = 'callfs.commands';
	export const CLUSTER_REFRESH_MS = 5000;
</script>

<script lang="ts">
	import { getContext, untrack } from 'svelte';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import {
		cluster,
		joinConfig,
		removeNode,
		transferLeadership,
		type JoinConfig,
		type JoinConfigRequest
	} from '$lib/api/admin/cluster';
	import type { ClusterInfo, Result } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import type { StatusKey } from '$lib/status/catalog';
	import type { ColumnDef, MenuItem, QueryStatus, UiError } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import CopyButton from '../../atoms/CopyButton.svelte';
	import Heading from '../../atoms/Heading.svelte';
	import Input from '../../atoms/Input.svelte';
	import Link from '../../atoms/Link.svelte';
	import Text from '../../atoms/Text.svelte';
	import DurationCell from '../../cells/DurationCell.svelte';
	import NumberCell from '../../cells/NumberCell.svelte';
	import StatusCell from '../../cells/StatusCell.svelte';
	import TextCell from '../../cells/TextCell.svelte';
	import ConfirmInline from '../../molecules/ConfirmInline.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import FormField from '../../molecules/FormField.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import QuorumMeter from '../../molecules/QuorumMeter.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import ApiEquivalent from '../../organisms/ApiEquivalent.svelte';
	import Drawer from '../../organisms/Drawer.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import ClusterCard from './ClusterCard.svelte';

	const STATUS_BY_STATE: Record<ClusterMember['state'], StatusKey> = {
		leader: 'member.leader',
		follower: 'member.follower',
		offline: 'member.unreachable'
	};

	const NODE_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9-]*$/;
	const CLUSTER_ADDRESS_PATTERN = /^[^\s:/]+:\d{1,5}$/;
	const SERVER_ADDRESS_PATTERN = /^https?:\/\/\S+$/;

	const membersQuery = createQuery<Result<ClusterInfo, UiError>>({
		key: ['cluster'],
		fetch: () => cluster(),
		staleMs: CLUSTER_REFRESH_MS,
		refetchMs: CLUSTER_REFRESH_MS,
		revalidateOnFocus: true
	});

	let known = $state<ClusterInfo | null>(null);
	$effect(() => {
		const latest = membersQuery.data;
		if (undefined !== latest && true === latest.ok) {
			known = latest.value;
		}
	});

	const failure = $derived.by<UiError | null>(() => {
		const latest = membersQuery.data;
		return undefined !== latest && false === latest.ok ? latest.error : null;
	});

	const members = $derived(known?.members ?? []);
	const single = $derived('single' === known?.mode);
	const onlineCount = $derived(members.filter((member) => member.online).length);

	const tableStatus = $derived.by<QueryStatus>(() => {
		if (null !== failure && null === known) {
			return 'error';
		}
		if (null === known) {
			return 'loading';
		}
		if (null !== failure) {
			return 'error';
		}
		if (0 === members.length) {
			return 'empty';
		}
		return 'success';
	});

	const indicatorState = $derived(null === failure ? 'live' : 'offline');

	let actionFailure = $state<UiError | null>(null);

	let addOpen = $state('member' === page.url.searchParams.get('add'));
	let nodeId = $state('');
	let apiEndpoint = $state('');
	let clusterAddr = $state('');
	let fieldErrors = $state<{ id: string | null; cluster: string | null; api: string | null }>({
		id: null,
		cluster: null,
		api: null
	});
	let generating = $state(false);
	let generated = $state<JoinConfig | null>(null);
	let addFailure = $state<UiError | null>(null);

	const joinRequest = $derived<JoinConfigRequest>({
		node_id: nodeId.trim(),
		api_endpoint: apiEndpoint.trim(),
		cluster_addr: clusterAddr.trim()
	});
	const hasDraft = $derived(null === generated && ('' !== nodeId || '' !== clusterAddr || '' !== apiEndpoint));

	let removeNodeId = $state<string | null>(page.url.searchParams.get('remove'));
	let removeOpen = $state(null !== page.url.searchParams.get('remove'));
	let removeFailure = $state<UiError | null>(null);
	let removing = $state(false);

	const removeTarget = $derived(members.find((member) => member.node_id === removeNodeId));
	const removeConflict = $derived(null !== removeFailure && 409 === removeFailure.status);

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'cluster.member.add',
				title: t('members.add.button'),
				group: 'Cluster',
				keywords: [t('members.title')],
				icon: 'plus',
				when: (ctx) => 'admin' === ctx.role,
				run: openAdd
			}
		]));
	});

	function refresh(): void {
		void membersQuery.refetch();
	}

	function openAdd(): void {
		generated = null;
		addFailure = null;
		addOpen = true;
	}

	function nodeIdError(id: string): string | null {
		if ('' === id) {
			return t('members.add.error.id_required');
		}
		if (false === NODE_ID_PATTERN.test(id)) {
			return t('members.add.error.id_format');
		}
		if (members.some((member) => member.node_id === id)) {
			return t('members.add.error.id_taken');
		}
		return null;
	}

	function checkAbout(): boolean {
		const next = {
			id: nodeIdError(joinRequest.node_id),
			cluster: CLUSTER_ADDRESS_PATTERN.test(joinRequest.cluster_addr) ? null : t('members.add.error.cluster'),
			api: SERVER_ADDRESS_PATTERN.test(joinRequest.api_endpoint) ? null : t('members.add.error.api')
		};
		fieldErrors = next;
		return null === next.id && null === next.cluster && null === next.api;
	}

	async function generate(): Promise<void> {
		if (true === generating || false === checkAbout()) {
			return;
		}
		generating = true;
		addFailure = null;
		const outcome = await joinConfig(joinRequest);
		generating = false;
		if (false === outcome.ok) {
			addFailure = outcome.error;
			return;
		}
		generated = outcome.value;
	}

	function editDetails(): void {
		generated = null;
	}

	function finishAdd(): void {
		nodeId = '';
		apiEndpoint = '';
		clusterAddr = '';
		generated = null;
		addOpen = false;
	}

	function openRemove(id: string): void {
		removeNodeId = id;
		removeFailure = null;
		removeOpen = true;
	}

	async function runTransfer(nodeIdToLead: string | null): Promise<void> {
		actionFailure = null;
		const outcome = await transferLeadership(nodeIdToLead);
		if (false === outcome.ok) {
			actionFailure = outcome.error;
			return;
		}
		announce(t('members.transfer.done', { leader: outcome.value.leader_id ?? t('common.unknown') }), 'polite');
		invalidate(['cluster']);
	}

	async function runRemove(): Promise<void> {
		if (null === removeNodeId || true === removing) {
			return;
		}
		const id = removeNodeId;
		removing = true;
		removeFailure = null;
		const outcome = await removeNode(id);
		removing = false;
		if (false === outcome.ok) {
			removeFailure = outcome.error;
			return;
		}
		announce(t('members.remove.done', { node: id }), 'polite');
		removeNodeId = null;
		removeOpen = false;
		invalidate(['cluster']);
	}

	function menuFor(member: ClusterMember): MenuItem[] {
		const items: MenuItem[] = [];
		if ('leader' !== member.state && true === member.online) {
			items.push({ id: 'transfer', label: t('members.menu.transfer'), onselect: () => void runTransfer(member.node_id) });
		}
		if ('offline' === member.state && false === member.is_self) {
			items.push({
				id: 'remove',
				label: t('members.menu.remove'),
				tone: 'danger',
				onselect: () => openRemove(member.node_id)
			});
		}
		return items;
	}
</script>

{#snippet memberCell(member: ClusterMember)}
	<Link href={`${base}/cluster/nodes/${encodeURIComponent(member.node_id)}`}>{member.node_id}</Link>
{/snippet}

{#snippet stateCell(member: ClusterMember)}
	<StatusCell status={STATUS_BY_STATE[member.state]} />
{/snippet}

{#snippet onlineCell(member: ClusterMember)}
	<TextCell value={member.online ? t('members.online.yes') : t('members.online.no')} />
{/snippet}

{#snippet serverCell(member: ClusterMember)}
	<span class="address" title={member.api_endpoint ?? undefined}>{member.api_endpoint ?? t('common.unknown')}</span>
{/snippet}

{#snippet seenCell(member: ClusterMember)}
	<DurationCell value={member.last_seen_ms ?? t('common.unknown')} />
{/snippet}

{#snippet behindCell(member: ClusterMember)}
	<NumberCell value={member.behind_by} />
{/snippet}

{#snippet consequences()}
	<Stack gap="1" as="ul">
		<li>{t('members.remove.from_to', { from: formatNumber(members.length), to: formatNumber(members.length - 1) })}</li>
		<li>{t('members.remove.moves')}</li>
		<li>{t('members.remove.stops_index')}</li>
	</Stack>
{/snippet}

{#snippet addFooter()}
	<Cluster justify="end" gap="2">
		{#if null === generated}
			<Button variant="primary" loading={generating} onclick={generate}>{t('members.add.generate')}</Button>
		{:else}
			<Button variant="ghost" onclick={editDetails}>{t('members.add.edit')}</Button>
			<Button variant="primary" onclick={finishAdd}>{t('members.add.close')}</Button>
		{/if}
	</Cluster>
{/snippet}

<PageFrame title={t('members.title')} job={t('members.job')}>
	{#snippet primary()}
		{#if !single}
			<Button variant="primary" iconStart="plus" onclick={openAdd}>{t('members.add.button')}</Button>
		{/if}
	{/snippet}
	{#snippet status()}
		<StaleIndicator updatedAt={membersQuery.updatedAt} intervalMs={CLUSTER_REFRESH_MS} state={indicatorState} onrefresh={refresh} />
	{/snippet}

	<Stack gap="6">
		{#if null !== actionFailure}
			<ErrorState error={actionFailure} variant="region" />
		{/if}
		{#if single}
			<EmptyState
				variant="not-available"
				icon="server"
				title={t('members.single.title')}
				body={t('members.single.body')}
				primary={{ label: t('cluster.topology.single.link'), href: `${base}/settings/index` }}
			/>
		{:else}
			{#if null !== known}
				<ClusterCard title={t('cluster.topology.quorum.title')} description={t('cluster.topology.quorum.body')} icon="shield">
					<QuorumMeter voters={known.replicas} healthy={onlineCount} />
				</ClusterCard>
			{/if}
			<DataTable
				rows={members}
				columns={[
					{ id: 'member', header: t('members.table.member'), cell: memberCell, width: 140, priority: 1 },
					{ id: 'state', header: t('members.table.state'), cell: stateCell, width: 130, priority: 1 },
					{ id: 'online', header: t('members.table.online'), cell: onlineCell, width: 90, priority: 2 },
					{ id: 'seen', header: t('members.table.last_seen'), cell: seenCell, align: 'end', width: 110, priority: 2 },
					{ id: 'behind', header: t('members.table.behind'), cell: behindCell, align: 'end', width: 110, priority: 2 },
					{ id: 'server', header: t('members.table.server_address'), cell: serverCell, width: 260, minWidth: 200, priority: 1 }
				] satisfies ColumnDef<ClusterMember>[]}
				rowKey={(member) => member.node_id}
				label={t('members.table.label')}
				status={tableStatus}
				error={failure ?? undefined}
				rowMenu={menuFor}
				onretry={refresh}
			>
				{#snippet empty()}
					<EmptyState
						variant="first-use"
						icon="users"
						title={t('members.empty.title')}
						body={t('members.empty.body')}
					/>
				{/snippet}
			</DataTable>
		{/if}
	</Stack>
</PageFrame>

<Drawer bind:open={addOpen} title={t('members.add.title')} width="form" modal dirty={hasDraft} footer={addFooter}>
	<Stack gap="6">
		{#if null !== addFailure}
			<ErrorState error={addFailure} />
		{/if}
		{#if null === generated}
			<Stack gap="4">
				<FormField
					label={t('members.add.id.label')}
					for="members-add-id"
					help={t('members.add.id.help')}
					example={t('members.add.id.example')}
					error={fieldErrors.id ?? undefined}
				>
					{#snippet control({ id, describedBy, invalid })}
						<Input {id} aria-describedby={describedBy} {invalid} mono bind:value={nodeId} />
					{/snippet}
				</FormField>
				<FormField
					label={t('members.add.api.label')}
					for="members-add-api"
					help={t('members.add.api.help')}
					example={t('members.add.api.example')}
					error={fieldErrors.api ?? undefined}
				>
					{#snippet control({ id, describedBy, invalid })}
						<Input {id} aria-describedby={describedBy} {invalid} mono bind:value={apiEndpoint} />
					{/snippet}
				</FormField>
				<FormField
					label={t('members.add.cluster.label')}
					for="members-add-cluster"
					help={t('members.add.cluster.help')}
					example={t('members.add.cluster.example')}
					error={fieldErrors.cluster ?? undefined}
				>
					{#snippet control({ id, describedBy, invalid })}
						<Input {id} aria-describedby={describedBy} {invalid} mono bind:value={clusterAddr} />
					{/snippet}
				</FormField>
			</Stack>
		{:else}
			<Stack gap="4">
				<Stack gap="1-5">
					<Heading level={3} size="title-sm">{t('members.add.config.title', { node: joinRequest.node_id })}</Heading>
					<Text tone="muted">{t('members.add.config.help')}</Text>
				</Stack>
				<pre class="yaml">{generated.yaml}</pre>
				<div>
					<CopyButton value={generated.yaml} variant="button" />
				</div>
				<Stack gap="2">
					<Heading level={3} size="title-sm">{t('members.add.steps.title')}</Heading>
					<ol class="steps">
						{#each generated.steps as step (step)}
							<li>{step}</li>
						{/each}
					</ol>
				</Stack>
				<ApiEquivalent
					request={{
						method: 'GET',
						url: `/v1/admin/cluster/join-config?node_id=${encodeURIComponent(joinRequest.node_id)}&api_endpoint=${encodeURIComponent(joinRequest.api_endpoint)}&cluster_addr=${encodeURIComponent(joinRequest.cluster_addr)}`
					}}
				/>
			</Stack>
		{/if}
	</Stack>
</Drawer>

<Drawer
	bind:open={removeOpen}
	title={t('members.remove.title', { node: removeNodeId ?? '' })}
	width="form"
	modal
>
	{#if undefined !== removeTarget}
		<Stack gap="4">
			{#if removeConflict}
				<InlineAlert
					tone="warn"
					title={t('members.remove.online.title', { node: removeTarget.node_id })}
					body={t('members.remove.online.body')}
				/>
			{:else if null !== removeFailure}
				<ErrorState error={removeFailure} />
			{/if}
			<ConfirmInline
				question={t('members.remove.title', { node: removeTarget.node_id })}
				consequence={consequences}
				confirmLabel={t('members.remove.confirm')}
				tone="danger"
				busy={removing}
				onconfirm={() => void runRemove()}
				oncancel={() => (removeOpen = false)}
			/>
		</Stack>
	{/if}
</Drawer>

<style>
	.address {
		font-family: var(--font-mono);
		font-size: var(--table-text);
		white-space: nowrap;
	}

	.yaml {
		margin: 0;
		padding: var(--space-3);
		overflow-x: auto;
		border-radius: var(--radius-sm);
		background: var(--code-bg);
		color: var(--text);
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
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
