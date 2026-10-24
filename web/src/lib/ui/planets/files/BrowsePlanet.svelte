<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { getContext, untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { locks } from '$lib/api/admin/runtime';
	import { audit, type AuditPage } from '$lib/api/admin/telemetry';
	import { instances, type InstanceItem } from '$lib/api/admin/topology';
	import { getSplitFile, repair, verify } from '$lib/api/erasure';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { listDir, readRange, stat } from '$lib/api/files';
	import { fileUrl, type ApiResponse } from '$lib/api/http';
	import { listLinks } from '$lib/api/links';
	import { permissionCheck } from '$lib/api/search';
	import type { DirPage, FileEntry, Link, LinkPage, Locks, Result, SplitFileDetail, Stat } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { kindFromName } from '$lib/format/mime';
	import { parentPath, segments } from '$lib/format/path';
	import { notifications } from '$lib/stores/notifications.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { QueryStatus, Tone, UiError } from '$lib/ui/types';
	import Button from '../../atoms/Button.svelte';
	import RelTime from '../../atoms/RelTime.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Text from '../../atoms/Text.svelte';
	import Breadcrumbs from '../../molecules/Breadcrumbs.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import SplitView from '../../environments/SplitView.svelte';
	import Dock from '../../environments/Dock.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import BulkBar from '../../organisms/BulkBar.svelte';
	import Dialog from '../../organisms/Dialog.svelte';
	import FileGrid from '../../organisms/FileGrid.svelte';
	import FileTable from '../../organisms/FileTable.svelte';
	import FileTree from '../../organisms/FileTree.svelte';
	import Inspector, { type InspectorTab } from '../../organisms/Inspector.svelte';
	import PermissionInspector, { type PermissionCheck, type PermissionKind } from '../../organisms/PermissionInspector.svelte';
	import { clearSelection, selectPage } from '../../organisms/table/table-state.svelte';
	import PreviewPane from '../../organisms/viewer/PreviewPane.svelte';
	import type { RangeLoader } from '../../organisms/viewer/TextViewer.svelte';
	import { BrowseState, ROOT_PATH, hrefFor, pathFromRoute, type BrowseDefaults, type BrowseSort } from './browse-state.svelte';
	import BrowseToolbar from './BrowseToolbar.svelte';
	import { createFileActions } from './file-actions.svelte';
	import FileActionDrawers from './FileActionDrawers.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		class?: string;
	}

	interface ActivityLine {
		at: string;
		actor?: string;
		title: string;
		tone: Tone;
	}

	let { class: className }: Props = $props();

	const PAGE_SIZE = 200;
	const ACTIVITY_LIMIT = 50;
	const COMMANDS_CONTEXT = 'callfs.commands';
	const PERMISSION_KINDS: readonly PermissionKind[] = ['read', 'write', 'delete'];
	const LINK_STATUS_KEYS: Record<Link['status'], string> = {
		active: 'browse.link.active',
		used: 'browse.link.used',
		expired: 'browse.link.expired',
		revoked: 'browse.link.revoked'
	};
	const ACTIVITY_TITLES: Partial<Record<string, string>> = {
		upload: 'browse.activity.upload',
		replace: 'browse.activity.replace',
		rename: 'browse.activity.rename',
		move: 'browse.activity.move',
		delete: 'browse.activity.delete',
		trash: 'browse.activity.trash',
		restore: 'browse.activity.restore',
		read: 'browse.activity.read',
		list: 'browse.activity.list',
		link_create: 'browse.activity.link_create',
		link_open: 'browse.activity.link_open'
	};

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	const browseState = new BrowseState((href) => void goto(`${base}${href}`, { keepFocus: true, noScroll: true }));
	const actions = createFileActions(browseState);

	let treeOpen = $state(true);
	let detailWidth = $state(400);
	let loadingMore = $state(false);
	let durabilityBusy = $state(false);
	let uploadInput = $state<HTMLInputElement | null>(null);

	function defaults(): BrowseDefaults {
		const files = preferences.value.files;
		return { view: files.view, hidden: files.hidden, sort: { key: files.sort.key, dir: files.sort.dir } };
	}

	function isEditable(target: EventTarget | null): boolean {
		return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName));
	}

	function fromResponse<T>(result: Result<ApiResponse<T>, ApiFailure>, action: string, object: string): Result<T, UiError> {
		return result.ok ? { ok: true, value: result.value.data } : { ok: false, error: toUiError(result.error, { action, object }) };
	}

	function fromResult<T>(result: Result<T, ApiFailure>, action: string, object: string): Result<T, UiError> {
		return result.ok ? { ok: true, value: result.value } : { ok: false, error: toUiError(result.error, { action, object }) };
	}

	async function fetchListing(path: string, sort: BrowseSort, includeHidden: boolean): Promise<Result<DirPage, UiError>> {
		return fromResponse(
			await listDir(path, { limit: PAGE_SIZE, sort: sort.key, order: sort.dir, includeHidden }),
			'browse.action.open_folder',
			path
		);
	}

	const listing = createQuery<Result<DirPage, UiError>>(() => ({
		key: ['dir', browseState.path, browseState.sort.key, browseState.sort.dir, browseState.showHidden],
		fetch: () => fetchListing(browseState.path, browseState.sort, browseState.showHidden),
		revalidateOnFocus: true,
		isEmpty: (result) => result.ok && 0 === result.value.items.length,
		isPartial: (result) => result.ok && result.value.partial
	}));

	const current = $derived(listing.data);
	const listingPage = $derived(current?.ok ? current.value : null);
	const failure = $derived(undefined !== current && !current.ok ? current.error : null);
	const pageState = $derived<QueryStatus>(
		undefined === current
			? 'loading'
			: null !== failure
				? 'error'
				: 0 === listingPage?.items.length
					? 'empty'
					: listingPage?.partial
						? 'partial'
						: 'success'
	);
	const listStatus = $derived<QueryStatus>('loading' === pageState ? 'loading' : 'partial' === pageState ? 'partial' : 'success');

	$effect(() => {
		if (undefined === current) {
			browseState.entries = [];
		} else if (current.ok) {
			browseState.entries = current.value.items;
		}
	});

	const needle = $derived(browseState.q.trim().toLowerCase());
	const visible = $derived(0 === needle.length ? browseState.entries : browseState.entries.filter((entry) => entry.name.toLowerCase().includes(needle)));
	const nextCursor = $derived(listingPage?.next_cursor ?? null);
	const hasMore = $derived(null !== nextCursor);
	const selectedBytes = $derived(actions.selected.reduce((sum, entry) => sum + entry.size, 0));
	const folderTitle = $derived(ROOT_PATH === browseState.path ? t('browse.root') : (segments(browseState.path).at(-1)?.label ?? t('browse.root')));
	const crumbs = $derived([
		{ label: t('browse.root'), href: `${base}/files` },
		...segments(browseState.path).map((segment) => ({ label: segment.label, href: `${base}${hrefFor(segment.path)}` }))
	]);

	async function loadMore(): Promise<void> {
		const cursor = nextCursor;
		if (null === cursor || loadingMore) {
			return;
		}
		loadingMore = true;
		const result = fromResponse(
			await listDir(browseState.path, {
				limit: PAGE_SIZE,
				cursor,
				sort: browseState.sort.key,
				order: browseState.sort.dir,
				includeHidden: browseState.showHidden
			}),
			'browse.action.load_more',
			browseState.path
		);
		loadingMore = false;
		if (!result.ok) {
			notifications.add({
				tone: 'danger',
				category: 'error',
				title: result.error.title,
				body: result.error.body,
				requestId: result.error.requestId ?? null
			});
			return;
		}
		const next = result.value;
		listing.setData((data) =>
			undefined !== data && data.ok
				? {
						ok: true,
						value: {
							...data.value,
							items: [...data.value.items, ...next.items],
							next_cursor: next.next_cursor,
							partial: next.partial,
							total: next.total
						}
					}
				: data
		);
	}

	async function loadFolders(path: string): Promise<Result<FileEntry[], UiError>> {
		const result = fromResponse(await listDir(path, { limit: PAGE_SIZE, type: 'directory' }), 'browse.action.list_folders', path);
		return result.ok ? { ok: true, value: result.value.items } : { ok: false, error: result.error };
	}

	async function fetchStat(path: string): Promise<Result<Stat, UiError>> {
		return fromResponse(await stat(path), 'browse.action.open_file', path);
	}

	const statQuery = createQuery<Result<Stat, UiError>>(() => ({
		key: ['stat', browseState.inspect],
		fetch: () => fetchStat(browseState.inspect ?? ''),
		enabled: () => null !== browseState.inspect
	}));
	const entity = $derived(undefined !== statQuery.data && statQuery.data.ok ? statQuery.data.value : null);

	const permissionsWanted = $derived(('permissions' === browseState.tab && null !== browseState.inspect) || 'permission' === failure?.layer);
	const permissionPath = $derived(permissionsWanted ? (browseState.inspect ?? browseState.path) : null);

	async function fetchPermissions(path: string): Promise<Result<PermissionCheck[], UiError>> {
		const results = await Promise.all(PERMISSION_KINDS.map((perm) => permissionCheck(path, perm)));
		const checks: PermissionCheck[] = [];
		for (const result of results) {
			const decision = fromResponse(result, 'browse.action.check_permissions', path);
			if (!decision.ok) {
				return { ok: false, error: decision.error };
			}
			checks.push({
				perm: decision.value.perm,
				allowed: decision.value.allowed,
				reason: decision.value.reason,
				owner: decision.value.owner
			});
		}
		return { ok: true, value: checks };
	}

	const permissionsQuery = createQuery<Result<PermissionCheck[], UiError>>(() => ({
		key: ['permissions', permissionPath],
		fetch: () => fetchPermissions(permissionPath ?? ''),
		enabled: () => null !== permissionPath
	}));
	const permissionChecks = $derived(undefined !== permissionsQuery.data && permissionsQuery.data.ok ? permissionsQuery.data.value : []);

	const linksQuery = createQuery<Result<LinkPage, UiError>>(() => ({
		key: ['links', browseState.inspect],
		fetch: () => listLinks({ path: browseState.inspect ?? '' }, null),
		enabled: () => null !== browseState.inspect && 'links' === browseState.tab
	}));
	const linkItems = $derived(linksQuery.data?.ok ? linksQuery.data.value.items : []);

	async function fetchSplit(path: string): Promise<Result<SplitFileDetail, UiError>> {
		return getSplitFile(path);
	}

	const erasureQuery = createQuery<Result<SplitFileDetail, UiError>>(() => ({
		key: ['split', browseState.inspect],
		fetch: () => fetchSplit(browseState.inspect ?? ''),
		enabled: () => null !== browseState.inspect && 'durability' === browseState.tab && true === entity?.erasure_coded
	}));
	const erasure = $derived(undefined !== erasureQuery.data && erasureQuery.data.ok ? erasureQuery.data.value : null);

	async function runDurability(run: (path: string) => Promise<Result<unknown, UiError>>): Promise<void> {
		const path = browseState.inspect;
		if (null === path || durabilityBusy) {
			return;
		}
		durabilityBusy = true;
		const result = await run(path);
		durabilityBusy = false;
		if (result.ok) {
			void erasureQuery.refetch();
			return;
		}
		notifications.add({
			tone: 'danger',
			category: 'error',
			title: result.error.title,
			body: result.error.body,
			requestId: result.error.requestId ?? null
		});
	}

	async function fetchActivity(path: string): Promise<Result<AuditPage, UiError>> {
		return fromResult(await audit({ path, limit: ACTIVITY_LIMIT }), 'browse.action.read_activity', path);
	}

	const activityQuery = createQuery<Result<AuditPage, UiError>>(() => ({
		key: ['audit', browseState.inspect],
		fetch: () => fetchActivity(browseState.inspect ?? ''),
		enabled: () => session.isAdmin && null !== browseState.inspect && 'activity' === browseState.tab
	}));
	const activityItems = $derived.by((): ActivityLine[] => {
		if (undefined === activityQuery.data || !activityQuery.data.ok) {
			return [];
		}
		return activityQuery.data.value.items.map((entry) => ({
			at: entry.at,
			actor: entry.user_id ?? undefined,
			title: t(ACTIVITY_TITLES[entry.op] ?? 'browse.activity.other'),
			tone: 400 <= entry.status ? 'danger' : 'neutral'
		}));
	});
	const activityStatus = $derived<QueryStatus>(session.isAdmin ? activityQuery.status : 'success');

	async function fetchInstances(): Promise<Result<InstanceItem[], UiError>> {
		const result = fromResult(await instances(), 'browse.action.read_placement', '');
		return result.ok ? { ok: true, value: result.value.items } : { ok: false, error: result.error };
	}

	const instancesQuery = createQuery<Result<InstanceItem[], UiError>>(() => ({
		key: ['instances'],
		fetch: fetchInstances,
		enabled: () => session.isAdmin && null !== browseState.inspect && 'placement' === browseState.tab
	}));

	async function fetchLocks(path: string): Promise<Result<Locks, UiError>> {
		return fromResult(await locks(path), 'browse.action.read_placement', path);
	}

	const locksQuery = createQuery<Result<Locks, UiError>>(() => ({
		key: ['locks', browseState.inspect],
		fetch: () => fetchLocks(browseState.inspect ?? ''),
		enabled: () => session.isAdmin && null !== browseState.inspect && 'placement' === browseState.tab
	}));

	const placementServer = $derived.by((): InstanceItem | null => {
		if (undefined === instancesQuery.data || !instancesQuery.data.ok || null === entity) {
			return null;
		}
		const instance = entity.instance_id;
		return instancesQuery.data.value.find((item) => instance === item.instance_id) ?? null;
	});
	const placementLock = $derived(
		undefined !== locksQuery.data && locksQuery.data.ok
			? (locksQuery.data.value.held.find((held) => held.path === browseState.inspect) ?? null)
			: null
	);
	const inspectorTabs = $derived<InspectorTab[]>([
		{ id: 'details' },
		{ id: 'placement' },
		...(true === entity?.erasure_coded ? [{ id: 'durability' as const }] : []),
		{ id: 'links' },
		{ id: 'permissions' },
		{ id: 'activity' }
	]);
	const inspectorActions = $derived(null === entity ? [] : actions.rowItems(entity));

	const previewEntry = $derived(null === browseState.preview ? null : (browseState.entries.find((entry) => browseState.preview === entry.path) ?? null));
	const previewFiles = $derived(visible.filter((entry) => 'file' === entry.type));

	function rangeLoaderFor(path: string): RangeLoader {
		return async (from, to) => {
			const result = await readRange(path, from, to);
			if (result.ok) {
				return result.value;
			}
			const readFailure = toUiError(result.error, { action: 'browse.action.read_preview', object: path });
			notifications.add({
				tone: 'danger',
				category: 'error',
				title: t('browse.preview.failed'),
				body: readFailure.body,
				requestId: readFailure.requestId ?? null
			});
			return new ArrayBuffer(0);
		};
	}

	function thumbnailFor(entry: FileEntry): string | null {
		return 'image' === kindFromName(entry.name) ? fileUrl(entry.path) : null;
	}

	function open(entry: FileEntry): void {
		if ('directory' === entry.type) {
			browseState.navigate(entry.path);
			return;
		}
		browseState.preview = entry.path;
	}

	function rowEntry(target: EventTarget | null): FileEntry | null {
		if (!(target instanceof Element)) {
			return null;
		}
		const row = target.closest<HTMLElement>('[data-row-index]');
		if (null === row) {
			return null;
		}
		return visible[Number(row.dataset.rowIndex)] ?? null;
	}

	function toggleInspector(entry: FileEntry | null): void {
		if (null !== browseState.inspect) {
			browseState.inspect = null;
			return;
		}
		const target = entry ?? browseState.entries.find((candidate) => browseState.selection.ids.has(candidate.path)) ?? null;
		if (null !== target) {
			browseState.inspect = target.path;
		}
	}

	function toggleView(): void {
		browseState.view = 'list' === browseState.view ? 'grid' : 'list';
	}

	function toggleHidden(): void {
		browseState.showHidden = !browseState.showHidden;
	}

	function selectAll(): void {
		browseState.selection = selectPage(
			{ mode: 'multi', ids: browseState.selection.ids, allMatching: false },
			visible.map((entry) => entry.path)
		);
	}

	function clearAll(): void {
		browseState.selection = clearSelection(browseState.selection);
	}

	function refresh(): void {
		void listing.refetch();
	}

	function openUpload(): void {
		uploadInput?.click();
	}

	function pickFiles(event: Event): void {
		const input = event.currentTarget as HTMLInputElement;
		const files = Array.from(input.files ?? []);
		input.value = '';
		actions.upload(files);
	}

	function downloadEntity(): void {
		if (null !== browseState.inspect) {
			globalThis.location.assign(fileUrl(browseState.inspect, { download: true }));
		}
	}

	function closeInspector(): void {
		browseState.inspect = null;
	}

	function onKeyDownCapture(event: KeyboardEvent): void {
		if (' ' !== event.key || isEditable(event.target) || !(event.target instanceof Element) || null !== event.target.closest('button, a')) {
			return;
		}
		const entry = rowEntry(event.target);
		if (null === entry) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		browseState.preview = entry.path;
	}

	function onKeyDown(event: KeyboardEvent): void {
		if (isEditable(event.target) || event.ctrlKey || event.metaKey) {
			return;
		}
		if ('Backspace' === event.key || (event.altKey && 'ArrowUp' === event.key)) {
			if (ROOT_PATH !== browseState.path) {
				event.preventDefault();
				browseState.navigate(parentPath(browseState.path));
			}
			return;
		}
		if (event.altKey || event.shiftKey) {
			return;
		}
		if ('i' === event.key) {
			event.preventDefault();
			toggleInspector(rowEntry(event.target));
		} else if ('v' === event.key) {
			toggleView();
		} else if ('.' === event.key) {
			toggleHidden();
		} else if ('r' === event.key) {
			refresh();
		} else if ('u' === event.key) {
			event.preventDefault();
			openUpload();
		} else if ('n' === event.key) {
			event.preventDefault();
			void actions.newFolder();
		}
	}

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'files.browse.view',
				title: t('browse.command.view'),
				group: 'Files',
				keywords: [t('browse.title')],
				icon: 'grid',
				run: toggleView
			},
			{
				id: 'files.browse.hidden',
				title: t('browse.command.hidden'),
				group: 'Files',
				keywords: [t('browse.title')],
				icon: 'eye',
				run: toggleHidden
			},
			{
				id: 'files.browse.inspector',
				title: t('browse.command.inspector'),
				group: 'Files',
				keywords: [t('browse.title')],
				icon: 'inspector',
				run: () => toggleInspector(null)
			},
			{
				id: 'files.browse.selectAll',
				title: t('browse.command.select_all'),
				group: 'Files',
				keywords: [t('browse.title')],
				icon: 'check',
				run: selectAll
			},
			{
				id: 'files.browse.refresh',
				title: t('browse.command.refresh'),
				group: 'Files',
				keywords: [t('browse.title')],
				icon: 'refresh',
				run: refresh
			}
		]));
	});

	$effect(() => {
		const routed = page.params.path;
		const search = page.url.search;
		untrack(() => {
			browseState.hydrate(pathFromRoute(routed), new URLSearchParams(search), defaults());
		});
	});

	$effect(() => {
		const target = browseState.search();
		untrack(() => {
			if (target !== page.url.search) {
				const url = new URL(page.url);
				url.search = target;
				void goto(url, { replaceState: true, keepFocus: true, noScroll: true });
			}
		});
	});

	$effect(() => {
		const view = browseState.view;
		const hidden = browseState.showHidden;
		const sort = browseState.sort;
		untrack(() => {
			const files = preferences.value.files;
			if (files.view !== view || files.hidden !== hidden || files.sort.key !== sort.key || files.sort.dir !== sort.dir) {
				preferences.update({ files: { view, hidden, sort: { key: sort.key, dir: sort.dir } } });
			}
		});
	});

</script>

{#snippet retryAction()}
	<Button variant="secondary" size="sm" onclick={refresh}>{t('browse.retry')}</Button>
{/snippet}

{#snippet emptyFolder()}
	{#if ROOT_PATH === browseState.path}
		<EmptyState
			variant="first-use"
			icon="folder"
			title={t('browse.empty.root.title')}
			body={t('browse.empty.root.body')}
			primary={{ label: t('browse.empty.upload'), onclick: openUpload }}
			secondary={{ label: t('browse.empty.new_folder'), onclick: () => void actions.newFolder(), icon: 'folder-plus' }}
		/>
	{:else}
		<EmptyState
			variant="first-use"
			icon="folder"
			title={t('browse.empty.folder.title')}
			body={t('browse.empty.folder.body')}
			primary={{ label: t('browse.empty.upload'), onclick: openUpload }}
			secondary={{ label: t('browse.empty.new_folder'), onclick: () => void actions.newFolder(), icon: 'folder-plus' }}
		/>
	{/if}
{/snippet}

{#snippet linksBody()}
	<Stack gap="2">
		{#if null !== entity}
			<Button variant="secondary" iconStart="link" onclick={() => actions.openDrawer('share', [entity])}>
				{t('browse.links.create')}
			</Button>
		{/if}
		{#if 0 === linkItems.length}
			<Text tone="muted">{t('browse.links.empty')}</Text>
		{:else}
			<ul class="stack-list">
				{#each linkItems as link (link.id)}
					<li class="stack-row">
						<Text>{t(LINK_STATUS_KEYS[link.status])}</Text>
						<RelTime at={link.expires_at} />
					</li>
				{/each}
			</ul>
		{/if}
	</Stack>
{/snippet}

{#snippet permissionsBody()}
	{#if null !== permissionPath && null !== session.me}
		<PermissionInspector path={permissionPath} checks={permissionChecks} me={session.me} />
	{/if}
{/snippet}

{#snippet listBody()}
	{#if null !== failure && 0 === browseState.entries.length}
		{#if 'permission' === failure.layer}
			<EmptyState variant="no-permission" icon="lock" title={t('browse.permission.title')} body={t('browse.permission.body')} />
			{#if null !== permissionPath && null !== session.me}
				<PermissionInspector path={permissionPath} checks={permissionChecks} me={session.me} />
			{/if}
		{:else if 'not_found' === failure.layer}
			<EmptyState
				variant="not-available"
				icon="folder"
				title={t('browse.missing.title')}
				body={t('browse.missing.body')}
				primary={{ label: t('browse.missing.parent'), onclick: () => browseState.navigate(parentPath(browseState.path)) }}
			/>
		{:else}
			<ErrorState error={failure} onretry={refresh} />
		{/if}
	{:else if 'loading' === pageState && 'grid' === browseState.view}
		<Skeleton shape="rect" lines={4} />
	{:else if 'empty' === pageState}
		{@render emptyFolder()}
	{:else}
		{#if null !== failure}
			<InlineAlert tone="danger" title={failure.title} body={failure.body} actions={retryAction} />
		{/if}
		{#if 'partial' === pageState}
			<InlineAlert tone="warn" title={t('browse.partial.title')} body={t('browse.partial.body')} />
		{/if}
		{#if 0 < needle.length && 0 === visible.length}
			<EmptyState
				variant="no-results"
				icon="search"
				title={t('browse.filter.empty.title', { query: browseState.q })}
				body={t('browse.filter.empty.body')}
				primary={{ label: t('browse.filter.clear'), onclick: () => (browseState.q = '') }}
				secondary={{ label: t('browse.filter.search_everywhere'), href: `${base}/files/search?q=${encodeURIComponent(browseState.q)}` }}
			/>
		{:else if 'grid' === browseState.view}
			<div class="grid-host">
				<FileGrid entries={visible} bind:selection={browseState.selection} onopen={open} thumbnails={thumbnailFor} />
			</div>
		{:else}
			<FileTable
				entries={visible}
				status={listStatus}
				bind:selection={browseState.selection}
				bind:sort={() => browseState.sort, (next) => browseState.setSort(next)}
				bind:renaming={() => actions.renaming, (next) => { if (null === next) actions.stopRename(); }}
				density={preferences.value.density}
				total={listingPage?.total ?? null}
				{hasMore}
				onloadmore={() => void loadMore()}
				onopen={open}
				onrename={(entry, name) => void actions.rename(entry, name)}
				rowMenu={(entry) => actions.rowItems(entry)}
				hiddenVisible={browseState.showHidden}
			/>
		{/if}
	{/if}
{/snippet}

{#snippet detailBody()}
	{#if null !== browseState.inspect}
		<Inspector
			entity={entity}
			tabs={inspectorTabs}
			bind:value={() => browseState.tab, (next: string) => browseState.setTab(next)}
			onclose={closeInspector}
			ondownload={downloadEntity}
			actions={inspectorActions}
			server={placementServer}
			lock={placementLock}
			cache={null}
			{erasure}
			onverify={() => void runDurability(verify)}
			onrepair={() => void runDurability(repair)}
			busy={durabilityBusy}
			events={activityItems}
			activityStatus={activityStatus}
			links={linksBody}
			permissions={permissionsBody}
		/>
	{/if}
{/snippet}

{#snippet crumbsRegion()}
	<Breadcrumbs segments={crumbs} label={t('browse.breadcrumbs.label')} editable onpath={(path) => browseState.navigate(path)} />
{/snippet}

<PageFrame
	title={folderTitle}
	class={className}
	breadcrumbs={ROOT_PATH === browseState.path ? undefined : crumbsRegion}
	onkeydowncapture={onKeyDownCapture}
	onkeydown={onKeyDown}
>
	<div class={['browse', treeOpen && 'with-tree', null !== browseState.inspect && 'inspecting']}>
		{#if treeOpen}
			<nav class="tree" aria-label={t('browse.tree.label')}>
				<p class="tree-title">{t('browse.tree.toggle')}</p>
				<FileTree root={ROOT_PATH} load={loadFolders} current={browseState.path} onnavigate={(path) => browseState.navigate(path)} />
			</nav>
		{/if}
		<div class="main">
			<BrowseToolbar state={browseState} actions={actions} bind:treeOpen />
			<SplitView list={listBody} detail={detailBody} bind:detailOpen={() => null !== browseState.inspect, (open) => { if (!open) closeInspector(); }} bind:detailWidth label={t('browse.split.label')} />
		</div>
	</div>

	{#if 0 < browseState.selection.ids.size}
		<Dock>
			<BulkBar
				count={browseState.selection.ids.size}
				bytes={selectedBytes}
				allMatching={browseState.selection.allMatching}
				total={listingPage?.total ?? undefined}
				actions={actions.bulkItems()}
				onselectall={selectAll}
				onclear={clearAll}
			/>
		</Dock>
	{/if}

	<Dialog
		bind:open={() => null !== previewEntry, (open) => { if (!open) browseState.preview = null; }}
		title={previewEntry?.name ?? ''}
		size="lg"
	>
		{#if null !== previewEntry}
			<PreviewPane
				stat={previewEntry}
				src={fileUrl(previewEntry.path)}
				loadRange={rangeLoaderFor(previewEntry.path)}
				siblings={previewFiles}
				onnavigate={(path) => (browseState.preview = path)}
			/>
		{/if}
	</Dialog>

	<input bind:this={uploadInput} type="file" multiple hidden class="picker" onchange={pickFiles} />
	<FileActionDrawers {actions} />
</PageFrame>

<style>
	.browse {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.picker {
		display: none;
	}

	.grid-host {
		display: grid;
		block-size: calc(100dvh - var(--topbar-h) - var(--space-16) * 4);
		min-block-size: var(--inspector-w);
	}

	.main {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.tree {
		display: none;
		flex-direction: column;
		gap: var(--space-2);
		align-self: start;
		position: sticky;
		inset-block-start: var(--space-4);
		min-inline-size: 0;
		max-block-size: calc(100dvh - var(--topbar-h) - var(--space-8));
		overflow: auto;
		padding: var(--space-3) var(--space-2);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
	}

	.tree-title {
		padding-inline: var(--space-2);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		text-transform: uppercase;
		color: var(--text-muted);
	}

	.stack-list {
		display: grid;
		gap: var(--space-2);
		padding: 0;
		list-style: none;
	}

	.stack-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	@media (width >= 1024px) {
		.browse.with-tree {
			grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
		}

		.browse.with-tree .tree {
			display: flex;
		}
	}

	@media (width >= 1024px) and (width < 1536px) {
		.browse.with-tree.inspecting {
			grid-template-columns: minmax(0, 1fr);
		}

		.browse.with-tree.inspecting .tree {
			display: none;
		}
	}
</style>
