<script lang="ts" module>
	import type { ClusterMember } from '$lib/api/types';

	export const COMMANDS_CONTEXT = 'callfs.commands';
	export const CLUSTER_REFRESH_MS = 5000;
	export const SERVING_STATES: ClusterMember['state'][] = ['leader', 'follower'];
</script>

<script lang="ts">
	import { getContext, untrack } from 'svelte';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import {
		cluster,
		join,
		preflight,
		removeNode,
		transferLeadership,
		type NodeRequest
	} from '$lib/api/admin/cluster';
	import type { ClusterInfo, Preflight, Result } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import type { StatusKey } from '$lib/status/catalog';
	import type { ColumnDef, MenuItem, QueryStatus, UiError } from '$lib/ui/types';
	import { announce } from '../../quarks/announce';
	import Button from '../../atoms/Button.svelte';
	import Code from '../../atoms/Code.svelte';
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
	import Stepper, { type StepperStep } from '../../molecules/Stepper.svelte';
	import ApiEquivalent from '../../organisms/ApiEquivalent.svelte';
	import Drawer from '../../organisms/Drawer.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import StatusGlyph from '../../quarks/StatusGlyph.svelte';
	import ClusterCard from './ClusterCard.svelte';

	const STATUS_BY_STATE: Record<ClusterMember['state'], StatusKey> = {
		leader: 'member.leader',
		follower: 'member.follower',
		candidate: 'member.candidate',
		shutdown: 'member.shutdown',
		unreachable: 'member.unreachable'
	};

	const CHECK_KEYS: Record<Preflight['checks'][number]['name'], string> = {
		id_unique: 'members.checks.name.id_unique',
		addr_unused: 'members.checks.name.addr_unused',
		raft_reachable: 'members.checks.name.raft_reachable',
		api_reachable: 'members.checks.name.api_reachable',
		api_https: 'members.checks.name.api_https',
		secret_ok: 'members.checks.name.secret_ok'
	};

	const NODE_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9-]*$/;
	const RAFT_ADDRESS_PATTERN = /^[^\s:/]+:\d{1,5}$/;
	const SERVER_ADDRESS_PATTERN = /^https?:\/\/\S+$/;
	const ADD_STEPS: StepperStep[] = [
		{ id: 'about', label: t('members.add.steps.about') },
		{ id: 'checks', label: t('members.add.steps.checks') },
		{ id: 'review', label: t('members.add.steps.review') }
	];

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
	const healthyCount = $derived(members.filter((member) => SERVING_STATES.includes(member.state)).length);

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
	let transferring = $state(false);

	let addOpen = $state(false);
	let addStep = $state(0);
	let nodeId = $state('');
	let raftAddr = $state('');
	let apiEndpoint = $state('');
	let fieldErrors = $state<{ id: string | null; raft: string | null; api: string | null }>({
		id: null,
		raft: null,
		api: null
	});
	let checks = $state<Preflight | null>(null);
	let checking = $state(false);
	let checkFailure = $state<UiError | null>(null);
	let joining = $state(false);
	let addFailure = $state<UiError | null>(null);

	const request = $derived<NodeRequest>({
		node_id: nodeId.trim(),
		raft_addr: raftAddr.trim(),
		api_endpoint: apiEndpoint.trim()
	});
	const hasDraft = $derived('' !== nodeId || '' !== raftAddr || '' !== apiEndpoint);
	const blockingChecks = $derived(checks?.checks.filter((check) => !check.ok && 'addr_unused' !== check.name) ?? []);
	const checksReason = $derived.by<string | undefined>(() => {
		if (null !== checkFailure) {
			return t('members.checks.blocked');
		}
		if (null === checks) {
			return t('members.checks.loading');
		}
		if (0 < blockingChecks.length) {
			return t('members.checks.blocked');
		}
		return undefined;
	});
	const cliCommand = $derived(
		`callfs cluster join ${request.node_id} --raft-addr ${request.raft_addr} --api-endpoint ${request.api_endpoint}`
	);

	let removeNodeId = $state<string | null>(page.url.searchParams.get('remove'));
	let removeOpen = $state(null !== page.url.searchParams.get('remove'));
	let removeFailure = $state<UiError | null>(null);
	let removing = $state(false);

	const removeTarget = $derived(members.find((member) => member.node_id === removeNodeId));
	const removal = $derived.by(() => {
		if (undefined === removeTarget || null === known) {
			return null;
		}
		const votersAfter = known.voters - ('voter' === removeTarget.suffrage ? 1 : 0);
		const healthyAfter = members.filter(
			(member) =>
				member.node_id !== removeTarget.node_id && 'voter' === member.suffrage && SERVING_STATES.includes(member.state)
		).length;
		const quorumNeeded = Math.floor(votersAfter / 2) + 1;
		const canLoseAfter = Math.max(0, Math.floor((votersAfter - 1) / 2) - (votersAfter - healthyAfter));
		return {
			votersAfter,
			blocked: healthyAfter < quorumNeeded,
			loseNone: 0 === canLoseAfter
		};
	});
	const removalBlocked = $derived(removal?.blocked ?? false);
	const removalLoseNone = $derived(removal?.loseNone ?? false);

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
		addStep = 0;
		checks = null;
		checkFailure = null;
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
			id: nodeIdError(request.node_id),
			raft: RAFT_ADDRESS_PATTERN.test(request.raft_addr) ? null : t('members.add.error.raft'),
			api: SERVER_ADDRESS_PATTERN.test(request.api_endpoint) ? null : t('members.add.error.api')
		};
		fieldErrors = next;
		return null === next.id && null === next.raft && null === next.api;
	}

	async function runChecks(): Promise<void> {
		checking = true;
		checkFailure = null;
		checks = null;
		const outcome = await preflight(request);
		checking = false;
		if (false === outcome.ok) {
			checkFailure = outcome.error;
			return;
		}
		checks = outcome.value;
	}

	function advance(): void {
		if (0 === addStep) {
			if (false === checkAbout()) {
				return;
			}
			addStep = 1;
			void runChecks();
			return;
		}
		if (1 === addStep) {
			addStep = 2;
		}
	}

	function back(): void {
		if (0 < addStep) {
			addStep -= 1;
		}
	}

	async function submitJoin(): Promise<void> {
		if (true === joining) {
			return;
		}
		joining = true;
		addFailure = null;
		const outcome = await join(request);
		joining = false;
		if (false === outcome.ok) {
			addFailure = outcome.error;
			return;
		}
		const message =
			'already_member' === outcome.value.status
				? t('members.add.already', { node: request.node_id })
				: t('members.add.done', { node: request.node_id });
		announce(message, 'polite');
		nodeId = '';
		raftAddr = '';
		apiEndpoint = '';
		checks = null;
		addStep = 0;
		addOpen = false;
		invalidate(['cluster']);
	}

	function openRemove(id: string): void {
		removeNodeId = id;
		removeFailure = null;
		removeOpen = true;
	}

	async function runTransfer(nodeIdToLead: string | null): Promise<void> {
		if (true === transferring) {
			return;
		}
		transferring = true;
		actionFailure = null;
		const outcome = await transferLeadership(nodeIdToLead);
		transferring = false;
		if (false === outcome.ok) {
			actionFailure = outcome.error;
			return;
		}
		announce(t('members.transfer.done', { leader: outcome.value.leader_id ?? t('common.unknown') }), 'polite');
		invalidate(['cluster']);
	}

	function transferFirst(): void {
		void runTransfer(null);
	}

	async function runRemove(force: boolean): Promise<void> {
		if (null === removeNodeId || true === removing) {
			return;
		}
		const id = removeNodeId;
		removing = true;
		removeFailure = null;
		const outcome = await removeNode(id, force);
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
		if ('leader' !== member.state && 'voter' === member.suffrage) {
			items.push({ id: 'transfer', label: t('members.menu.transfer'), onselect: () => void runTransfer(member.node_id) });
		}
		items.push({
			id: 'remove',
			label: t('members.menu.remove'),
			tone: 'danger',
			onselect: () => openRemove(member.node_id)
		});
		return items;
	}
</script>

{#snippet memberCell(member: ClusterMember)}
	<Link href={`${base}/cluster/nodes/${encodeURIComponent(member.node_id)}`}>{member.node_id}</Link>
{/snippet}

{#snippet stateCell(member: ClusterMember)}
	<StatusCell status={STATUS_BY_STATE[member.state]} />
{/snippet}

{#snippet voteCell(member: ClusterMember)}
	<TextCell value={'voter' === member.suffrage ? t('members.vote.voter') : t('members.vote.nonvoter')} />
{/snippet}

{#snippet raftCell(member: ClusterMember)}
	<TextCell value={member.raft_addr} mono />
{/snippet}

{#snippet serverCell(member: ClusterMember)}
	<TextCell value={member.api_endpoint} mono />
{/snippet}

{#snippet contactCell(member: ClusterMember)}
	<DurationCell value={member.last_contact_ms ?? t('common.unknown')} />
{/snippet}

{#snippet behindCell(member: ClusterMember)}
	<NumberCell value={member.behind_by} />
{/snippet}

{#snippet consequences()}
	<Stack gap="1" as="ul">
		<li>{t('members.remove.from_to', { from: formatNumber(members.length), to: formatNumber(members.length - 1) })}</li>
		{#if removalLoseNone}
			<li>{t('members.remove.no_loss')}</li>
		{/if}
		<li>{t('members.remove.stops_index')}</li>
	</Stack>
{/snippet}

{#snippet addFooter()}
	<Cluster justify="end" gap="2">
		{#if 0 < addStep}
			<Button variant="ghost" onclick={back}>{t('members.add.back')}</Button>
		{/if}
		{#if 2 > addStep}
			<Button variant="primary" disabledReason={1 === addStep ? checksReason : undefined} onclick={advance}>
				{t('members.add.next')}
			</Button>
		{:else}
			<Button variant="primary" loading={joining} disabledReason={checksReason} onclick={submitJoin}>
				{t('members.add.button')}
			</Button>
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
					<QuorumMeter voters={known.voters} healthy={healthyCount} />
				</ClusterCard>
			{/if}
			<DataTable
				rows={members}
				columns={[
					{ id: 'member', header: t('members.table.member'), cell: memberCell },
					{ id: 'state', header: t('members.table.state'), cell: stateCell },
					{ id: 'vote', header: t('members.table.vote'), cell: voteCell },
					{ id: 'raft', header: t('members.table.raft_address'), cell: raftCell },
					{ id: 'server', header: t('members.table.server_address'), cell: serverCell },
					{ id: 'contact', header: t('members.table.last_contact'), cell: contactCell, align: 'end' },
					{ id: 'behind', header: t('members.table.behind'), cell: behindCell, align: 'end' }
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
		<Stepper steps={ADD_STEPS} current={addStep} />
		{#if null !== addFailure}
			<ErrorState error={addFailure} />
		{/if}
		{#if 0 === addStep}
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
					label={t('members.add.raft.label')}
					for="members-add-raft"
					help={t('members.add.raft.help')}
					example={t('members.add.raft.example')}
					error={fieldErrors.raft ?? undefined}
				>
					{#snippet control({ id, describedBy, invalid })}
						<Input {id} aria-describedby={describedBy} {invalid} mono bind:value={raftAddr} />
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
			</Stack>
		{:else if 1 === addStep}
			{#if null !== checkFailure}
				<ErrorState error={checkFailure} onretry={runChecks} />
			{:else if null === checks}
				<Text tone="muted">{t('members.checks.loading')}</Text>
			{:else}
				{#if 0 < checks.would_replace.length}
					<InlineAlert
						tone="warn"
						title={t('members.checks.replace', { node: checks.would_replace.join(', ') })}
					/>
				{/if}
				<Stack gap="3" as="ul">
					{#each checks.checks as check (check.name)}
						<li>
							<Cluster gap="2" align="start">
								<StatusGlyph tone={check.ok ? 'ok' : 'danger'} />
								<Stack gap="1">
									<Text>{t(CHECK_KEYS[check.name])}</Text>
									<Text tone="muted">{check.ok ? t('members.checks.passed') : t('members.checks.failed')}</Text>
									{#if '' !== check.detail}
										<Text tone="faint">{check.detail}</Text>
									{/if}
								</Stack>
							</Cluster>
						</li>
					{/each}
				</Stack>
			{/if}
		{:else}
			<Stack gap="4">
				<Text>
					{t('members.review.sentence', {
						node: request.node_id,
						before: formatNumber(members.length),
						after: formatNumber(members.length + 1),
						quorum: formatNumber(Math.floor((members.length + 1) / 2) + 1)
					})}
				</Text>
				<Stack gap="1-5">
					<Text tone="muted">{t('members.review.cli')}</Text>
					<Code copy>{cliCommand}</Code>
				</Stack>
				<ApiEquivalent request={{ method: 'POST', url: '/v1/admin/cluster/nodes', body: request }} />
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
	{#if undefined !== removeTarget && null !== removal}
		<Stack gap="4">
			{#if null !== actionFailure}
				<ErrorState error={actionFailure} />
			{/if}
			{#if removalBlocked}
				<InlineAlert
					tone="danger"
					title={t('members.remove.blocked.title', { node: removeTarget.node_id })}
					body={t('members.remove.blocked.body', { count: removal.votersAfter })}
				/>
			{/if}
			{#if null !== removeFailure}
				<ErrorState error={removeFailure} />
			{/if}
			{#if 'leader' === removeTarget.state}
				<Button variant="primary" loading={transferring} onclick={transferFirst}>
					{t('members.remove.transfer_first')}
				</Button>
			{/if}
			<ConfirmInline
				question={t('members.remove.title', { node: removeTarget.node_id })}
				consequence={consequences}
				confirmLabel={removalBlocked ? t('members.remove.anyway') : t('members.remove.confirm')}
				typedWord={removalBlocked ? 'REMOVE' : undefined}
				tone="danger"
				busy={removing}
				onconfirm={() => void runRemove(removalBlocked)}
				oncancel={() => (removeOpen = false)}
			/>
		</Stack>
	{/if}
</Drawer>
