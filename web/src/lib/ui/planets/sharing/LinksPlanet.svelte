<script lang="ts" module>
	import type { LinkStatus } from '$lib/api/links';
	import type { Link } from '$lib/api/types';
	import type { StatusKey } from '$lib/ui/types';

	const HOUR_MS = 3_600_000;
	const DEFAULT_EXPIRY_SECONDS = 3_600;

	const STATUS_BY_TAB: Record<string, LinkStatus | undefined> = {
		active: 'active',
		used: 'used',
		expired: 'expired',
		revoked: 'revoked',
		all: undefined
	};

	const LINK_STATUS_KEY: Record<Link['status'], StatusKey> = {
		active: 'link.active',
		used: 'link.used',
		expired: 'link.expired',
		revoked: 'link.revoked'
	};

	function rowKey(link: Link): string {
		return String(link.id);
	}

	function remainingMs(link: Link): number {
		return Date.parse(link.expires_at) - Date.now();
	}

	function fileName(path: string): string {
		return path.split('/').filter(Boolean).at(-1) ?? path;
	}

	function folderOf(path: string): string {
		const parts = path.split('/').filter(Boolean);
		return 1 >= parts.length ? '/' : parts.slice(0, -1).join('/');
	}

	function expiresSoon(link: Link): boolean {
		const remaining = remainingMs(link);
		return 'active' === link.status && 0 < remaining && HOUR_MS > remaining;
	}
</script>

<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { runWorker } from '$lib/api/admin/jobs';
	import { toUiError } from '$lib/api/errors';
	import { generateLink, linkStats, listLinks, revokeLink } from '$lib/api/links';
	import { stat } from '$lib/api/files';
	import type { Link as LinkRow } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatDuration } from '$lib/format/duration';
	import { formatRelative } from '$lib/format/time';
	import { session } from '$lib/stores/session.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { ColumnDef, MenuItem, QueryStatus, Selection, UiError } from '$lib/ui/types';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import HelpText from '$lib/ui/atoms/HelpText.svelte';
	import Label from '$lib/ui/atoms/Label.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import StatusCell from '$lib/ui/cells/StatusCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import IPCell from '$lib/ui/cells/IPCell.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Tabs from '$lib/ui/molecules/Tabs.svelte';
	import CursorPager from '$lib/ui/molecules/CursorPager.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Icon from '$lib/ui/quarks/Icon.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import Surface from '$lib/ui/quarks/Surface.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import BulkBar from '$lib/ui/organisms/BulkBar.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import ShareLinkForm from '$lib/ui/organisms/ShareLinkForm.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';

	type Tile = ComponentProps<typeof KpiStrip>['tiles'][number];

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	let tab = $state('active');
	let cursorsByTab = $state<Record<string, string[]>>({});
	const stack = $derived(cursorsByTab[tab] ?? []);
	const cursor = $derived(stack.at(-1) ?? null);

	const listQuery = createQuery(() => ({
		key: ['links', tab, cursor],
		fetch: () => listLinks({ status: STATUS_BY_TAB[tab] }, cursor),
		staleMs: 5000,
		revalidateOnFocus: true
	}));

	const statsQuery = createQuery({
		key: ['link-stats', '24h'],
		fetch: () => linkStats('24h'),
		staleMs: 5000,
		revalidateOnFocus: true
	});

	$effect(() => listQuery.attach());
	$effect(() => statsQuery.attach());

	const listResult = $derived(listQuery.data);
	const listError = $derived(false === listResult?.ok ? listResult.error : null);
	const rows = $derived(true === listResult?.ok ? listResult.value.items : []);
	const nextCursor = $derived(true === listResult?.ok ? listResult.value.next_cursor : null);
	const counts = $derived(true === listResult?.ok ? listResult.value.counts : null);
	const totalLinks = $derived(counts ? counts.active + counts.used + counts.expired + counts.revoked : 0);
	const firstUse = $derived(null !== counts && 0 === totalLinks);
	const listState = $derived.by((): QueryStatus => {
		if (undefined === listResult) {
			return 'loading';
		}
		if (false === listResult.ok) {
			return 'error';
		}
		return 0 === listResult.value.items.length ? 'empty' : 'success';
	});

	const tabItems = $derived([
		{ id: 'active', label: t('sharing.tabs.active'), count: counts?.active },
		{ id: 'used', label: t('sharing.tabs.used'), count: counts?.used },
		{ id: 'expired', label: t('sharing.tabs.expired'), count: counts?.expired },
		{ id: 'revoked', label: t('sharing.tabs.revoked'), count: counts?.revoked },
		{ id: 'all', label: t('sharing.tabs.all'), count: counts ? totalLinks : undefined }
	]);

	const statsResult = $derived(statsQuery.data);
	const stats = $derived(true === statsResult?.ok ? statsResult.value : null);
	const statsError = $derived(false === statsResult?.ok ? statsResult.error : null);
	const failedOpens = $derived(
		stats ? stats.consumed.not_found + stats.consumed.expired + stats.consumed.invalid : null
	);
	const totalOpens = $derived(
		stats ? stats.consumed.success + stats.consumed.not_found + stats.consumed.expired + stats.consumed.invalid : null
	);
	const tiles = $derived.by((): Tile[] => [
		{ label: t('sharing.kpi.active'), value: stats?.active ?? null, unit: 'count' },
		{ label: t('sharing.kpi.used'), value: stats?.used_last_24h ?? null, unit: 'count' },
		{
			label: t('sharing.kpi.failed'),
			value: failedOpens,
			unit: 'count',
			base: null === failedOpens || null === totalOpens ? undefined : { num: failedOpens, den: totalOpens },
			help: stats
				? t('sharing.kpi.reasons', {
						expired: stats.consumed.expired,
						not_found: stats.consumed.not_found,
						invalid: stats.consumed.invalid
					})
				: undefined
		}
	]);

	const lastCleanup = $derived(stats?.last_cleanup ?? null);
	const lastCleanupText = $derived.by(() => {
		if (null === lastCleanup || null === lastCleanup.at) {
			return t('sharing.cleanup.never');
		}
		return t('sharing.cleanup.last', {
			when: formatRelative(lastCleanup.at),
			expired: lastCleanup.expired_removed,
			used: lastCleanup.used_removed
		});
	});

	let cleanupError = $state<UiError | null>(null);

	let cleanupBusy = $state(false);

	async function cleanUp(): Promise<void> {
		cleanupBusy = true;
		const result = await runWorker('link_cleanup');
		cleanupBusy = false;
		if (false === result.ok) {
			cleanupError = toUiError(result.error, { action: 'sharing.cleanup' });
			return;
		}
		cleanupError = null;
		toasts.push({ tone: 'ok', title: t('sharing.cleanup.started') });
		invalidate(['link-stats']);
	}

	let selection = $state<Selection>({ mode: 'multi', ids: new Set<string>(), allMatching: false });
	const selectedIds = $derived([...selection.ids].map(Number));
	const selectedCount = $derived(selection.ids.size);

	let revokeOpen = $state(false);
	let revokeIds = $state<number[]>([]);
	let revokeBusy = $state(false);
	let actionError = $state<UiError | null>(null);

	const revokeTitle = $derived(
		1 === revokeIds.length ? t('sharing.revoke.title') : t('sharing.bulk.title', { count: revokeIds.length })
	);
	const revokeBody = $derived(
		1 === revokeIds.length ? t('sharing.revoke.body') : t('sharing.bulk.body', { count: revokeIds.length })
	);
	const revokeLabel = $derived(
		1 === revokeIds.length ? t('sharing.revoke.confirm') : t('sharing.bulk.revoke', { count: revokeIds.length })
	);

	function askRevoke(ids: number[]): void {
		revokeIds = ids;
		revokeOpen = true;
	}

	async function confirmRevoke(): Promise<void> {
		revokeBusy = true;
		const results = await Promise.all(revokeIds.map((id) => revokeLink(id)));
		revokeBusy = false;
		const failures = results.flatMap((result) => (false === result.ok ? [result.error] : []));
		const revokedCount = results.length - failures.length;
		const [firstFailure] = failures;
		actionError =
			undefined === firstFailure
				? null
				: {
						title: t('sharing.bulk.failed', { count: failures.length }),
						body: firstFailure.body,
						retryable: firstFailure.retryable
					};
		revokeOpen = false;
		selection = { mode: 'multi', ids: new Set<string>(), allMatching: false };
		invalidate(['links']);
		invalidate(['link-stats']);
		if (0 < revokedCount) {
			toasts.push({ tone: 'ok', title: t('sharing.revoked', { count: revokedCount }) });
		}
	}

	function copyPath(link: LinkRow): void {
		navigator.clipboard.writeText(link.file_path).then(
			() => toasts.push({ tone: 'ok', title: t('sharing.copied') }),
			() => toasts.push({ tone: 'danger', title: t('sharing.copy_failed') })
		);
	}

	function navigate(href: string): void {
		goto(`${base}${href}`).catch(() => toasts.push({ tone: 'danger', title: t('sharing.navigate_failed') }));
	}

	function rowMenu(link: LinkRow): MenuItem[] {
		return [
			{
				id: 'revoke',
				label: t('sharing.row.revoke'),
				icon: 'link-off',
				tone: 'danger',
				disabledReason: 'active' === link.status ? undefined : t('sharing.revoke.not_active'),
				onselect: () => askRevoke([link.id])
			},
			{ id: 'copy', label: t('sharing.row.copy_path'), icon: 'copy', onselect: () => copyPath(link) },
			{
				id: 'open',
				label: t('sharing.row.open_file'),
				icon: 'file',
				onselect: () => navigate(`/files${link.file_path}`)
			},
			{
				id: 'details',
				label: t('sharing.row.details'),
				icon: 'info',
				onselect: () => navigate(`/sharing/${link.id}`)
			}
		];
	}

	const bulkActions = $derived.by((): MenuItem[] => [
		{
			id: 'revoke',
			label: t('sharing.bulk.revoke', { count: selectedCount }),
			icon: 'link-off',
			tone: 'danger',
			onselect: () => askRevoke(selectedIds)
		}
	]);

	function selectAll(): void {
		selection = { mode: 'multi', ids: new Set(rows.map(rowKey)), allMatching: false };
	}

	function clearSelection(): void {
		selection = { mode: 'multi', ids: new Set<string>(), allMatching: false };
	}

	let newOpen = $state(false);
	let newPath = $state('');
	let newFile = $state<{ path: string; size: number } | null>(null);
	let newError = $state<UiError | null>(null);
	let newChecking = $state(false);
	let newBusy = $state(false);
	let newExpiry = $state(DEFAULT_EXPIRY_SECONDS);
	let newResult = $state<{ url: string; expiresAt: string } | null>(null);

	function openNew(path: string): void {
		newPath = path;
		newFile = null;
		newError = null;
		newResult = null;
		newExpiry = DEFAULT_EXPIRY_SECONDS;
		newOpen = true;
	}

	$effect(() => {
		if ('1' === page.url.searchParams.get('new')) {
			openNew(page.url.searchParams.get('path') ?? '');
		}
	});

	async function checkFile(): Promise<void> {
		newChecking = true;
		const result = await stat(newPath);
		newChecking = false;
		if (false === result.ok) {
			newError = toUiError(result.error, { action: 'sharing.check', object: newPath });
			return;
		}
		if ('directory' === result.value.data.type) {
			newError = { title: t('sharing.new.folder.title'), body: t('sharing.new.folder.body'), retryable: false };
			return;
		}
		newError = null;
		newFile = { path: newPath, size: result.value.data.size };
	}

	async function generate(): Promise<void> {
		if (null === newFile) {
			return;
		}
		newBusy = true;
		const result = await generateLink(newFile.path, newExpiry);
		newBusy = false;
		if (false === result.ok) {
			newError = result.error;
			return;
		}
		newError = null;
		newResult = { url: result.value.url, expiresAt: result.value.expires };
		invalidate(['links']);
		invalidate(['link-stats']);
	}

	function nextPage(): void {
		if (null !== nextCursor) {
			cursorsByTab = { ...cursorsByTab, [tab]: [...stack, nextCursor] };
		}
	}

	function previousPage(): void {
		cursorsByTab = { ...cursorsByTab, [tab]: stack.slice(0, -1) };
	}

	function retryList(): void {
		void listQuery.refetch();
	}

	function retryStats(): void {
		void statsQuery.refetch();
	}

	function openDetail(link: LinkRow): void {
		navigate(`/sharing/${link.id}`);
	}

	const columns: ColumnDef<LinkRow>[] = [
		{ id: 'file', header: t('sharing.col.file'), cell: fileCell, width: 300, minWidth: 220, priority: 1 },
		{ id: 'status', header: t('sharing.col.status'), cell: statusCell, width: 110, priority: 1 },
		{ id: 'expires', header: t('sharing.col.expires'), cell: expiresCell, width: 140, priority: 1 },
		{ id: 'created', header: t('sharing.col.created'), cell: createdCell, width: 140, priority: 2 },
		{ id: 'created_by', header: t('sharing.col.created_by'), cell: createdByCell, width: 120, priority: 3 },
		{ id: 'used', header: t('sharing.col.used'), cell: usedCell, width: 130, priority: 3 },
		{ id: 'used_from', header: t('sharing.col.used_from'), cell: usedFromCell, width: 130, priority: 3 },
		{ id: 'link_id', header: t('sharing.col.link_id'), cell: linkIdCell, width: 120, hidden: true, priority: 3 }
	];
</script>

{#snippet fileCell(link: LinkRow)}
	<span class="file" title={link.file_path}>
		<Icon name="file" size="sm" />
		<a class="file-name" href={`${base}/files${link.file_path}`}>{fileName(link.file_path)}</a>
		<span class="file-dir">{folderOf(link.file_path)}</span>
	</span>
{/snippet}

{#snippet statusCell(link: LinkRow)}
	<StatusCell status={LINK_STATUS_KEY[link.status]} />
{/snippet}

{#snippet createdByCell(link: LinkRow)}
	<TextCell value={link.created_by} />
{/snippet}

{#snippet createdCell(link: LinkRow)}
	<TimeCell value={link.created_at} />
{/snippet}

{#snippet expiresCell(link: LinkRow)}
	{#if expiresSoon(link)}
		<span class="soon"><Icon name="hourglass" size="sm" />{t('sharing.expires_in', { time: formatDuration(remainingMs(link), 'short') })}</span>
	{:else}
		<TimeCell value={link.expires_at} />
	{/if}
{/snippet}

{#snippet usedCell(link: LinkRow)}
	<TimeCell value={link.used_at} />
{/snippet}

{#snippet usedFromCell(link: LinkRow)}
	<IPCell value={link.used_by_ip} />
{/snippet}

{#snippet linkIdCell(link: LinkRow)}
	<TextCell value={link.token_prefix} mono />
{/snippet}

<PageFrame title={t('sharing.title')} job={t('sharing.job')} class={className}>
	{#snippet scope()}
		{#if session.isAdmin && null !== stats && !firstUse}
			<span class="meta">
				<Icon name="clock" size="sm" />
				<Text tone="muted" size="sm">{lastCleanupText}</Text>
				<Button variant="ghost" size="sm" iconStart="refresh" loading={cleanupBusy} onclick={() => void cleanUp()}>
					{t('sharing.cleanup')}
				</Button>
			</span>
		{/if}
	{/snippet}

	{#snippet primary()}
		{#if !firstUse}
			<Button variant="primary" iconStart="plus" onclick={() => openNew('')}>{t('sharing.new_link')}</Button>
		{/if}
	{/snippet}

	{#if null !== cleanupError}
		<InlineAlert tone="danger" title={cleanupError.title} body={cleanupError.body} />
	{/if}

	{#if firstUse}
		<Surface level="raised" pad="card" radius="lg" border>
			<EmptyState
				variant="first-use"
				icon="link"
				title={t('sharing.empty.first.title')}
				body={t('sharing.empty.first.body')}
				primary={{ label: t('sharing.new_link'), onclick: () => openNew('') }}
			>
				{#snippet example()}
					<ul class="facts" aria-label={t('sharing.facts.label')}>
						<li><Icon name="users" size="sm" /><Text size="sm">{t('sharing.facts.anyone')}</Text></li>
						<li><Icon name="download" size="sm" /><Text size="sm">{t('sharing.facts.used_up')}</Text></li>
						<li><Icon name="clock" size="sm" /><Text size="sm">{t('sharing.facts.expires')}</Text></li>
					</ul>
				{/snippet}
			</EmptyState>
		</Surface>
	{:else}
		{#if null !== statsError}
			<InlineAlert tone="warn" title={t('sharing.stats.error.title')} body={t('sharing.stats.error.body')}>
				{#snippet actions()}
					<Button size="sm" variant="secondary" onclick={retryStats}>{t('common.retry')}</Button>
				{/snippet}
			</InlineAlert>
		{:else}
			<KpiStrip {tiles} />
		{/if}

		{#if null !== actionError}
			<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
		{/if}

		{#if null !== listError}
			<ErrorState error={listError} onretry={retryList} />
		{:else}
			<DataTable
				{rows}
				{columns}
				{rowKey}
				label={t('sharing.table.label')}
				status={listState}
				bind:selection
				{rowMenu}
				onopen={openDetail}
			>
				{#snippet toolbar()}
					<div class="tabs-scroll">
						<Tabs bind:value={tab} items={tabItems} label={t('sharing.tabs.label')} />
					</div>
				{/snippet}
				{#snippet empty()}
					<EmptyState
						variant="no-results"
						icon="filter"
						title={t('sharing.empty.view.title')}
						body={t('sharing.empty.view.body')}
					/>
				{/snippet}
				{#snippet footer()}
					{#if 0 < stack.length || null !== nextCursor}
						<CursorPager
							hasPrev={0 < stack.length}
							hasNext={null !== nextCursor}
							onprev={previousPage}
							onnext={nextPage}
							countLabel={t('sharing.count', { count: rows.length })}
						/>
					{/if}
				{/snippet}
			</DataTable>
		{/if}

		{#if 0 < selectedCount}
			<BulkBar
				count={selectedCount}
				bytes={null}
				allMatching={false}
				actions={bulkActions}
				onselectall={selectAll}
				onclear={clearSelection}
			/>
		{/if}
	{/if}
</PageFrame>

<AlertDialog
	bind:open={revokeOpen}
	title={revokeTitle}
	body={revokeBody}
	confirmLabel={revokeLabel}
	tone="danger"
	onconfirm={() => void confirmRevoke()}
/>

<Dialog bind:open={newOpen} title={t('sharing.new.title')} description={t('sharing.new.body')}>
	{#if null === newFile}
		<Stack gap="2">
			<Label for="new-link-path">{t('sharing.new.path')}</Label>
			<Input id="new-link-path" bind:value={newPath} mono invalid={null !== newError} aria-describedby="new-link-path-help" />
			<HelpText id="new-link-path-help">{t('sharing.new.path_help')}</HelpText>
			{#if null !== newError}
				<InlineAlert tone="danger" title={newError.title} body={newError.body} />
			{/if}
		</Stack>
	{:else}
		<ShareLinkForm
			path={newFile.path}
			size={newFile.size}
			bind:value={newExpiry}
			result={newResult}
			busy={newBusy}
			ongenerate={generate}
		/>
		{#if null !== newError}
			<InlineAlert tone="danger" title={newError.title} body={newError.body} />
		{/if}
	{/if}
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (newOpen = false)}>{t('common.close')}</Button>
			{#if null === newFile}
				<Button
					variant="primary"
					loading={newChecking}
					disabled={'' === newPath || newChecking}
					onclick={() => void checkFile()}
				>
					{t('sharing.new.check')}
				</Button>
			{/if}
		</Cluster>
	{/snippet}
</Dialog>

<style>
	.meta {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1-5);
		color: var(--text-muted);
	}

	.tabs-scroll {
		max-inline-size: calc(var(--viewport-w) - 2 * var(--gutter));
		min-inline-size: 0;
		overflow-x: auto;
		scrollbar-width: none;
	}

	.facts {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--space-2) var(--space-6);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.facts li {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-muted);
	}

	.file {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
		color: var(--text-muted);
	}

	.file-name {
		flex: 0 1 auto;
		min-inline-size: 0;
		overflow: hidden;
		color: var(--text);
		font-weight: var(--text-label-weight);
		text-decoration: none;
		text-overflow: ellipsis;
		white-space: nowrap;
		border-radius: var(--radius-sm);
	}

	@media (hover: hover) {
		.file-name:hover {
			color: var(--text-link);
			text-decoration: underline;
		}
	}

	.file-name:focus-visible {
		box-shadow: var(--focus-ring);
		outline: none;
	}

	.file-dir {
		flex: 1 1 0;
		min-inline-size: 0;
		overflow: hidden;
		font-size: var(--text-sm-size);
		color: var(--text-muted);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.soon {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1-5);
		color: var(--warn-text);
		font-weight: var(--text-label-weight);
		white-space: nowrap;
	}
</style>
