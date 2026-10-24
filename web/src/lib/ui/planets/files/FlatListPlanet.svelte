<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		countObjects,
		deletePrefix,
		listObjects,
		planPrefixDelete,
		type ObjectInfo,
		type ObjectListPage,
		type PrefixDeletePlan,
		type PrefixDeleteResult
	} from '$lib/api/objects';
	import type { ObjectPage, Result } from '$lib/api/types';
	import type { ApiResponse } from '$lib/api/http';
	import type { ApiFailure } from '$lib/api/errors';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatNumber } from '$lib/format/number';
	import { prefixBoundary } from '$lib/format/path';
	import BytesCell from '../../cells/BytesCell.svelte';
	import ETagCell from '../../cells/ETagCell.svelte';
	import PathCell from '../../cells/PathCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import AlertDialog from '../../organisms/AlertDialog.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import Button from '../../atoms/Button.svelte';
	import Switch from '../../atoms/Switch.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import CursorPager from '../../molecules/CursorPager.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import PathInput from '../../molecules/PathInput.svelte';
	import HelpText from '../../atoms/HelpText.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import type { ColumnDef } from '../../types';

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	function dataOrReject<T>(outcome: Result<ApiResponse<T>, ApiFailure>): Promise<T> {
		return outcome.ok ? Promise.resolve(outcome.value.data) : Promise.reject(outcome.error);
	}

	const PAGE_LIMIT = 1000;
	const FORBIDDEN = 403;

	const initialPrefix = page.url.searchParams.get('prefix') ?? '';
	let pathText = $state(initialPrefix);
	let prefix = $state(initialPrefix);
	let matchFolder = $state(true);
	let cursors = $state<(string | null)[]>([null]);
	let pageIndex = $state(0);
	let confirmOpen = $state(false);
	let planned = $state<PrefixDeletePlan>({ would_delete: 0, bytes: 0, forbidden: 0 });
	let report = $state<PrefixDeleteResult | null>(null);

	const effectivePrefix = $derived(matchFolder ? prefixBoundary(prefix) : prefix);
	const startAfter = $derived(cursors[pageIndex] ?? null);

	const listing = createQuery<ObjectListPage>(() => ({
		key: ['objects', effectivePrefix, startAfter],
		fetch: async () => dataOrReject(await listObjects(effectivePrefix, startAfter, PAGE_LIMIT)),
		isEmpty: (list) => 0 === list.objects.length
	}));

	const totals = createQuery<ObjectPage>(() => ({
		key: ['objects-count', effectivePrefix],
		fetch: async () => dataOrReject(await countObjects(effectivePrefix))
	}));

	const objects = $derived<ObjectInfo[]>(listing.data?.objects ?? []);
	const hasNext = $derived(true === listing.data?.truncated);
	const hasPrev = $derived(0 < pageIndex);

	const listError = $derived(
		null === listing.error
			? undefined
			: FORBIDDEN === listing.error.status
				? {
						...listing.error,
						title: t('search.flat.forbidden_title'),
						body: t('search.flat.forbidden_body')
					}
				: listing.error
	);

	const footerText = $derived(
		undefined === totals.data
			? t('common.loading_label')
			: t('files-screens.flat.totals', {
					total: true === totals.data.truncated
						? t('search.flat.total_more', { total: formatNumber(totals.data.count) })
						: formatNumber(totals.data.count),
					bytes: formatBytes(totals.data.bytes).text
				})
	);

	const planMutation = createMutation<string, PrefixDeletePlan>({
		run: async (target) => dataOrReject(await planPrefixDelete(target)),
		context: { action: 'search.flat.action_plan' }
	});

	const deleteMutation = createMutation<string, PrefixDeleteResult>({
		run: async (target) => dataOrReject(await deletePrefix(target)),
		context: { action: 'search.flat.action_delete' },
		invalidate: () => [['objects'], ['objects-count']]
	});

	async function openDelete(): Promise<void> {
		const plan = await planMutation.run(effectivePrefix);
		if (null === plan) {
			return;
		}
		report = null;
		planned = plan;
		confirmOpen = true;
	}

	async function confirmDelete(): Promise<void> {
		const result = await deleteMutation.run(effectivePrefix);
		if (null === result) {
			return;
		}
		report = result;
		cursors = [null];
		pageIndex = 0;
	}

	const deleteBody = $derived(
		0 < planned.forbidden
			? `${t('search.flat.delete_body', { count: formatNumber(planned.would_delete), bytes: formatBytes(planned.bytes).text })} ${t('search.flat.delete_forbidden', { count: formatNumber(planned.forbidden) })}`
			: t('search.flat.delete_body', { count: formatNumber(planned.would_delete), bytes: formatBytes(planned.bytes).text })
	);

	function nextPage(): void {
		const next = listing.data?.next_start_after;
		if (undefined === next) {
			return;
		}
		cursors = [...cursors.slice(0, pageIndex + 1), next];
		pageIndex += 1;
	}

	function previousPage(): void {
		if (hasPrev) {
			pageIndex -= 1;
		}
	}

	function updateUrl(value: string): void {
		const url = new URL(page.url);
		if ('' === value) {
			url.searchParams.delete('prefix');
		} else {
			url.searchParams.set('prefix', value);
		}
		void goto(url, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function submitPath(): void {
		prefix = pathText.trim();
		cursors = [null];
		pageIndex = 0;
		report = null;
		updateUrl(prefix);
	}

	const highlight = $derived(prefix);

	const columns: ColumnDef<ObjectInfo>[] = [
		{ id: 'path', header: t('search.column.path'), cell: pathCell, priority: 1 },
		{ id: 'size', header: t('search.column.size'), cell: sizeCell, align: 'end', priority: 1 },
		{ id: 'version', header: t('search.column.version'), cell: versionCell },
		{ id: 'modified', header: t('search.column.modified'), cell: modifiedCell, priority: 1 }
	];
	const rowKey = (object: ObjectInfo): string => object.path;
</script>

{#snippet pathCell(object: ObjectInfo)}
	<PathCell path={object.path} {highlight} />
{/snippet}

{#snippet sizeCell(object: ObjectInfo)}
	<BytesCell value={object.size} />
{/snippet}

{#snippet versionCell(object: ObjectInfo)}
	<ETagCell value={object.etag} />
{/snippet}

{#snippet modifiedCell(object: ObjectInfo)}
	<TimeCell value={object.mtime} />
{/snippet}

{#snippet deleteAction()}
	<Button variant="secondary" iconStart="trash" loading={planMutation.pending} onclick={openDelete}>
		{t('search.flat.delete_button')}
	</Button>
{/snippet}

{#snippet empty()}
	<EmptyState
		variant="no-results"
		icon="folder"
		title={t('search.flat.empty_title')}
		body={t('search.flat.empty_body')}
	/>
{/snippet}

{#snippet footer()}
	<CursorPager
		{hasPrev}
		{hasNext}
		onprev={previousPage}
		onnext={nextPage}
		countLabel={footerText}
	/>
{/snippet}

<PageFrame title={t('search.flat.title')} job={t('files-screens.flat.job')} class={className} secondary={deleteAction}>
	{#snippet toolbar()}
		<form
			class="query"
			onsubmit={(event) => {
				event.preventDefault();
				submitPath();
			}}
		>
			<div class="path">
				<PathInput bind:value={pathText} label={t('search.flat.path.label')} placeholder={t('files-screens.flat.placeholder')} onsubmit={submitPath} />
				<HelpText id="flat-path-help">{t('search.flat.path.help')}</HelpText>
			</div>
			<div class="submit">
				<Button type="submit" variant="primary" iconStart="list">{t('files-screens.flat.list')}</Button>
			</div>
			<div class="folder">
				<Switch bind:checked={matchFolder} label={t('search.flat.folder.label')} description={t('search.flat.folder.description')} />
			</div>
		</form>
	{/snippet}



	{#if null !== planMutation.error}
		<InlineAlert tone="danger" title={planMutation.error.title} body={planMutation.error.body} />
	{/if}
	{#if null !== deleteMutation.error}
		<InlineAlert tone="danger" title={deleteMutation.error.title} body={deleteMutation.error.body} />
	{/if}
	{#if null !== report}
		<InlineAlert tone="ok" title={t('search.flat.deleted', { count: formatNumber(report.deleted) })} />
	{/if}

	<DataTable
		rows={objects}
		{columns}
		{rowKey}
		label={t('search.flat.table.label')}
		status={listing.status}
		error={listError}
		empty={empty}
		{footer}
	/>

	<AlertDialog
		bind:open={confirmOpen}
		title={t('search.flat.delete_title', { prefix: effectivePrefix })}
		body={deleteBody}
		confirmLabel={t('search.flat.delete_confirm', { count: formatNumber(planned.would_delete) })}
		tone="danger"
		typedWord={t('search.flat.typed_word')}
		onconfirm={confirmDelete}
	/>
</PageFrame>

<style>
	.query {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--space-4) var(--space-3);
		inline-size: 100%;
		padding: var(--card-pad);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
	}

	.path {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.submit {
		display: flex;
		padding-block-start: calc(var(--text-sm-line) + var(--space-1-5));
	}

	.folder {
		grid-column: 1 / -1;
		max-inline-size: var(--form-max);
		padding-block-start: var(--space-4);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	@media (width >= 1024px) {
		.query {
			grid-template-columns: minmax(0, 1fr) auto minmax(0, var(--inspector-w-min));
		}

		.folder {
			grid-column: auto;
			padding-block-start: 0;
			padding-inline-start: var(--space-4);
			border-block-start: 0;
			border-inline-start: var(--hairline) solid var(--border-subtle);
		}
	}
</style>
