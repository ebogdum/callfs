<script lang="ts" module>
	import type { RestoreOptions } from '$lib/api/trash';
	import type { ConflictPolicy } from '$lib/stores/transfers.svelte';

	const CONFLICT_OPTIONS: Partial<Record<ConflictPolicy, RestoreOptions['onConflict']>> = {
		'keep-both': 'keep_both',
		replace: 'replace'
	};
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import { stat } from '$lib/api/files';
	import { toUiError } from '$lib/api/errors';
	import type { Result, TrashItem } from '$lib/api/types';
	import { deleteTrashItem, emptyTrash, listTrash, restore } from '$lib/api/trash';
	import { t } from '$lib/copy/t';
	import { formatAbsolute } from '$lib/format/time';
	import { announce } from '$lib/ui/quarks/announce';
	import type { ConflictRow } from '$lib/ui/organisms/ConflictResolver.svelte';
	import type { ColumnDef, MenuItem, QueryStatus, Selection, UiError } from '$lib/ui/types';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import NameCell from '$lib/ui/cells/NameCell.svelte';
	import PathCell from '$lib/ui/cells/PathCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import BytesCell from '$lib/ui/cells/BytesCell.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import BulkBar from '$lib/ui/organisms/BulkBar.svelte';
	import Dock from '$lib/ui/environments/Dock.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import ConflictResolver from '$lib/ui/organisms/ConflictResolver.svelte';

	interface ConflictTarget {
		item: TrashItem;
		row: ConflictRow;
	}

	const rowKey = (row: TrashItem): string => row.id;

	const clearedSelection = (): Selection => ({ mode: 'multi', ids: new Set<string>(), allMatching: false });

	let status = $state<QueryStatus>('loading');
	let items = $state<TrashItem[]>([]);
	let nextCursor = $state<string | null>(null);
	let loadError = $state<UiError | null>(null);
	let actionError = $state<UiError | null>(null);
	let selection = $state<Selection>(clearedSelection());
	let busy = $state(false);
	let conflictOpen = $state(false);
	let conflictTargets = $state<ConflictTarget[]>([]);
	let policy = $state<ConflictPolicy>('skip');
	let overrides = $state<Record<string, ConflictPolicy>>({});
	let deleteOpen = $state(false);
	let deleteTargets = $state<TrashItem[]>([]);
	let emptyOpen = $state(false);

	const selectedItems = $derived(items.filter((item) => selection.ids.has(item.id)));
	const selectedCount = $derived(selectedItems.length);
	const selectedBytes = $derived(selectedItems.reduce((sum, item) => sum + item.size, 0));
	const conflictRows = $derived(conflictTargets.map((target) => target.row));
	const bulkActions = $derived<MenuItem[]>([
		{
			id: 'restore',
			label: t('transfers-trash.trash.action.restore_count', { count: selectedCount }),
			onselect: () => void restoreItems(selectedItems)
		},
		{
			id: 'delete',
			label: t('transfers-trash.trash.action.delete_count', { count: selectedCount }),
			tone: 'danger',
			onselect: () => openDelete(selectedItems)
		}
	]);

	const load = async (cursor: string | null): Promise<void> => {
		const result = await listTrash(cursor);
		if (!result.ok) {
			loadError = result.error;
			status = 'error';
			return;
		}
		items = null === cursor ? result.value.items : [...items, ...result.value.items];
		nextCursor = result.value.next_cursor;
		loadError = null;
		status = 0 === items.length ? 'empty' : 'success';
	};

	const describeConflicts = async (blocked: TrashItem[]): Promise<Result<ConflictTarget[], UiError>> => {
		const targets: ConflictTarget[] = [];
		for (const item of blocked) {
			const current = await stat(item.original_path);
			if (!current.ok) {
				return { ok: false, error: toUiError(current.error, { action: 'transfers-trash.trash.restore' }) };
			}
			const existing = current.value.data;
			targets.push({
				item,
				row: {
					path: item.original_path,
					existing: { size: existing.size, mtime: existing.mtime },
					incoming: { size: item.size, mtime: item.deleted_at }
				}
			});
		}
		return { ok: true, value: targets };
	};

	const restoreItems = async (targets: TrashItem[]): Promise<void> => {
		busy = true;
		actionError = null;
		const blocked: TrashItem[] = [];
		let restored = 0;
		for (const item of targets) {
			const result = await restore(item.id);
			if (result.ok) {
				restored += 1;
			} else if ('conflict' === result.error.layer) {
				blocked.push(item);
			} else {
				actionError = result.error;
				break;
			}
		}
		if (0 < restored) {
			announce(t('transfers-trash.trash.restored', { count: restored }), 'polite');
		}
		if (0 < blocked.length) {
			const described = await describeConflicts(blocked);
			if (described.ok) {
				conflictTargets = described.value;
				overrides = {};
				conflictOpen = true;
			} else {
				actionError = described.error;
			}
		}
		selection = clearedSelection();
		busy = false;
		await load(null);
	};

	const resolveConflicts = async (): Promise<void> => {
		busy = true;
		let restored = 0;
		for (const target of conflictTargets) {
			const onConflict = CONFLICT_OPTIONS[overrides[target.row.path] ?? policy];
			if (undefined === onConflict) {
				continue;
			}
			const result = await restore(target.item.id, { onConflict });
			if (result.ok) {
				restored += 1;
			} else {
				actionError = result.error;
				break;
			}
		}
		if (0 < restored) {
			announce(t('transfers-trash.trash.restored', { count: restored }), 'polite');
		}
		conflictOpen = false;
		conflictTargets = [];
		overrides = {};
		busy = false;
		await load(null);
	};

	const openDelete = (targets: TrashItem[]): void => {
		deleteTargets = targets;
		deleteOpen = true;
	};

	const deleteItems = async (targets: TrashItem[]): Promise<void> => {
		busy = true;
		actionError = null;
		let deleted = 0;
		for (const item of targets) {
			const result = await deleteTrashItem(item.id);
			if (result.ok) {
				deleted += 1;
			} else {
				actionError = result.error;
				break;
			}
		}
		if (0 < deleted) {
			announce(t('transfers-trash.trash.deleted', { count: deleted }), 'polite');
		}
		selection = clearedSelection();
		busy = false;
		await load(null);
	};

	const emptyAll = async (): Promise<void> => {
		busy = true;
		actionError = null;
		const result = await emptyTrash();
		busy = false;
		if (!result.ok) {
			actionError = result.error;
			return;
		}
		announce(t('transfers-trash.trash.emptying'), 'polite');
		await load(null);
	};

	const menuFor = (row: TrashItem): MenuItem[] => [
		{ id: 'restore', label: t('transfers-trash.trash.action.restore'), onselect: () => void restoreItems([row]) },
		{
			id: 'delete',
			label: t('transfers-trash.trash.action.delete'),
			tone: 'danger',
			onselect: () => openDelete([row])
		}
	];

	onMount(() => {
		void load(null);
	});

	const columns: ColumnDef<TrashItem>[] = [
		{ id: 'name', header: t('transfers-trash.trash.col.name'), cell: nameCell, minWidth: 200, priority: 1 },
		{ id: 'original_path', header: t('transfers-trash.trash.col.original_path'), cell: originalPathCell, minWidth: 200, priority: 1 },
		{ id: 'deleted_by', header: t('transfers-trash.trash.col.deleted_by'), cell: deletedByCell, minWidth: 120 },
		{ id: 'deleted', header: t('transfers-trash.trash.col.deleted'), cell: deletedCell },
		{ id: 'expires', header: t('transfers-trash.trash.col.expires'), cell: expiresCell, minWidth: 160, priority: 1 },
		{ id: 'size', header: t('transfers-trash.trash.col.size'), cell: sizeCell, align: 'end' }
	];
</script>

{#snippet nameCell(row: TrashItem)}
	<NameCell name={row.name} kind={'directory' === row.type ? 'folder' : 'file'} />
{/snippet}

{#snippet originalPathCell(row: TrashItem)}
	<PathCell path={row.original_path} />
{/snippet}

{#snippet deletedByCell(row: TrashItem)}
	<TextCell value={row.deleted_by} />
{/snippet}

{#snippet deletedCell(row: TrashItem)}
	<TimeCell value={row.deleted_at} />
{/snippet}

{#snippet expiresCell(row: TrashItem)}
	<TextCell value={formatAbsolute(row.expires_at)} />
{/snippet}

{#snippet sizeCell(row: TrashItem)}
	<BytesCell value={row.size} />
{/snippet}

{#snippet emptyState()}
	<EmptyState
		variant="cleared"
		icon="trash"
		title={t('transfers-trash.trash.empty.title')}
		body={t('transfers-trash.trash.empty.body')}
	/>
{/snippet}

{#snippet emptySecondary()}
	<Button variant="secondary" iconStart="trash" onclick={() => (emptyOpen = true)}>
		{t('transfers-trash.trash.action.empty')}
	</Button>
{/snippet}

<PageFrame
	title={t('transfers-trash.trash.title')}
	job={t('transfers-trash.trash.job')}
	secondary={0 < items.length ? emptySecondary : undefined}
>
	<InlineAlert tone="info" title={t('transfers-trash.trash.notice')} />
	{#if null !== actionError}
		<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
	{/if}
	<DataTable
		rows={items}
		{columns}
		{rowKey}
		rowLabel={(row) => row.name}
		label={t('transfers-trash.trash.table')}
		{status}
		error={loadError ?? undefined}
		onretry={() => void load(null)}
		bind:selection
		hasMore={null !== nextCursor}
		onloadmore={() => void load(nextCursor)}
		rowMenu={menuFor}
		rowInline={(row) => menuFor(row).slice(0, 2)}
		empty={emptyState}
	/>
	{#if 0 < selectedCount}
		<Dock>
			<BulkBar
				count={selectedCount}
				bytes={selectedBytes}
				allMatching={false}
				total={items.length}
				actions={bulkActions}
				onselectall={() => (selection = { mode: 'multi', ids: new Set(items.map((item) => item.id)), allMatching: false })}
				onclear={() => (selection = clearedSelection())}
			/>
		</Dock>
	{/if}
</PageFrame>

<Dialog
	bind:open={conflictOpen}
	title={t('transfers-trash.trash.conflict.title')}
	description={t('transfers-trash.trash.conflict.body')}
	size="lg"
>
	<ConflictResolver conflicts={conflictRows} bind:policy bind:overrides />
	{#snippet footer()}
		<Button variant="secondary" onclick={() => (conflictOpen = false)}>{t('transfers-trash.trash.cancel')}</Button>
		<Button variant="primary" loading={busy} onclick={() => void resolveConflicts()}>
			{t('transfers-trash.trash.conflict.confirm', { count: conflictTargets.length })}
		</Button>
	{/snippet}
</Dialog>

<AlertDialog
	bind:open={deleteOpen}
	title={t('transfers-trash.trash.delete.title', { count: deleteTargets.length })}
	body={t('transfers-trash.trash.delete.body')}
	confirmLabel={t('transfers-trash.trash.delete.confirm', { count: deleteTargets.length })}
	tone="danger"
	onconfirm={() => void deleteItems(deleteTargets)}
/>

<AlertDialog
	bind:open={emptyOpen}
	title={t('transfers-trash.trash.empty.confirm_title')}
	body={t('transfers-trash.trash.empty.confirm_body')}
	confirmLabel={t('transfers-trash.trash.empty.confirm')}
	tone="danger"
	onconfirm={() => void emptyAll()}
/>
