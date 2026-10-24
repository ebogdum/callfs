<script lang="ts">
	import {
		createUser,
		deleteUser,
		migratePositional,
		owners,
		reassignOwner,
		rotateKey,
		updateUser,
		users,
		type CreatedUser,
		type OwnerPage,
		type PositionalMapping
	} from '$lib/api/admin/users';
	import type { JobAccepted } from '$lib/api/admin/topology';
	import type { ApiFailure } from '$lib/api/errors';
	import type { OwnerStat, Result, UserInfo } from '$lib/api/types';
	import { base } from '$app/paths';
	import { t } from '$lib/copy/t';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import BooleanCell from '$lib/ui/cells/BooleanCell.svelte';
	import NumberCell from '$lib/ui/cells/NumberCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Badge from '$lib/ui/atoms/Badge.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Switch from '$lib/ui/atoms/Switch.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import CopyField from '$lib/ui/molecules/CopyField.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import FormField from '$lib/ui/molecules/FormField.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import ApiEquivalent from '$lib/ui/organisms/ApiEquivalent.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import type { ColumnDef, MenuItem, SortState } from '$lib/ui/types';
	import PositionalMigration from './PositionalMigration.svelte';
	import SettingsCard from './SettingsCard.svelte';
	import SettingsItem from './SettingsItem.svelte';

	const RESERVED_SOURCE = 'reserved';
	const POSITIONAL_SOURCE = 'api_keys';
	const STALE_INTERVAL_MS = 15000;

	async function unwrap<T>(outcome: Promise<Result<T, ApiFailure>>): Promise<T> {
		const result = await outcome;
		if (result.ok) {
			return result.value;
		}
		return Promise.reject(result.error);
	}

	async function listAllOwners(): Promise<OwnerStat[]> {
		const collected: OwnerStat[] = [];
		let cursor: string | null = null;
		do {
			const page: OwnerPage = await unwrap(owners(cursor));
			collected.push(...page.items);
			cursor = page.next_cursor;
		} while (null !== cursor);
		return collected;
	}

	const usersQuery = createQuery({
		key: ['users'],
		fetch: () => unwrap(users()),
		isEmpty: (page) => 0 === page.items.length
	});

	const ownersQuery = createQuery({
		key: ['owners'],
		fetch: () => listAllOwners()
	});

	const addMutation = createMutation({
		run: (vars: { id: string; isAdmin: boolean }) => unwrap(createUser(vars.id, vars.isAdmin)),
		invalidate: () => [['users']],
		context: { action: 'access.action.add', object: (vars) => vars.id }
	});

	const rotateMutation = createMutation({
		run: (id: string) => unwrap(rotateKey(id)),
		invalidate: () => [['users']],
		context: { action: 'access.action.rotate', object: (id) => id }
	});

	const adminMutation = createMutation({
		run: (vars: { id: string; isAdmin: boolean }) => unwrap(updateUser(vars.id, { is_admin: vars.isAdmin })),
		invalidate: () => [['users']],
		context: { action: 'access.action.admin', object: (vars) => vars.id }
	});

	const removeMutation = createMutation({
		run: (id: string) => unwrap(deleteUser(id)),
		invalidate: () => [['users'], ['owners']],
		context: { action: 'access.action.remove', object: (id) => id }
	});

	const transferMutation = createMutation({
		run: (vars: { from: string; to: string; prefix: string }) =>
			unwrap(reassignOwner(vars.from, vars.to, vars.prefix)),
		invalidate: () => [['users'], ['owners']],
		context: { action: 'access.action.transfer' }
	});

	const migrateMutation = createMutation({
		run: (mapping: PositionalMapping[]) => unwrap(migratePositional(mapping)),
		invalidate: () => [['users'], ['owners']],
		context: { action: 'access.action.migrate' }
	});

	let sort = $state<SortState | null>(null);
	let addOpen = $state(false);
	let addId = $state('');
	let addAdmin = $state(false);
	let issued = $state<CreatedUser | null>(null);
	let keyOpen = $state(false);
	let rotateOpen = $state(false);
	let rotateTarget = $state<UserInfo | null>(null);
	let removeOpen = $state(false);
	let removeTarget = $state<UserInfo | null>(null);
	let transferOpen = $state(false);
	let transferFrom = $state('');
	let transferTo = $state('');
	let transferFolder = $state('/');
	let transferJob = $state<string | null>(null);
	let migrateOpen = $state(false);
	let migrationJob = $state<string | null>(null);

	const allUsers = $derived(usersQuery.data?.items ?? []);
	const managed = $derived(allUsers.filter((user) => RESERVED_SOURCE !== user.source));
	const systemRows = $derived(allUsers.filter((user) => RESERVED_SOURCE === user.source));
	const reservedIds = $derived(systemRows.map((user) => user.user_id));
	const positionalRows = $derived(allUsers.filter((user) => POSITIONAL_SOURCE === user.source));
	const positionalCount = $derived(usersQuery.data?.positional_count ?? 0);
	const unownedFiles = $derived(ownersQuery.data?.find((owner) => '' === owner.owner)?.files ?? 0);
	const staleState = $derived('error' === usersQuery.status ? 'offline' : 'live');
	const pageError = $derived(rotateMutation.error ?? adminMutation.error ?? removeMutation.error ?? null);

	const addIdTrimmed = $derived(addId.trim());
	const addIsReserved = $derived(reservedIds.includes(addIdTrimmed));
	const addReason = $derived.by((): string | undefined => {
		if (0 === addIdTrimmed.length) {
			return t('access.add.user_id.empty');
		}
		if (addIsReserved) {
			return t('access.add.user_id.reserved');
		}
		return undefined;
	});
	const addRequest = $derived({
		method: 'POST' as const,
		url: '/v1/admin/users',
		body: { user_id: addIdTrimmed, is_admin: addAdmin }
	});

	const transferTarget = $derived(transferTo.trim());
	const transferFolderTrimmed = $derived(transferFolder.trim());
	const transferReason = $derived.by((): string | undefined => {
		if (0 === transferTarget.length) {
			return t('access.transfer.to.empty');
		}
		if (0 === transferFolderTrimmed.length) {
			return t('access.transfer.folder.empty');
		}
		return undefined;
	});
	const transferRequest = $derived({
		method: 'POST' as const,
		url: '/v1/admin/owners/reassign',
		body: { from: transferFrom, to: transferTarget, prefix: transferFolderTrimmed }
	});

	const toggleAdmin = (user: UserInfo) => {
		void adminMutation.run({ id: user.user_id, isAdmin: !user.is_admin });
	};

	const openAdd = () => {
		addOpen = true;
	};

	const submitAdd = async () => {
		if (undefined !== addReason) {
			return;
		}
		const created = await addMutation.run({ id: addIdTrimmed, isAdmin: addAdmin });
		if (null !== created) {
			addOpen = false;
			addId = '';
			addAdmin = false;
			issued = created;
			keyOpen = true;
		}
	};

	const openRotate = (user: UserInfo) => {
		rotateTarget = user;
		rotateOpen = true;
	};

	const confirmRotate = async () => {
		const target = rotateTarget;
		rotateOpen = false;
		rotateTarget = null;
		if (null === target) {
			return;
		}
		const created = await rotateMutation.run(target.user_id);
		if (null !== created) {
			issued = created;
			keyOpen = true;
		}
	};

	const openRemove = (user: UserInfo) => {
		removeTarget = user;
		removeOpen = true;
	};

	const confirmRemove = async () => {
		const target = removeTarget;
		removeOpen = false;
		removeTarget = null;
		if (null === target) {
			return;
		}
		await removeMutation.run(target.user_id);
	};

	const openTransfer = (from: string) => {
		transferFrom = from;
		transferTo = '';
		transferFolder = '/';
		transferOpen = true;
	};

	const submitTransfer = async () => {
		if (undefined !== transferReason) {
			return;
		}
		const accepted = await transferMutation.run({
			from: transferFrom,
			to: transferTarget,
			prefix: transferFolderTrimmed
		});
		if (null !== accepted) {
			transferOpen = false;
			transferJob = accepted.job_id;
		}
	};

	const openMigration = () => {
		migrateOpen = true;
	};

	const submitMigration = async (mapping: PositionalMapping[]) => {
		const accepted: JobAccepted | null = await migrateMutation.run(mapping);
		if (null !== accepted) {
			migrateOpen = false;
			migrationJob = accepted.job_id;
		}
	};

	const rowMenu = (user: UserInfo): MenuItem[] => [
		{
			id: 'admin',
			label: user.is_admin ? t('access.menu.remove_admin') : t('access.menu.make_admin'),
			icon: 'admin',
			onselect: () => toggleAdmin(user)
		},
		{
			id: 'replace',
			label: t('access.menu.replace_key'),
			icon: 'key',
			onselect: () => openRotate(user)
		},
		{
			id: 'transfer',
			label: t('access.menu.transfer'),
			icon: 'move',
			onselect: () => openTransfer(user.user_id)
		},
		{
			id: 'remove',
			label: t('access.menu.remove'),
			icon: 'trash',
			tone: 'danger',
			onselect: () => openRemove(user)
		}
	];

	const columns: ColumnDef<UserInfo>[] = [
		{ id: 'user', header: t('access.column.user'), cell: userCell, sortKey: 'user_id', width: 176, priority: 1 },
		{ id: 'admin', header: t('access.column.admin'), cell: adminCell, sortKey: 'is_admin', width: 112, priority: 1 },
		{ id: 'key', header: t('access.column.key'), cell: keyCell, sortKey: 'key_hint', width: 96 },
		{ id: 'files', header: t('access.column.files'), cell: filesCell, align: 'end', sortKey: 'owned_files', width: 120 },
		{ id: 'last', header: t('access.column.last_sign_in'), cell: lastCell, sortKey: 'last_auth_at', width: 136, priority: 1 },
		{
			id: 'activity',
			header: t('settings-screens.access.activity'),
			cell: activityCell,
			sortKey: 'requests_24h',
			width: 152,
			priority: 1,
			headerHelp: `${t('access.column.requests')}, ${t('access.column.failed')}`
		}
	];

	$effect(() => {
		if (false === keyOpen) {
			issued = null;
		}
	});
</script>

{#snippet userCell(user: UserInfo)}
	{#if POSITIONAL_SOURCE === user.source}
		{t('access.positional_row', { index: (user.index ?? 0) + 1, user: user.user_id })}
	{:else}
		<TextCell value={user.user_id} />
	{/if}
{/snippet}

{#snippet adminCell(user: UserInfo)}
	<BooleanCell value={user.is_admin} trueLabel={t('access.value.admin')} falseLabel={t('access.value.not_admin')} />
{/snippet}

{#snippet keyCell(user: UserInfo)}
	<TextCell value={user.key_hint} mono />
{/snippet}

{#snippet filesCell(user: UserInfo)}
	<NumberCell value={user.owned_files} />
{/snippet}

{#snippet lastCell(user: UserInfo)}
	{#if null === user.last_auth_at}
		{t('access.last_sign_in.none')}
	{:else}
		<TimeCell value={user.last_auth_at} />
	{/if}
{/snippet}

{#snippet activityCell(user: UserInfo)}
	<span class="activity">
		<span>{t('settings-screens.access.activity.value', { requests: user.requests_24h })}</span>
		{#if 0 < user.auth_failures_24h}
			<span class="failed">{t('settings-screens.access.activity.failed', { count: user.auth_failures_24h })}</span>
		{/if}
	</span>
{/snippet}

<PageFrame title={t('access.title')} job={t('access.job')}>
	{#snippet primary()}
		<Button variant="primary" iconStart="plus" onclick={openAdd}>{t('access.add.open')}</Button>
	{/snippet}
	{#snippet status()}
		<StaleIndicator
			updatedAt={usersQuery.updatedAt}
			intervalMs={STALE_INTERVAL_MS}
			state={staleState}
			onrefresh={() => usersQuery.refetch()}
		/>
	{/snippet}

	<Stack gap="6">
		{#if 0 < positionalCount}
			<InlineAlert tone="warn" title={t('access.positional.title')} body={t('access.positional.body', { count: positionalCount })}>
				{#snippet actions()}
					<Button variant="secondary" onclick={openMigration}>{t('access.positional.fix')}</Button>
				{/snippet}
			</InlineAlert>
		{/if}
		{#if 0 < unownedFiles}
			<InlineAlert tone="warn" title={t('access.unowned.title')} body={t('access.unowned.body', { count: unownedFiles })}>
				{#snippet actions()}
					<Button variant="secondary" onclick={() => openTransfer('')}>{t('access.menu.transfer')}</Button>
				{/snippet}
			</InlineAlert>
		{/if}
		{#if null !== pageError}
			<InlineAlert tone="danger" title={pageError.title} body={pageError.body} />
		{/if}
		{#if null !== transferJob}
			<InlineAlert tone="ok" title={t('access.transfer.started')}>
				{#snippet actions()}
					<Link href={`${base}/activity/jobs/${encodeURIComponent(null === transferJob ? '' : transferJob)}`}>{t('access.job_link')}</Link>
				{/snippet}
			</InlineAlert>
		{/if}
		{#if null !== migrationJob}
			<InlineAlert tone="ok" title={t('access.positional.started')}>
				{#snippet actions()}
					<Link href={`${base}/activity/jobs/${encodeURIComponent(null === migrationJob ? '' : migrationJob)}`}>{t('access.job_link')}</Link>
				{/snippet}
			</InlineAlert>
		{/if}
		<DataTable
			rows={managed}
			{columns}
			rowKey={(user) => user.user_id}
			label={t('access.table.label')}
			status={usersQuery.status}
			error={usersQuery.error ?? undefined}
			bind:sort
			rowMenu={rowMenu}
		>
			{#snippet empty()}
				<EmptyState variant="first-use" icon="key" title={t('access.empty.title')} body={t('access.empty.body')} />
			{/snippet}
		</DataTable>
		{#if 0 < systemRows.length}
			<SettingsCard title={t('access.system.title')} description={t('settings-screens.access.system.body')} flush>
				<ul class="system-list">
					{#each systemRows as user (user.user_id)}
						<SettingsItem label={user.user_id} mono>
							{#snippet trail()}
								<Badge tone="neutral">{t('settings-screens.access.system.badge')}</Badge>
							{/snippet}
						</SettingsItem>
					{/each}
				</ul>
			</SettingsCard>
		{/if}
	</Stack>
</PageFrame>

<Dialog bind:open={addOpen} title={t('access.add.title')} description={t('access.add.description')} size="md">
	<Stack gap="4">
		<FormField
			label={t('access.add.user_id.label')}
			for="access-add-id"
			help={t('access.add.user_id.help')}
			error={addIsReserved ? t('access.add.user_id.reserved') : undefined}
		>
			{#snippet control({ id, describedBy, invalid })}
				<Input {id} mono autocomplete="off" aria-describedby={describedBy} {invalid} bind:value={addId} />
			{/snippet}
		</FormField>
		<Switch
			checked={addAdmin}
			label={t('access.add.admin.label')}
			description={t('access.add.admin.description')}
			onchange={(next) => (addAdmin = next)}
		/>
		{#if null !== addMutation.error}
			<InlineAlert tone="danger" title={addMutation.error.title} body={addMutation.error.body} />
		{/if}
		<ApiEquivalent request={addRequest} />
	</Stack>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (addOpen = false)}>{t('common.cancel')}</Button>
			<Button variant="primary" disabledReason={addReason} loading={addMutation.pending} onclick={submitAdd}>
				{t('access.add.submit')}
			</Button>
		</Cluster>
	{/snippet}
</Dialog>

<Dialog
	bind:open={keyOpen}
	title={t('access.key.title', { user: issued?.user_id ?? '' })}
	description={t('access.key.description', { user: issued?.user_id ?? '' })}
	size="md"
>
	{#if null !== issued}
		<CopyField label={t('access.key.field', { user: issued.user_id })} value={issued.api_key} mono secretOnce />
	{/if}
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="primary" onclick={() => (keyOpen = false)}>{t('access.key.done')}</Button>
		</Cluster>
	{/snippet}
</Dialog>

<AlertDialog
	bind:open={rotateOpen}
	title={t('access.rotate.title', { user: rotateTarget?.user_id ?? '' })}
	body={t('access.rotate.body', { user: rotateTarget?.user_id ?? '' })}
	confirmLabel={t('access.rotate.confirm')}
	tone="warn"
	onconfirm={confirmRotate}
/>

<AlertDialog
	bind:open={removeOpen}
	title={t('access.remove.title', { user: removeTarget?.user_id ?? '' })}
	body={t('access.remove.body')}
	confirmLabel={t('access.remove.confirm')}
	tone="danger"
	onconfirm={confirmRemove}
/>

<Dialog
	bind:open={transferOpen}
	title={t('access.transfer.title', { from: '' === transferFrom ? t('access.owner.none') : transferFrom })}
	description={t('access.transfer.description')}
	size="md"
>
	<Stack gap="4">
		<FormField
			label={t('access.transfer.to.label')}
			for="access-transfer-to"
			help={t('access.transfer.to.help')}
		>
			{#snippet control({ id, describedBy, invalid })}
				<Input {id} mono autocomplete="off" aria-describedby={describedBy} {invalid} bind:value={transferTo} />
			{/snippet}
		</FormField>
		<FormField
			label={t('access.transfer.folder.label')}
			for="access-transfer-folder"
			help={t('access.transfer.folder.help')}
		>
			{#snippet control({ id, describedBy, invalid })}
				<Input {id} mono aria-describedby={describedBy} {invalid} bind:value={transferFolder} />
			{/snippet}
		</FormField>
		{#if null !== transferMutation.error}
			<InlineAlert tone="danger" title={transferMutation.error.title} body={transferMutation.error.body} />
		{/if}
		<ApiEquivalent request={transferRequest} />
	</Stack>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (transferOpen = false)}>{t('common.cancel')}</Button>
			<Button
				variant="primary"
				disabledReason={transferReason}
				loading={transferMutation.pending}
				onclick={submitTransfer}
			>
				{t('access.transfer.submit')}
			</Button>
		</Cluster>
	{/snippet}
</Dialog>

<PositionalMigration
	bind:open={migrateOpen}
	rows={positionalRows}
	{reservedIds}
	busy={migrateMutation.pending}
	error={migrateMutation.error}
	onsubmit={submitMigration}
/>

<style>
	.system-list {
		display: flex;
		flex-direction: column;
		margin: 0;
		padding: var(--space-1) 0;
		list-style: none;
	}

	.activity {
		display: inline-flex;
		flex-direction: column;
		gap: var(--space-0-5);
		font-variant-numeric: tabular-nums;
	}

	.failed {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--danger-text);
	}
</style>
