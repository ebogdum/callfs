import { goto } from '$app/navigation';
import { base } from '$app/paths';
import { batch, type BatchMode } from '$lib/api/batch';
import { toUiError, type ApiFailure } from '$lib/api/errors';
import { repair, verify } from '$lib/api/erasure';
import {
	createFolder,
	deletePermanent,
	planDelete,
	readRangeBlob,
	rename,
	setMode,
	setOwner,
	trash,
	type DeleteResult,
	type TrashReceipt
} from '$lib/api/files';
import { fileUrl, type ApiResponse } from '$lib/api/http';
import { generateLink } from '$lib/api/links';
import { users } from '$lib/api/admin/users';
import { permissionCheck, usage } from '$lib/api/search';
import { restore } from '$lib/api/trash';
import type { BatchOp, BatchResult, DeletePlan, FileEntry, PermissionDecision, Result, VerifyReport } from '$lib/api/types';
import { invalidate } from '$lib/data/cache';
import { createMutation, type Mutation, type UndoAction } from '$lib/data/mutation.svelte';
import { t } from '$lib/copy/t';
import { baseName, joinPath, parentPath } from '$lib/format/path';
import { formatBytes } from '$lib/format/bytes';
import { isValidMode } from '$lib/format/mode';
import { notifications } from '$lib/stores/notifications.svelte';
import { session } from '$lib/stores/session.svelte';
import { toasts } from '$lib/stores/toasts.svelte';
import { relativeDirectory, transfers, type ConflictPolicy, type ExistingFile } from '$lib/stores/transfers.svelte';
import type { MenuItem, Selection, UiError } from '$lib/ui/types';
import { filesClipboard } from '$lib/stores/clipboard.svelte';
import type { PermissionCheck } from '$lib/ui/organisms/PermissionInspector.svelte';
import type { ConflictRow } from '$lib/ui/organisms/ConflictResolver.svelte';
import type { MoveOptions } from '$lib/ui/organisms/DestinationPicker.svelte';
import type { ShareLinkResult } from '$lib/ui/organisms/ShareLinkForm.svelte';
import type { ComboboxItem } from '$lib/ui/molecules/Combobox.svelte';
import type { BrowseState } from './browse-state.svelte';

export const TYPED_DELETE_FILE_LIMIT = 100;
export const TYPED_DELETE_BYTES = 1024 ** 3;
export const UPLOAD_LIMIT_BYTES = 10 * 1024 ** 3;

const PERMISSION_KINDS: readonly PermissionCheck['perm'][] = ['read', 'write', 'delete'];
const FOLDER_BATCH_STATUS = 400;

export type DrawerKind = 'move' | 'share' | 'mode' | 'owner' | 'permissions' | 'delete' | 'range' | 'conflicts';

export interface DrawerRequest {
	kind: DrawerKind;
	entries: FileEntry[];
}

export interface ActionForms {
	move: { destination: string; options: MoveOptions };
	mode: string;
	owner: string[];
	seconds: number;
	range: { from: number; to: number | null };
	conflicts: { policy: ConflictPolicy; overrides: Record<string, ConflictPolicy> };
}

export interface FileActions {
	readonly drawer: DrawerRequest | null;
	readonly busy: boolean;
	readonly renaming: string | null;
	readonly permissionChecks: PermissionCheck[];
	readonly deletePlan: DeletePlan | null;
	readonly shareLink: ShareLinkResult | null;
	readonly conflicts: ConflictRow[];
	readonly pendingCount: number;
	readonly clipboardCount: number;
	readonly ownerItems: ComboboxItem[];
	readonly forms: ActionForms;
	readonly selected: FileEntry[];
	rowItems(entry: FileEntry): MenuItem[];
	bulkItems(): MenuItem[];
	toolbarItems(): MenuItem[];
	folderItems(): MenuItem[];
	cut(entries: FileEntry[]): void;
	paste(): Promise<void>;
	openDrawer(kind: DrawerKind, entries: FileEntry[]): void;
	closeDrawer(): void;
	startRename(entry: FileEntry): void;
	stopRename(): void;
	rename(entry: FileEntry, name: string): Promise<Result<unknown, UiError>>;
	newFolder(): Promise<void>;
	upload(files: File[]): void;
	confirmUpload(): void;
	trashSelected(): Promise<void>;
	submitMove(): Promise<void>;
	submitMode(): Promise<void>;
	submitOwner(): Promise<void>;
	submitDelete(): Promise<void>;
	submitShare(): Promise<void>;
	submitRange(): Promise<void>;
	exportList(): void;
	calculateSize(): Promise<void>;
}

export function requiresTypedDelete(plan: Pick<DeletePlan, 'files' | 'bytes'>): boolean {
	return TYPED_DELETE_FILE_LIMIT < plan.files || TYPED_DELETE_BYTES < plan.bytes;
}

export function sumPlans(plans: readonly DeletePlan[]): DeletePlan {
	return plans.reduce<DeletePlan>(
		(total, plan) => ({
			files: total.files + plan.files,
			directories: total.directories + plan.directories,
			bytes: total.bytes + plan.bytes,
			erasure_files: total.erasure_files + plan.erasure_files,
			remote_instances: [...new Set([...total.remote_instances, ...plan.remote_instances])]
		}),
		{ files: 0, directories: 0, bytes: 0, erasure_files: 0, remote_instances: [] }
	);
}

export function batchFailures(result: BatchResult): number {
	return (result.results ?? []).filter((item) => FOLDER_BATCH_STATUS <= item.status).length;
}

export function uniqueFolderName(base: string, names: readonly string[]): string {
	if (!names.includes(base)) {
		return base;
	}
	let counter = 2;
	while (names.includes(`${base} ${counter}`)) {
		counter += 1;
	}
	return `${base} ${counter}`;
}

export function isCrossStorage(options: MoveOptions): boolean {
	return [options.storage, options.server].some((value) => undefined !== value && '' !== value);
}

async function settle<T>(outcome: Promise<Result<ApiResponse<T>, ApiFailure>>): Promise<T> {
	const result = await outcome;
	return result.ok ? result.value.data : Promise.reject(result.error);
}

function toCheck(decision: PermissionDecision): PermissionCheck {
	return { perm: decision.perm, allowed: decision.allowed, reason: decision.reason, owner: decision.owner };
}

function saveBlob(name: string, blob: Blob): void {
	const url = URL.createObjectURL(blob);
	const anchor = document.createElement('a');
	anchor.href = url;
	anchor.download = name;
	anchor.click();
	setTimeout(() => URL.revokeObjectURL(url), 0);
}

function absoluteUrl(path: string): string {
	return new URL(fileUrl(path, { download: true }), window.location.origin).href;
}

function singleFilePlan(entry: FileEntry): DeletePlan {
	return { files: 1, directories: 0, bytes: entry.size, erasure_files: entry.erasure_coded ? 1 : 0, remote_instances: [] };
}

export function createFileActions(state: BrowseState): FileActions {
	let drawer = $state<DrawerRequest | null>(null);
	let busy = $state(false);
	let renaming = $state<string | null>(null);
	let permissionChecks = $state<PermissionCheck[]>([]);
	let deletePlan = $state<DeletePlan | null>(null);
	let shareLink = $state<ShareLinkResult | null>(null);
	let conflicts = $state<ConflictRow[]>([]);
	let pendingUploads = $state<File[]>([]);
	let ownerItems = $state<ComboboxItem[]>([]);
	let drawerToken = 0;
	const forms = $state<ActionForms>({
		move: { destination: '/', options: { overwrite: false, createParents: false } },
		mode: '644',
		owner: [],
		seconds: 3600,
		range: { from: 0, to: null },
		conflicts: { policy: 'skip', overrides: {} }
	});

	const selected = (): FileEntry[] => state.entries.filter((entry) => state.selection.ids.has(entry.path));

	const listingMap = (): Map<string, ExistingFile> =>
		new Map(state.entries.map((entry) => [entry.name, { size: entry.size, etag: entry.etag }]));

	function report(error: UiError): void {
		notifications.add({
			tone: 'danger',
			category: 'error',
			title: error.title,
			body: error.body,
			requestId: error.requestId ?? null
		});
		toasts.push({ tone: 'danger', title: error.title, body: error.body });
	}

	function confirmed(title: string): void {
		toasts.push({ tone: 'ok', title });
	}

	function offerUndo(undo: UndoAction): void {
		toasts.push({
			tone: 'info',
			title: undo.label,
			action: {
				label: t('common.undo'),
				run: () => {
					void runUndo(undo);
				}
			}
		});
	}

	async function runUndo(undo: UndoAction): Promise<void> {
		const outcome = await undo.run();
		if (!outcome.ok) {
			report(outcome.error);
		}
	}

	const deps = { registerUndo: offerUndo };

	async function perform<V, R>(mutation: Mutation<V, R>, vars: V, action: string): Promise<Result<R, UiError>> {
		const value = await mutation.run(vars);
		if (null !== value) {
			return { ok: true, value };
		}
		const error = mutation.error ?? toUiError({ kind: 'aborted' }, { action });
		report(error);
		return { ok: false, error };
	}

	function copyText(value: string): void {
		if (undefined === navigator.clipboard) {
			toasts.push({ tone: 'warn', title: t('fileactions.toast.copy_failed') });
			return;
		}
		navigator.clipboard.writeText(value).then(
			() => confirmed(t('fileactions.toast.copied')),
			() => toasts.push({ tone: 'warn', title: t('fileactions.toast.copy_failed') })
		);
	}

	const renameMutation = createMutation<{ entry: FileEntry; name: string }, unknown>(
		{
			run: ({ entry, name }) => settle(rename(entry.path, name)),
			optimistic: ({ entry, name }) => {
				const before = state.entries;
				state.entries = before.map((item) =>
					item.path === entry.path ? { ...item, name, path: joinPath(parentPath(entry.path), name) } : item
				);
				return () => {
					state.entries = before;
				};
			},
			invalidate: () => [['dir', state.path]],
			undo: ({ entry, name }) => ({
				label: t('fileactions.toast.renamed', { name }),
				run: () => settle(rename(joinPath(parentPath(entry.path), name), entry.name))
			}),
			context: { action: 'files.rename', object: ({ entry }) => entry.name }
		},
		deps
	);

	const folderMutation = createMutation<string, unknown>(
		{
			run: (path) => settle(createFolder(path)),
			invalidate: () => [['dir', state.path]],
			context: { action: 'files.create_folder' }
		},
		deps
	);

	const trashMutation = createMutation<FileEntry, TrashReceipt>(
		{
			run: (entry) => settle(trash(entry.path)),
			optimistic: (entry) => {
				const before = state.entries;
				state.entries = before.filter((item) => item.path !== entry.path);
				return () => {
					state.entries = before;
				};
			},
			invalidate: (entry) => [['dir', state.path], ['stat', entry.path]],
			context: { action: 'files.trash', object: (entry) => entry.name }
		},
		deps
	);

	const deleteMutation = createMutation<FileEntry, DeleteResult>(
		{
			run: (entry) => settle(deletePermanent(entry.path, { recursive: 'directory' === entry.type })),
			invalidate: (entry) => [['dir', state.path], ['stat', entry.path]],
			context: { action: 'files.delete', object: (entry) => entry.name }
		},
		deps
	);

	const batchMutation = createMutation<{ ops: BatchOp[]; mode: BatchMode }, BatchResult>(
		{
			run: ({ ops, mode }) => settle(batch(ops, mode)),
			invalidate: () => [['dir', state.path]],
			context: { action: 'files.batch' }
		},
		deps
	);

	const moveMutation = createMutation<{ ops: BatchOp[]; reverse: BatchOp[]; destination: string; count: number }, BatchResult>(
		{
			run: ({ ops }) => settle(batch(ops, 'sync')),
			invalidate: ({ destination }) => [['dir', state.path], ['dir', destination]],
			undo: ({ reverse, count, destination }) => ({
				label: t('fileactions.toast.moved', { count, destination }),
				run: () => settle(batch(reverse, 'sync'))
			}),
			context: { action: 'files.move' }
		},
		deps
	);

	const modeMutation = createMutation<{ entry: FileEntry; mode: string }, unknown>(
		{
			run: ({ entry, mode }) => settle(setMode(entry.path, mode)),
			optimistic: ({ entry, mode }) => {
				const before = state.entries;
				state.entries = before.map((item) => (item.path === entry.path ? { ...item, mode } : item));
				return () => {
					state.entries = before;
				};
			},
			invalidate: ({ entry }) => [['dir', state.path], ['stat', entry.path]],
			undo: ({ entry, mode }) => ({
				label: t('fileactions.toast.mode', { mode: entry.mode }),
				run: () => settle(setMode(entry.path, entry.mode))
			}),
			context: { action: 'files.mode', object: ({ entry }) => entry.name }
		},
		deps
	);

	const ownerMutation = createMutation<{ entry: FileEntry; owner: string }, unknown>(
		{
			run: ({ entry, owner }) => settle(setOwner(entry.path, owner)),
			invalidate: ({ entry }) => [['dir', state.path], ['stat', entry.path]],
			context: { action: 'files.owner', object: ({ entry }) => entry.name }
		},
		deps
	);

	function open(entry: FileEntry): void {
		if ('directory' === entry.type) {
			state.navigate(entry.path);
			return;
		}
		download([entry]);
	}

	function download(entries: FileEntry[]): void {
		for (const entry of entries) {
			if ('file' === entry.type) {
				transfers.download(entry.path, entry.name, entry.size);
			}
		}
	}

	async function planFor(entry: FileEntry): Promise<Result<DeletePlan, UiError>> {
		if ('file' === entry.type) {
			return { ok: true, value: singleFilePlan(entry) };
		}
		const outcome = await planDelete(entry.path);
		return outcome.ok
			? { ok: true, value: outcome.value.data }
			: { ok: false, error: toUiError(outcome.error, { action: 'files.delete' }) };
	}

	async function loadDeletePlan(entries: FileEntry[], token: number): Promise<void> {
		deletePlan = null;
		const plans: DeletePlan[] = [];
		const outcomes = await Promise.all(entries.map(planFor));
		if (token !== drawerToken) {
			return;
		}
		for (const outcome of outcomes) {
			if (!outcome.ok) {
				report(outcome.error);
				return;
			}
			plans.push(outcome.value);
		}
		deletePlan = sumPlans(plans);
	}

	async function loadPermissions(entry: FileEntry, token: number): Promise<void> {
		permissionChecks = [];
		const outcomes = await Promise.all(PERMISSION_KINDS.map((perm) => permissionCheck(entry.path, perm)));
		if (token !== drawerToken) {
			return;
		}
		permissionChecks = outcomes.flatMap((outcome) => (outcome.ok ? [toCheck(outcome.value.data)] : []));
	}

	async function loadOwners(): Promise<void> {
		const outcome = await users();
		if (!outcome.ok) {
			report(toUiError(outcome.error, { action: 'files.owner' }));
			return;
		}
		ownerItems = outcome.value.items.map((item) => ({ value: item.user_id, label: item.user_id }));
	}

	function openDrawer(kind: DrawerKind, entries: FileEntry[]): void {
		const [first] = entries;
		drawerToken += 1;
		const token = drawerToken;
		drawer = { kind, entries };
		if ('owner' === kind) {
			void loadOwners();
		}
		if ('permissions' === kind && undefined !== first) {
			void loadPermissions(first, token);
		}
		if ('delete' === kind) {
			void loadDeletePlan(entries, token);
		}
		if ('mode' === kind && undefined !== first) {
			forms.mode = first.mode;
		}
		if ('owner' === kind && undefined !== first) {
			forms.owner = [first.owner];
		}
		if ('move' === kind) {
			forms.move.destination = state.path;
		}
		if ('share' === kind) {
			shareLink = null;
		}
		if ('range' === kind) {
			forms.range = { from: 0, to: null };
		}
	}

	function closeDrawer(): void {
		drawerToken += 1;
		drawer = null;
		pendingUploads = [];
		conflicts = [];
		deletePlan = null;
		shareLink = null;
		permissionChecks = [];
	}

	function startRename(entry: FileEntry): void {
		renaming = entry.path;
	}

	function stopRename(): void {
		renaming = null;
	}

	async function rowRename(entry: FileEntry, name: string): Promise<Result<unknown, UiError>> {
		renaming = null;
		return perform(renameMutation, { entry, name }, 'files.rename');
	}

	async function newFolder(): Promise<void> {
		const name = uniqueFolderName(
			t('fileactions.new_folder_default'),
			state.entries.map((entry) => entry.name)
		);
		const outcome = await perform(folderMutation, joinPath(state.path, name), 'files.create_folder');
		if (outcome.ok) {
			confirmed(t('fileactions.toast.folder_created', { name }));
		}
	}

	function listingEntriesFor(files: File[]): ConflictRow[] {
		const existing = new Map(state.entries.map((entry) => [entry.name, entry]));
		return files.flatMap((file): ConflictRow[] => {
			if ('' !== relativeDirectory(file)) {
				return [];
			}
			const match = existing.get(file.name);
			if (undefined === match) {
				return [];
			}
			return [
				{
					path: joinPath(state.path, file.name),
					existing: { size: match.size, mtime: match.mtime },
					incoming: { size: file.size, mtime: new Date(file.lastModified).toISOString() }
				}
			];
		});
	}

	function enqueueUploads(files: File[], policyFor: (file: File) => ConflictPolicy | null): void {
		const groups = new Map<ConflictPolicy | null, File[]>();
		for (const file of files) {
			const policy = policyFor(file);
			groups.set(policy, [...(groups.get(policy) ?? []), file]);
		}
		let queued = 0;
		for (const [policy, group] of groups) {
			queued += transfers.enqueueUploads(group, state.path, listingMap(), policy).queued.length;
		}
		confirmed(t('fileactions.toast.uploads_queued', { count: queued }));
	}

	function upload(files: File[]): void {
		const accepted = files.filter((file) => UPLOAD_LIMIT_BYTES >= file.size);
		if (accepted.length < files.length) {
			toasts.push({ tone: 'warn', title: t('fileactions.toast.too_big') });
		}
		if (0 === accepted.length) {
			return;
		}
		const rows = listingEntriesFor(accepted);
		if (0 === rows.length) {
			enqueueUploads(accepted, () => null);
			return;
		}
		pendingUploads = accepted;
		conflicts = rows;
		forms.conflicts = { policy: 'skip', overrides: {} };
		drawer = { kind: 'conflicts', entries: [] };
	}

	function confirmUpload(): void {
		const names = new Set(conflicts.map((row) => row.path));
		const { policy, overrides } = forms.conflicts;
		enqueueUploads(pendingUploads, (file) => {
			const path = joinPath(state.path, file.name);
			return names.has(path) ? (overrides[path] ?? policy) : null;
		});
		closeDrawer();
	}

	async function trashSelected(): Promise<void> {
		const entries = selected();
		const [only] = entries;
		if (1 === entries.length && undefined !== only) {
			await trashOne(only);
			return;
		}
		if (0 === entries.length) {
			return;
		}
		const outcome = await perform(
			batchMutation,
			{ ops: entries.map((entry): BatchOp => ({ op: 'trash', path: entry.path })), mode: 'sync' },
			'files.trash'
		);
		if (outcome.ok) {
			state.selection = { mode: 'none', ids: new Set(), allMatching: false } satisfies Selection;
			reportBatch(outcome.value, entries.length, t('fileactions.toast.trashed_many', { count: entries.length }));
		}
	}

	function reportBatch(result: BatchResult, count: number, success: string | null): void {
		const failed = batchFailures(result);
		if (0 === failed) {
			if (null !== success) {
				confirmed(success);
			}
			return;
		}
		toasts.push({ tone: 'warn', title: t('fileactions.toast.partial', { failed, count }) });
	}

	async function submitMove(): Promise<void> {
		const entries = drawer?.entries ?? [];
		const destination = forms.move.destination;
		if (0 === entries.length || '' === destination) {
			return;
		}
		const { options } = forms.move;
		const ops = entries.map(
			(entry): BatchOp => ({
				op: 'move',
				path: entry.path,
				destination,
				overwrite: options.overwrite,
				create_parents: options.createParents,
				storage: options.storage,
				instance: options.server
			})
		);
		busy = true;
		const outcome = isCrossStorage(options)
			? await perform(batchMutation, { ops, mode: 'job' }, 'files.move')
			: await perform(
					moveMutation,
					{
						ops,
						reverse: entries.map((entry): BatchOp => ({
							op: 'move',
							path: joinPath(destination, entry.name),
							destination: parentPath(entry.path)
						})),
						destination,
						count: entries.length
					},
					'files.move'
				);
		busy = false;
		if (!outcome.ok) {
			return;
		}
		closeDrawer();
		if (isCrossStorage(options)) {
			confirmed(t('fileactions.toast.move_job'));
			return;
		}
		reportBatch(outcome.value, entries.length, null);
	}

	async function submitMode(): Promise<void> {
		const [entry] = drawer?.entries ?? [];
		if (undefined === entry) {
			return;
		}
		const mode = forms.mode;
		if (!isValidMode(mode)) {
			report(toUiError({ kind: 'malformed' }, { action: 'files.mode' }));
			return;
		}
		busy = true;
		const outcome = await perform(modeMutation, { entry, mode }, 'files.mode');
		busy = false;
		if (outcome.ok) {
			closeDrawer();
		}
	}

	async function submitOwner(): Promise<void> {
		const [entry] = drawer?.entries ?? [];
		const [owner] = forms.owner;
		if (undefined === entry || undefined === owner) {
			return;
		}
		busy = true;
		const outcome = await perform(ownerMutation, { entry, owner }, 'files.owner');
		busy = false;
		if (outcome.ok) {
			closeDrawer();
			confirmed(t('fileactions.toast.owner', { owner }));
		}
	}

	async function submitDelete(): Promise<void> {
		const entries = drawer?.entries ?? [];
		if (0 === entries.length) {
			return;
		}
		busy = true;
		const [only] = entries;
		if (1 === entries.length && undefined !== only) {
			const single = await perform(deleteMutation, only, 'files.delete');
			busy = false;
			if (!single.ok) {
				return;
			}
			closeDrawer();
			state.selection = { mode: 'none', ids: new Set(), allMatching: false } satisfies Selection;
			confirmed(t('fileactions.toast.deleted', { count: 1 }));
			return;
		}
		const ops = entries.map(
			(entry): BatchOp => ({ op: 'delete', path: entry.path, recursive: 'directory' === entry.type })
		);
		const outcome = await perform(batchMutation, { ops, mode: 'sync' }, 'files.delete');
		busy = false;
		if (!outcome.ok) {
			return;
		}
		closeDrawer();
		state.selection = { mode: 'none', ids: new Set(), allMatching: false } satisfies Selection;
		reportBatch(outcome.value, entries.length, t('fileactions.toast.deleted', { count: entries.length }));
	}

	async function submitShare(): Promise<void> {
		const [entry] = drawer?.entries ?? [];
		if (undefined === entry || 'file' !== entry.type) {
			return;
		}
		busy = true;
		const outcome = await generateLink(entry.path, forms.seconds);
		busy = false;
		if (!outcome.ok) {
			report(outcome.error);
			return;
		}
		shareLink = { url: outcome.value.url, expiresAt: outcome.value.expires };
		confirmed(t('fileactions.toast.link_created', { date: outcome.value.expires }));
	}

	async function submitRange(): Promise<void> {
		const [entry] = drawer?.entries ?? [];
		if (undefined === entry || 'file' !== entry.type) {
			return;
		}
		const from = forms.range.from;
		const to = forms.range.to ?? entry.size - 1;
		busy = true;
		const outcome = await readRangeBlob(entry.path, from, to);
		busy = false;
		if (!outcome.ok) {
			report(toUiError(outcome.error, { action: 'files.download' }));
			return;
		}
		saveBlob(entry.name, outcome.value);
		closeDrawer();
		confirmed(
			t('fileactions.toast.range_saved', {
				from: formatBytes(from).text,
				to: formatBytes(to).text,
				name: entry.name
			})
		);
	}

	async function verifyPieces(entry: FileEntry): Promise<void> {
		const outcome = await verify(entry.path);
		if (!outcome.ok) {
			report(outcome.error);
			return;
		}
		const verified: VerifyReport = outcome.value;
		if (verified.required <= verified.healthy) {
			confirmed(t('fileactions.toast.verify_ok', { name: entry.name }));
			return;
		}
		toasts.push({ tone: 'warn', title: t('fileactions.toast.verify_damaged', { name: entry.name }) });
	}

	async function repairPieces(entry: FileEntry): Promise<void> {
		const outcome = await repair(entry.path);
		if (!outcome.ok) {
			report(outcome.error);
			return;
		}
		confirmed(t('fileactions.toast.repair_started', { name: entry.name }));
	}

	async function calculateSize(): Promise<void> {
		const outcome = await usage(state.path);
		if (!outcome.ok) {
			report(toUiError(outcome.error, { action: 'files.usage' }));
			return;
		}
		confirmed(
			t('fileactions.toast.size', {
				name: baseName(state.path),
				size: formatBytes(outcome.value.data.bytes).text,
				files: outcome.value.data.files
			})
		);
	}

	function exportList(): void {
		const blob = new Blob([JSON.stringify(state.entries, null, 2)], { type: 'application/json' });
		saveBlob(`${baseName(state.path) || 'files'}.json`, blob);
		confirmed(t('fileactions.toast.export'));
	}

	function rowItems(entry: FileEntry): MenuItem[] {
		const isFile = 'file' === entry.type;
		const folderOnly = isFile ? undefined : t('fileactions.folder_only');
		const items: MenuItem[] = [
			{ id: 'open', label: t('fileactions.open'), onselect: () => open(entry) },
			{
				id: 'download',
				label: t('fileactions.download'),
				icon: 'download',
				shortcut: ['d'],
				disabledReason: folderOnly,
				onselect: () => download([entry])
			},
			{
				id: 'range',
				label: t('fileactions.download_range'),
				shortcut: ['shift', 'd'],
				disabledReason: folderOnly,
				onselect: () => openDrawer('range', [entry])
			},
			{
				id: 'link',
				label: t('fileactions.create_link'),
				icon: 'link',
				shortcut: ['l'],
				disabledReason: folderOnly,
				onselect: () => openDrawer('share', [entry])
			},
			{ id: 'copy-path', label: t('fileactions.copy_path'), icon: 'copy', shortcut: ['c'], onselect: () => copyText(entry.path) },
			{ id: 'copy-url', label: t('fileactions.copy_url'), onselect: () => copyText(absoluteUrl(entry.path)) },
			{ id: 'rename', label: t('fileactions.rename'), icon: 'rename', shortcut: ['f2'], onselect: () => startRename(entry) },
			{ id: 'move', label: t('fileactions.move'), icon: 'move', shortcut: ['m'], onselect: () => openDrawer('move', [entry]) },
			{ id: 'cut', label: t('fileactions.cut'), shortcut: ['mod', 'x'], onselect: () => cut(cutTargets(entry)) },
				{ id: 'mode', label: t('fileactions.change_mode'), onselect: () => openDrawer('mode', [entry]) }
		];
		if (session.isAdmin) {
			items.push({ id: 'owner', label: t('fileactions.change_owner'), onselect: () => openDrawer('owner', [entry]) });
		}
		items.push({
			id: 'permissions',
			label: t('fileactions.permissions'),
			icon: 'shield',
			onselect: () => openDrawer('permissions', [entry])
		});
		if (entry.erasure_coded) {
			items.push(
				{ id: 'verify', label: t('fileactions.verify'), onselect: () => void verifyPieces(entry) },
				{ id: 'repair', label: t('fileactions.repair'), onselect: () => void repairPieces(entry) }
			);
		}
		items.push(
			{ id: 'history', label: t('fileactions.history'), icon: 'history', onselect: () => void goto(`${base}/activity?path=${encodeURIComponent(entry.path)}`) },
			{ id: 'trash', label: t('fileactions.trash'), icon: 'trash', shortcut: ['delete'], onselect: () => void trashOne(entry) },
			{ id: 'delete', label: t('fileactions.delete_forever'), tone: 'danger', onselect: () => openDrawer('delete', [entry]) }
		);
		return items;
	}

	function cut(entries: FileEntry[]): void {
		filesClipboard.cut(entries.map((entry) => entry.path));
		if (0 < entries.length) {
			toasts.push({ tone: 'info', title: t('fileactions.toast.cut', { count: entries.length }) });
		}
	}

	async function paste(): Promise<void> {
		const clipboard = filesClipboard.value;
		if (null === clipboard) {
			return;
		}
		const destination = state.path;
		const paths = clipboard.paths.filter(
			(path) => parentPath(path) !== destination && destination !== path && !destination.startsWith(`${path}/`)
		);
		if (0 === paths.length) {
			filesClipboard.clear();
			toasts.push({ tone: 'warn', title: t('fileactions.toast.paste_nothing') });
			return;
		}
		const outcome = await perform(
			moveMutation,
			{
				ops: paths.map((path): BatchOp => ({ op: 'move', path, destination, overwrite: false, create_parents: false })),
				reverse: paths.map((path): BatchOp => ({
					op: 'move',
					path: joinPath(destination, baseName(path)),
					destination: parentPath(path)
				})),
				destination,
				count: paths.length
			},
			'files.move'
		);
		if (!outcome.ok) {
			return;
		}
		filesClipboard.clear();
		for (const source of new Set(paths.map(parentPath))) {
			invalidate(['dir', source]);
		}
		reportBatch(outcome.value, paths.length, null);
	}

	function cutTargets(entry: FileEntry): FileEntry[] {
		const chosen = selected();
		return 1 < chosen.length && chosen.some((item) => item.path === entry.path) ? chosen : [entry];
	}

	async function trashOne(entry: FileEntry): Promise<void> {
		const outcome = await perform(trashMutation, entry, 'files.trash');
		if (outcome.ok) {
			const receipt = outcome.value;
			offerUndo({
				label: t('fileactions.toast.trashed'),
				run: async () => {
					const restored = await restore(receipt.trash_id);
					invalidate(['dir', state.path]);
					return restored;
				}
			});
		}
	}

	function bulkItems(): MenuItem[] {
		const entries = selected();
		const count = entries.length;
		const files = entries.filter((entry) => 'file' === entry.type);
		return [
			{ id: 'bulk-move', label: t('fileactions.move_many', { count }), onselect: () => openDrawer('move', entries) },
			{
				id: 'bulk-download',
				label: t('fileactions.download_many', { count: files.length }),
				disabledReason: 0 === files.length ? t('fileactions.folder_only') : undefined,
				onselect: () => download(files)
			},
			{ id: 'bulk-trash', label: t('fileactions.trash_many', { count }), onselect: () => void trashSelected() },
			{ id: 'bulk-delete', label: t('fileactions.delete_forever'), tone: 'danger', onselect: () => openDrawer('delete', entries) }
		];
	}

	function toolbarItems(): MenuItem[] {
		return [
			{ id: 'calculate-size', label: t('fileactions.calculate_size'), onselect: () => void calculateSize() },
			{ id: 'export-list', label: t('fileactions.export_list'), onselect: () => exportList() },
			{ id: 'copy-folder-path', label: t('fileactions.copy_path'), onselect: () => copyText(state.path) }
		];
	}

	function folderItems(): MenuItem[] {
		return [
			{ id: 'new-folder', label: t('fileactions.new_folder'), icon: 'folder-plus', shortcut: ['n'], onselect: () => void newFolder() },
			{
				id: 'paste',
				label: t('fileactions.paste'),
				shortcut: ['mod', 'v'],
				disabledReason: null === filesClipboard.value ? t('fileactions.paste_empty') : undefined,
				onselect: () => void paste()
			},
			...toolbarItems()
		];
	}

	return {
		get drawer() {
			return drawer;
		},
		get busy() {
			return busy;
		},
		get renaming() {
			return renaming;
		},
		get permissionChecks() {
			return permissionChecks;
		},
		get deletePlan() {
			return deletePlan;
		},
		get shareLink() {
			return shareLink;
		},
		get conflicts() {
			return conflicts;
		},
		get pendingCount() {
			return pendingUploads.length;
		},
		get clipboardCount() {
			return filesClipboard.value?.paths.length ?? 0;
		},
		get ownerItems() {
			return ownerItems;
		},
		forms,
		get selected() {
			return selected();
		},
		rowItems,
		bulkItems,
		toolbarItems,
		folderItems,
		cut,
		paste,
		openDrawer,
		closeDrawer,
		startRename,
		stopRename,
		rename: rowRename,
		newFolder,
		upload,
		confirmUpload,
		trashSelected,
		submitMove,
		submitMode,
		submitOwner,
		submitDelete,
		submitShare,
		submitRange,
		exportList,
		calculateSize
	};
}
