<script lang="ts" module>
	export const TOKENS = ['--file-grid-card-bg', '--file-grid-card-selected-bg'] as const;

	const CARD_MIN_PX = 176;
	const GAP_PX = 16;
	const LABEL_PX = 80;
	const OVERSCAN_ROWS = 4;
</script>

<script lang="ts">
	import { tick } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { FileEntry } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { iconForKind, kindFromName } from '$lib/format/mime';
	import Frame from '../quarks/Frame.svelte';
	import Grid from '../quarks/Grid.svelte';
	import Icon from '../quarks/Icon.svelte';
	import Thumb from '../atoms/Thumb.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import type { IconName, Selection } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children' | 'onopen'> {
		entries: FileEntry[];
		selection?: Selection;
		onopen?: (entry: FileEntry) => void;
		thumbnails: (entry: FileEntry) => string | null;
		class?: string;
	}

	let {
		entries,
		selection = $bindable({ mode: 'multi', ids: new Set<string>(), allMatching: false }),
		onopen,
		thumbnails,
		class: className,
		...rest
	}: Props = $props();

	let scroller = $state<HTMLDivElement | null>(null);
	let width = $state(0);
	let viewport = $state(0);
	let scrollTop = $state(0);
	let cursor = $state(0);

	const cols = $derived(Math.max(1, Math.floor((width + GAP_PX) / (CARD_MIN_PX + GAP_PX))));
	const cardWidth = $derived(Math.max(0, (width - GAP_PX * (cols - 1)) / cols));
	const rowHeight = $derived(Math.round(cardWidth * 0.75) + LABEL_PX + GAP_PX);
	const rowCount = $derived(Math.ceil(entries.length / cols));
	const activeIndex = $derived(Math.min(cursor, Math.max(0, entries.length - 1)));

	const windowRows = $derived.by(() => {
		const first = Math.max(0, Math.floor(scrollTop / rowHeight) - OVERSCAN_ROWS);
		const last = Math.min(rowCount - 1, Math.ceil((scrollTop + viewport) / rowHeight) + OVERSCAN_ROWS);
		const rows: number[] = [];
		for (let row = first; row <= last; row += 1) {
			rows.push(row);
		}
		return rows;
	});

	const cardsInRow = (row: number): { entry: FileEntry; index: number }[] =>
		entries.slice(row * cols, row * cols + cols).map((entry, offset) => ({ entry, index: row * cols + offset }));

	const iconFor = (entry: FileEntry): IconName =>
		'directory' === entry.type ? 'folder' : iconForKind(kindFromName(entry.name));

	const handleScroll = (event: Event): void => {
		const element = event.currentTarget;
		if (element instanceof HTMLElement) {
			scrollTop = element.scrollTop;
		}
	};

	const selectAll = (): void => {
		if ('multi' !== selection.mode) {
			return;
		}
		selection = { ...selection, ids: new Set(entries.map((entry) => entry.path)), allMatching: false };
	};

	const clearSelection = (): void => {
		selection = { ...selection, ids: new Set<string>(), allMatching: false };
	};

	const toggleAt = (index: number): void => {
		const entry = entries[index];
		if (undefined === entry || 'none' === selection.mode) {
			return;
		}
		const ids = 'single' === selection.mode ? new Set<string>() : new Set(selection.ids);
		if (selection.ids.has(entry.path)) {
			ids.delete(entry.path);
		} else {
			ids.add(entry.path);
		}
		selection = { ...selection, ids, allMatching: false };
	};

	const selectOnly = (entry: FileEntry, index: number): void => {
		cursor = index;
		if ('none' === selection.mode) {
			return;
		}
		selection = { ...selection, ids: new Set([entry.path]), allMatching: false };
	};

	const extendTo = (from: number, to: number): void => {
		if ('multi' !== selection.mode) {
			return;
		}
		const ids = new Set(selection.ids);
		for (let index = Math.min(from, to); index <= Math.max(from, to); index += 1) {
			const entry = entries[index];
			if (undefined !== entry) {
				ids.add(entry.path);
			}
		}
		selection = { ...selection, ids, allMatching: false };
	};

	const openAt = (index: number): void => {
		const entry = entries[index];
		if (undefined !== entry) {
			onopen?.(entry);
		}
	};

	const activate = async (index: number): Promise<void> => {
		cursor = index;
		const top = Math.floor(index / cols) * rowHeight;
		const bottom = top + rowHeight;
		if (scroller) {
			if (scroller.scrollTop > top) {
				scroller.scrollTop = top;
			} else if (scroller.scrollTop + scroller.clientHeight < bottom) {
				scroller.scrollTop = bottom - scroller.clientHeight;
			}
			scrollTop = scroller.scrollTop;
		}
		await tick();
		scroller?.querySelector<HTMLElement>(`[data-index="${index}"]`)?.focus();
	};

	const stepFor = (key: string): number | null => {
		switch (key) {
			case 'ArrowRight':
				return 1;
			case 'ArrowLeft':
				return -1;
			case 'ArrowDown':
			case 'j':
				return cols;
			case 'ArrowUp':
			case 'k':
				return -cols;
			default:
				return null;
		}
	};

	const handleKeydown = (event: KeyboardEvent): void => {
		if (0 === entries.length) {
			return;
		}
		const key = 1 === event.key.length ? event.key.toLowerCase() : event.key;
		if ((event.metaKey || event.ctrlKey) && 'a' === key) {
			event.preventDefault();
			selectAll();
			return;
		}
		if ('Escape' === key) {
			clearSelection();
			return;
		}
		if ('Enter' === key || 'o' === key) {
			event.preventDefault();
			openAt(activeIndex);
			return;
		}
		if ('x' === key || ' ' === key) {
			event.preventDefault();
			toggleAt(activeIndex);
			return;
		}
		const last = entries.length - 1;
		const step = stepFor(key);
		let destination: number | null = null;
		if ('Home' === key) {
			destination = 0;
		} else if ('End' === key) {
			destination = last;
		} else if (null !== step) {
			destination = Math.min(last, Math.max(0, activeIndex + step));
		}
		if (null === destination) {
			return;
		}
		event.preventDefault();
		if (event.shiftKey) {
			extendTo(activeIndex, destination);
		}
		void activate(destination);
	};
</script>

<div class={['file-grid', className]} {...rest}>
	{#if 0 === entries.length}
		<EmptyState
			variant="first-use"
			icon="folder"
			title={t('files.table.empty_title')}
			body={t('files.table.empty_body')}
		/>
	{:else}
		<div
			class="grid-scroll"
			role="listbox"
			aria-label={t('files.grid.label')}
			aria-multiselectable={'multi' === selection.mode}
			bind:this={scroller}
			bind:clientWidth={width}
			bind:clientHeight={viewport}
			onscroll={handleScroll}
			onkeydown={handleKeydown}
		>
			<div class="grid-canvas" style:height={`${rowCount * rowHeight}px`}>
				{#each windowRows as row (row)}
					<div class="grid-row" role="presentation" style:top={`${row * rowHeight}px`} style:height={`${rowHeight}px`}>
						<Grid min="16" gap="4" {cols}>
							{#each cardsInRow(row) as card (card.entry.path)}
								{@const selected = selection.ids.has(card.entry.path)}
								{@const thumbnail = thumbnails(card.entry)}
								<div
									class="card"
									role="option"
									tabindex={card.index === activeIndex ? 0 : -1}
									aria-selected={selected}
									data-index={card.index}
									data-entry-path={card.entry.path}
									data-state={selected ? 'selected' : undefined}
									title={card.entry.name}
									onclick={() => selectOnly(card.entry, card.index)}
									ondblclick={() => onopen?.(card.entry)}
									onfocus={() => {
										cursor = card.index;
									}}
								>
									<Frame ratio="4/3">
										{#if null === thumbnail}
											<span class="glyph">
												<Icon name={iconFor(card.entry)} size="xl" />
											</span>
										{:else}
											<Thumb src={thumbnail} alt={card.entry.name} ratio="4/3" />
										{/if}
									</Frame>
									<span class="label">{card.entry.name}</span>
								</div>
							{/each}
						</Grid>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>

<style>
	.file-grid {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
		min-block-size: 0;
		block-size: 100%;
	}

	.grid-scroll {
		position: relative;
		flex: 1 1 auto;
		min-block-size: 0;
		overflow-y: auto;
		contain: strict;
	}

	.grid-canvas {
		position: relative;
		inline-size: 100%;
	}

	.grid-row {
		position: absolute;
		inset-inline: 0;
		padding-block-end: var(--space-4);
		box-sizing: border-box;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: 0;
		padding: var(--space-2);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--file-grid-card-bg, var(--surface-raised));
		color: var(--text);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	@media (hover: hover) {
		.card:hover {
			background: var(--surface-hover);
		}
	}

	.card[data-state='selected'] {
		background: var(--file-grid-card-selected-bg, var(--accent-soft));
		border-color: var(--accent);
	}

	.card:active {
		background: var(--surface-selected);
	}

	.card:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.glyph {
		display: flex;
		align-items: center;
		justify-content: center;
		block-size: 100%;
		inline-size: 100%;
		border-radius: var(--radius-sm);
		background: var(--surface-sunken);
		color: var(--text-muted);
	}

	.label {
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
