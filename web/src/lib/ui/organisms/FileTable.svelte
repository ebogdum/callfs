<script lang="ts" module>
	export const TOKENS = ['--file-drop-target-bg'] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { FileEntry } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { DASH } from '$lib/format/number';
	import { isHidden } from '$lib/format/path';
	import type { NameBadge } from '../cells/NameCell.svelte';
	import BooleanCell from '../cells/BooleanCell.svelte';
	import BytesCell from '../cells/BytesCell.svelte';
	import ETagCell from '../cells/ETagCell.svelte';
	import ModeCell from '../cells/ModeCell.svelte';
	import NameCell from '../cells/NameCell.svelte';
	import OwnerCell from '../cells/OwnerCell.svelte';
	import ServerCell from '../cells/ServerCell.svelte';
	import StatusCell from '../cells/StatusCell.svelte';
	import StorageCell from '../cells/StorageCell.svelte';
	import TextCell from '../cells/TextCell.svelte';
	import TimeCell from '../cells/TimeCell.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import type {
		ColumnDef,
		Density,
		MenuItem,
		QueryStatus,
		Selection,
		SortState,
		StatusKey,
		UiError
	} from '../types';
	import DataTable from './table/DataTable.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children' | 'ondrop'> {
		entries: FileEntry[];
		status: QueryStatus;
		error?: UiError;
		selection?: Selection;
		sort?: SortState | null;
		renaming?: string | null;
		density?: Density;
		virtual?: 'auto' | boolean;
		total?: number | null;
		totalIsEstimate?: boolean;
		hasMore?: boolean;
		onloadmore?: () => void;
		onopen?: (entry: FileEntry) => void;
		onrename?: (entry: FileEntry, name: string) => void;
		rowMenu?: (entry: FileEntry) => MenuItem[];
		ondrop?: (entry: FileEntry, event: DragEvent) => void;
		hiddenVisible: boolean;
		localInstance?: string;
		backupStatus?: (entry: FileEntry) => StatusKey | null;
		toolbar?: Snippet;
		class?: string;
	}

	let {
		entries,
		status,
		error,
		selection = $bindable({ mode: 'multi', ids: new Set<string>(), allMatching: false }),
		sort = $bindable(null),
		renaming = $bindable(null),
		density,
		virtual,
		total,
		totalIsEstimate,
		hasMore,
		onloadmore,
		onopen,
		onrename,
		rowMenu,
		ondrop,
		hiddenVisible,
		localInstance = '',
		backupStatus = () => null,
		toolbar,
		class: className,
		...rest
	}: Props = $props();

	const isFolder = (entry: FileEntry): boolean => 'directory' === entry.type;

	const visibleEntries = $derived(
		entries
			.filter((entry) => hiddenVisible || !isHidden(entry.path))
			.sort((first, second) => Number(isFolder(second)) - Number(isFolder(first)))
	);

	const rowKey = (entry: FileEntry): string => entry.path;

	const hrefFor = (entry: FileEntry): string =>
		`/files${entry.path
			.split('/')
			.map((segment) => encodeURIComponent(segment))
			.join('/')}`;

	const badgesFor = (entry: FileEntry): NameBadge[] => {
		const badges: NameBadge[] = [];
		if (entry.locked) {
			badges.push('locked');
		}
		if (entry.erasure_coded) {
			badges.push('split');
		}
		if ('' !== localInstance && entry.instance_id !== localInstance) {
			badges.push('remote');
		}
		if (isHidden(entry.path)) {
			badges.push('hidden');
		}
		return badges;
	};

	const finishRename = (entry: FileEntry, name: string): void => {
		renaming = null;
		onrename?.(entry, name);
	};

	const cancelRename = (): void => {
		renaming = null;
	};

	let dropTarget = $state<string | null>(null);

	const allowDrop = (entry: FileEntry, event: DragEvent): void => {
		if (isFolder(entry) && undefined !== ondrop) {
			event.preventDefault();
			dropTarget = entry.path;
		}
	};

	const leaveDrop = (entry: FileEntry): void => {
		if (entry.path === dropTarget) {
			dropTarget = null;
		}
	};

	const dropOn = (entry: FileEntry, event: DragEvent): void => {
		dropTarget = null;
		if (isFolder(entry)) {
			ondrop?.(entry, event);
		}
	};

	const nameColumn: ColumnDef<FileEntry> = {
		id: 'name',
		header: t('files.column.name'),
		cell: nameCell,
		sortKey: 'name',
		priority: 1
	};
	const sizeColumn: ColumnDef<FileEntry> = {
		id: 'size',
		header: t('files.column.size'),
		cell: sizeCell,
		sortKey: 'size',
		align: 'end',
		priority: 2
	};
	const modifiedColumn: ColumnDef<FileEntry> = {
		id: 'modified',
		header: t('files.column.modified'),
		cell: modifiedCell,
		sortKey: 'mtime',
		priority: 2
	};
	const ownerColumn: ColumnDef<FileEntry> = {
		id: 'owner',
		header: t('files.column.owner'),
		cell: ownerCell,
		sortKey: 'owner',
		priority: 3
	};
	const storageColumn: ColumnDef<FileEntry> = {
		id: 'storage',
		header: t('files.column.storage'),
		cell: storageCell,
		sortKey: 'storage',
		priority: 3
	};
	const serverColumn: ColumnDef<FileEntry> = {
		id: 'server',
		header: t('files.column.server'),
		cell: serverCell,
		priority: 3,
		hidden: true
	};
	const versionColumn: ColumnDef<FileEntry> = {
		id: 'version',
		header: t('files.column.version'),
		cell: versionCell,
		priority: 3,
		hidden: true
	};
	const modeColumn: ColumnDef<FileEntry> = {
		id: 'mode',
		header: t('files.column.mode'),
		cell: modeCell,
		priority: 3,
		hidden: true
	};
	const splitColumn: ColumnDef<FileEntry> = {
		id: 'split',
		header: t('files.column.split'),
		cell: splitCell,
		priority: 3,
		hidden: true
	};
	const backupColumn: ColumnDef<FileEntry> = {
		id: 'backup',
		header: t('files.column.backup'),
		cell: backupCell,
		priority: 3,
		hidden: true
	};
	const lockedColumn: ColumnDef<FileEntry> = {
		id: 'locked',
		header: t('files.column.locked'),
		cell: lockedCell,
		priority: 2,
		hidden: true
	};

	const columns = $derived<ColumnDef<FileEntry>[]>([
		nameColumn,
		sizeColumn,
		modifiedColumn,
		ownerColumn,
		storageColumn,
		serverColumn,
		versionColumn,
		modeColumn,
		splitColumn,
		backupColumn,
		lockedColumn
	]);
</script>

{#snippet nameCell(entry: FileEntry)}
	<span
		class="name-slot"
		data-entry-path={entry.path}
		data-drop-target={isFolder(entry) ? String(entry.path === dropTarget) : undefined}
		ondragover={(event: DragEvent) => allowDrop(entry, event)}
		ondragleave={() => leaveDrop(entry)}
		ondrop={(event: DragEvent) => dropOn(entry, event)}
	>
		<NameCell
			name={entry.name}
			kind={isFolder(entry) ? 'folder' : 'file'}
			href={hrefFor(entry)}
			badges={badgesFor(entry)}
			editing={renaming === entry.path}
			onrename={(name: string) => finishRename(entry, name)}
			oncancel={cancelRename}
		/>
	</span>
{/snippet}

{#snippet sizeCell(entry: FileEntry)}
	<BytesCell value={isFolder(entry) ? null : entry.size} />
{/snippet}

{#snippet modifiedCell(entry: FileEntry)}
	<TimeCell value={entry.mtime} />
{/snippet}

{#snippet ownerCell(entry: FileEntry)}
	<OwnerCell value={entry.owner} />
{/snippet}

{#snippet storageCell(entry: FileEntry)}
	<StorageCell value={entry.storage} />
{/snippet}

{#snippet serverCell(entry: FileEntry)}
	<ServerCell id={entry.instance_id} self={'' !== localInstance && entry.instance_id === localInstance} />
{/snippet}

{#snippet versionCell(entry: FileEntry)}
	<ETagCell value={entry.etag} />
{/snippet}

{#snippet modeCell(entry: FileEntry)}
	<ModeCell value={entry.mode} />
{/snippet}

{#snippet splitCell(entry: FileEntry)}
	<BooleanCell value={entry.erasure_coded} trueLabel={t('files.split.yes')} falseLabel={t('files.split.no')} />
{/snippet}

{#snippet backupCell(entry: FileEntry)}
	{@const backup = backupStatus(entry)}
	{#if null === backup}
		<TextCell value={DASH} muted />
	{:else}
		<StatusCell status={backup} />
	{/if}
{/snippet}

{#snippet lockedCell(entry: FileEntry)}
	<BooleanCell value={entry.locked} trueLabel={t('files.locked.yes')} falseLabel={t('files.locked.no')} />
{/snippet}

{#snippet empty()}
	<EmptyState variant="first-use" icon="folder" title={t('files.table.empty_title')} body={t('files.table.empty_body')} />
{/snippet}

<div class={['file-table', className]} {...rest}>
	<DataTable
		rows={visibleEntries}
		{columns}
		{rowKey}
		rowLabel={(entry: FileEntry) => entry.name}
		label={t('files.table.label')}
		{status}
		{error}
		bind:sort
		bind:selection
		{density}
		{virtual}
		{total}
		{totalIsEstimate}
		{hasMore}
		{onloadmore}
		{onopen}
		{rowMenu}
		{empty}
		{toolbar}
		stickyHeader
		freezeFirst
		resizable
	/>
</div>

<style>
	.file-table {
		display: grid;
		min-inline-size: 0;
	}

	.name-slot {
		display: flex;
		min-inline-size: 0;
	}

	.name-slot[data-drop-target='true'] {
		background: var(--file-drop-target-bg, var(--accent-soft));
	}
</style>
