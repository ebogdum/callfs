<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { search } from '$lib/api/search';
	import type { FileEntry, SearchQuery, SearchResult } from '$lib/api/types';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { defaultLocale, formatFixed } from '$lib/format/number';
	import { parentPath } from '$lib/format/path';
	import { preferences } from '$lib/stores/preferences.svelte';
	import * as queryGrammar from '../../organisms/query-grammar';
	import QueryBar from '../../organisms/QueryBar.svelte';
	import FacetSidebar from '../../organisms/FacetSidebar.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import BytesCell from '../../cells/BytesCell.svelte';
	import OwnerCell from '../../cells/OwnerCell.svelte';
	import PathCell from '../../cells/PathCell.svelte';
	import StorageCell from '../../cells/StorageCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import Button from '../../atoms/Button.svelte';
	import Text from '../../atoms/Text.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import type { Chip, ColumnDef, Facet } from '../../types';

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	const DAY_MS = 86_400_000;
	const SIZE_FACET_VALUES = ['1048576', '104857600', '1073741824'];

	const initialQuery = page.url.searchParams.get('q') ?? '';
	let text = $state(initialQuery);
	let submitted = $state(initialQuery);
	let include = $state<Chip[]>([]);
	let filtersOpen = $state(false);

	const parsed = $derived(queryGrammar.parse(submitted));

	const toSearchQuery = (source: queryGrammar.SearchQuery): SearchQuery => ({
		q: source.name ?? source.text,
		path: source.path,
		type: source.type,
		min_size: source.minSize,
		max_size: source.maxSize,
		modified_after: source.modifiedAfter,
		modified_before: source.modifiedBefore,
		owner: source.owner,
		storage: source.storage,
		instance: source.server,
		erasure: source.split,
		locked: source.locked
	});

	const applyChip = (query: SearchQuery, chip: Chip): SearchQuery => {
		if ('type' === chip.facet) {
			return { ...query, type: 'directory' === chip.value ? 'directory' : 'file' };
		}
		if ('storage' === chip.facet) {
			return { ...query, storage: chip.value };
		}
		if ('owner' === chip.facet) {
			return { ...query, owner: chip.value };
		}
		if ('server' === chip.facet) {
			return { ...query, instance: chip.value };
		}
		if ('modified' === chip.facet) {
			const days = Number.parseInt(chip.value, 10);
			return { ...query, modified_after: new Date(Date.now() - days * DAY_MS).toISOString() };
		}
		if ('size' === chip.facet) {
			return { ...query, min_size: Number(chip.value) };
		}
		if ('split' === chip.facet) {
			return { ...query, erasure: true };
		}
		if ('locked' === chip.facet) {
			return { ...query, locked: true };
		}
		return query;
	};

	const query = $derived(include.reduce<SearchQuery>(applyChip, toSearchQuery(parsed.query)));
	const hasQuery = $derived(0 < Object.values(query).filter((value) => undefined !== value).length);

	const results = createQuery<SearchResult>(() => ({
		key: ['search', query],
		fetch: () => fetchPage(null),
		enabled: () => hasQuery,
		isEmpty: (result) => 0 === result.items.length,
		isPartial: (result) => result.scan_truncated
	}));

	function fetchPage(cursor: string | null): Promise<SearchResult> {
		return search(query, cursor).then((outcome) =>
			false === outcome.ok ? Promise.reject(outcome.error) : outcome.value.data
		);
	}

	const moreResults = createMutation<string, SearchResult>({
		run: fetchPage,
		context: { action: 'search.action.load_more' }
	});

	async function loadMore(): Promise<void> {
		const current = results.data;
		if (undefined === current || null === current.next_cursor) {
			return;
		}
		const next = await moreResults.run(current.next_cursor);
		if (null === next) {
			return;
		}
		results.setData((data) =>
			undefined === data
				? data
				: {
						...data,
						items: [...data.items, ...next.items],
						next_cursor: next.next_cursor,
						scan_truncated: next.scan_truncated,
						scanned: next.scanned,
						took_ms: data.took_ms + next.took_ms
					}
		);
	}

	const counts = $derived(results.data?.facets);
	const items = $derived<FileEntry[]>(results.data?.items ?? []);
	const hasMore = $derived(undefined !== results.data && null !== results.data.next_cursor);
	const truncated = $derived(true === results.data?.scan_truncated);

	const facetValues = (group: Record<string, number> | undefined) =>
		Object.entries(group ?? {}).map(([value, count]) => ({ value, label: value, count }));

	const allFacets = $derived<Facet[]>([
		{
			key: 'type',
			label: t('search.facet.type'),
			values: [
				{ value: 'file', label: t('search.facet.type.file'), count: counts?.type.file },
				{ value: 'directory', label: t('search.facet.type.directory'), count: counts?.type.directory }
			]
		},
		{ key: 'storage', label: t('search.facet.storage'), values: facetValues(counts?.storage) },
		{ key: 'owner', label: t('search.facet.owner'), values: facetValues(counts?.owner) },
		{ key: 'server', label: t('search.facet.server'), values: facetValues(counts?.instance) },
		{
			key: 'modified',
			label: t('search.facet.modified'),
			values: [
				{ value: '1d', label: t('search.facet.modified.1d') },
				{ value: '7d', label: t('search.facet.modified.7d') },
				{ value: '30d', label: t('search.facet.modified.30d') }
			]
		},
		{
			key: 'size',
			label: t('search.facet.size'),
			values: [
				{ value: SIZE_FACET_VALUES[0], label: t('search.facet.size.1mb') },
				{ value: SIZE_FACET_VALUES[1], label: t('search.facet.size.100mb') },
				{ value: SIZE_FACET_VALUES[2], label: t('search.facet.size.1gb') }
			]
		},
		{ key: 'split', label: t('search.facet.split'), values: [{ value: 'yes', label: t('search.facet.split.yes') }] },
		{ key: 'locked', label: t('search.facet.locked'), values: [{ value: 'yes', label: t('search.facet.locked.yes') }] }
	]);

	const facets = $derived(allFacets.filter((facet) => 0 < facet.values.length));

	const alreadySaved = $derived(preferences.value.files.savedSearches.some((saved) => saved.query === submitted.trim()));
	const saveReason = $derived(
		alreadySaved ? t('search.save.reason_saved') : 0 === submitted.trim().length ? t('search.save.reason_empty') : undefined
	);

	function saveSearch(): void {
		const trimmed = submitted.trim();
		preferences.update({
			files: { savedSearches: [...preferences.value.files.savedSearches, { name: trimmed, query: trimmed }] }
		});
	}

	function updateUrl(value: string): void {
		const url = new URL(page.url);
		if ('' === value) {
			url.searchParams.delete('q');
		} else {
			url.searchParams.set('q', value);
		}
		void goto(url, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function submit(): void {
		submitted = text.trim();
		updateUrl(submitted);
	}

	function clearFilters(): void {
		include = [];
		text = '';
		submit();
	}

	function openEntry(entry: FileEntry): void {
		void goto(`${base}/files${parentPath(entry.path)}?inspect=${encodeURIComponent(entry.path)}`);
	}

	function onKeyDown(event: KeyboardEvent): void {
		if (!(event.metaKey || event.ctrlKey) || 'Enter' !== event.key) {
			return;
		}
		if (!(event.target instanceof HTMLElement)) {
			return;
		}
		const row = event.target.closest<HTMLTableRowElement>('tr[data-row-index]');
		if (null === row) {
			return;
		}
		const entry = items[Number(row.dataset.rowIndex)];
		if (undefined !== entry) {
			openEntry(entry);
		}
	}

	const resultHeader = $derived(
		undefined === results.data
			? ''
			: truncated
				? t('search.result.truncated')
				: t('search.result.count', {
						count: results.data.items.length,
						seconds: formatFixed(results.data.took_ms / 1000, 1, defaultLocale())
					})
	);

	const columns = $derived<ColumnDef<FileEntry>[]>([
		{ id: 'path', header: t('search.column.path'), cell: pathCell, priority: 1 },
		{ id: 'size', header: t('search.column.size'), cell: sizeCell, align: 'end', priority: 1 },
		{ id: 'modified', header: t('search.column.modified'), cell: modifiedCell, priority: 1 },
		{ id: 'owner', header: t('search.column.owner'), cell: ownerCell },
		{ id: 'storage', header: t('search.column.storage'), cell: storageCell }
	]);

	const EXAMPLES = [
		{ query: 'name:*.log', key: 'files-screens.search.example.name' },
		{ query: 'size:>100MB', key: 'files-screens.search.example.size' },
		{ query: 'modified:<7d', key: 'files-screens.search.example.modified' },
		{ query: 'type:file', key: 'files-screens.search.example.type' }
	] as const;

	function runExample(query: string): void {
		text = query;
		submit();
	}

	const highlight = $derived(parsed.query.name ?? parsed.query.text ?? '');
	const rowKey = (entry: FileEntry): string => entry.path;
</script>

{#snippet pathCell(entry: FileEntry)}
	<PathCell path={entry.path} href={`${base}/files${entry.path}`} {highlight} />
{/snippet}

{#snippet sizeCell(entry: FileEntry)}
	<BytesCell value={entry.size} />
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

{#snippet tips()}
	<ul class="tips" aria-label={t('files-screens.search.examples')}>
		{#each EXAMPLES as example (example.query)}
			<li>
				<button type="button" class="tip" onclick={() => runExample(example.query)}>
					<code class="tip-query">{example.query}</code>
					<span class="tip-text">{t(example.key)}</span>
				</button>
			</li>
		{/each}
	</ul>
{/snippet}

{#snippet empty()}
	<EmptyState
		variant="no-results"
		icon="search"
		title={t('search.empty.title', { query: submitted })}
		body={t('search.empty.body')}
		primary={{ label: t('search.empty.clear'), onclick: clearFilters }}
	/>
{/snippet}

<PageFrame title={t('search.title')} job={t('files-screens.search.job')} class={className} onkeydown={onKeyDown}>
	<div class="search">
		<div class="facets-toggle">
			<Button
				variant="secondary"
				iconStart="filter"
				aria-expanded={filtersOpen}
				aria-controls="search-facets"
				onclick={() => (filtersOpen = !filtersOpen)}
			>
				{0 < include.length ? t('files-screens.search.filters_count', { count: include.length }) : t('files-screens.search.filters')}
			</Button>
		</div>
		<div id="search-facets" class={['facets', !filtersOpen && 'collapsed']}>
			<FacetSidebar {facets} bind:include label={t('search.facets.label')} />
		</div>

		<div class="query">
			<QueryBar bind:value={text} grammar={queryGrammar.grammar} label={t('search.query.label')} placeholder={t('search.query.placeholder')} onsubmit={submit} />
		</div>

		<Stack gap="4" class="search-results">
			{#if hasQuery}
				<Cluster gap="2" justify="space-between" align="center" wrap>
					<Text weight={500} tabular>{resultHeader}</Text>
					<Button variant="secondary" size="sm" disabledReason={saveReason} onclick={saveSearch}>
						{t('search.save')}
					</Button>
				</Cluster>

				{#if truncated}
					<InlineAlert tone="warn" title={t('search.result.partial_title')} body={t('search.result.truncated_body')} />
				{/if}

				{#if null !== moreResults.error}
					<InlineAlert tone="danger" title={moreResults.error.title} body={moreResults.error.body} />
				{/if}

				<DataTable
					rows={items}
					{columns}
					{rowKey}
					label={t('search.table.label')}
					status={results.status}
					error={results.error ?? undefined}
					{hasMore}
					onloadmore={loadMore}
					onopen={openEntry}
					empty={empty}
				/>
			{:else}
				<EmptyState
					variant="first-use"
					icon="search"
					title={t('search.tips.title')}
					body={t('search.tips.body')}
					example={tips}
				/>
			{/if}
		</Stack>
	</div>
</PageFrame>

<style>
	.query {
		grid-column: 2;
		grid-row: 1;
		inline-size: 100%;
		min-inline-size: 0;
	}

	.search :global(.search-results) {
		grid-column: 2;
		grid-row: 2;
		min-inline-size: 0;
	}

	.facets-toggle {
		display: none;
	}

	.facets {
		grid-column: 1;
		grid-row: 1 / span 2;
		position: sticky;
		inset-block-start: var(--space-4);
		padding: var(--space-4);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
	}

	.search {
		display: grid;
		grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
		grid-template-rows: auto 1fr;
		align-items: start;
		gap: var(--space-6);
	}

	@media (width < 840px) {
		.search {
			grid-template-columns: minmax(0, 1fr);
			grid-template-rows: none;
			gap: var(--space-4);
		}

		.query,
		.facets,
		.facets-toggle,
		.search :global(.search-results) {
			grid-column: 1;
			grid-row: auto;
		}

		.facets {
			position: static;
		}

		.query {
			order: 1;
		}

		.facets-toggle {
			order: 2;
		}

		.facets {
			order: 3;
		}

		.search :global(.search-results) {
			order: 4;
		}

		.facets.collapsed {
			display: none;
		}

		.facets-toggle {
			display: flex;
		}
	}

	.tips {
		display: grid;
		gap: var(--space-2);
		inline-size: min(100%, var(--dialog-md));
		padding: 0;
		list-style: none;
		text-align: start;
	}

	.tip {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		inline-size: 100%;
		min-block-size: var(--control-h);
		padding: var(--space-2) var(--space-3);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface-raised);
		color: var(--text);
		font: inherit;
		text-align: start;
		cursor: pointer;
		transition: border-color var(--dur-micro) var(--ease-out), background-color var(--dur-micro) var(--ease-out);
	}

	@media (hover: hover) {
		.tip:hover {
			border-color: var(--border-strong);
			background: var(--surface-hover);
		}
	}

	.tip:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.tip-query {
		flex: none;
		min-inline-size: calc(var(--space-16) * 2);
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		color: var(--text-link);
	}

	.tip-text {
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}
</style>
