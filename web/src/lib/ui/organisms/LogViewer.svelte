<script lang="ts" module>
	export interface LogLine {
		id: string;
		at: string;
		level: 'debug' | 'info' | 'warn' | 'error';
		message: string;
		logger: string;
		fields: Record<string, unknown>;
		request_id: string | null;
	}

	export type LogLevel = LogLine['level'];

	export const LOG_LEVELS: readonly LogLevel[] = ['error', 'warn', 'info', 'debug'];
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { QueryStatus, Tone } from '../types';
	import { t } from '$lib/copy/t';
	import { DASH, defaultLocale, formatNumber } from '$lib/format/number';
	import { formatTimestamp } from '$lib/format/time';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Input from '../atoms/Input.svelte';
	import Skeleton from '../atoms/Skeleton.svelte';
	import ToggleButton from '../atoms/ToggleButton.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import DescriptionList from '../molecules/DescriptionList.svelte';
	import type { DescriptionItem } from '../molecules/DescriptionList.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		entries: LogLine[];
		status: QueryStatus;
		live?: boolean;
		level?: string[];
		query?: string;
		onloadolder?: () => void;
		class?: string;
	}

	let {
		entries,
		status,
		live = $bindable(false),
		level = $bindable([]),
		query = $bindable(''),
		onloadolder,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const searchId = `${uid}-search`;
	const ROW_HEIGHT = 40;
	const OVERSCAN = 8;
	const DEFAULT_VIEWPORT_HEIGHT = 640;
	const LEVEL_TONE: Record<LogLevel, Tone | 'muted'> = {
		error: 'danger',
		warn: 'warn',
		info: 'info',
		debug: 'muted',
	};

	let viewport: HTMLDivElement | undefined = $state();
	let scrollTop = $state(0);
	let viewportHeight = $state(DEFAULT_VIEWPORT_HEIGHT);
	let pinnedToLatest = $state(true);
	let seenCount = $state(0);
	let expandedIds: Set<string> = $state(new Set());

	const clockFormat = new Intl.DateTimeFormat(defaultLocale(), { hour: 'numeric', minute: '2-digit', second: '2-digit' });
	const shortTime = (at: string): string => {
		const stamp = new Date(at);
		return Number.isNaN(stamp.getTime()) ? DASH : clockFormat.format(stamp);
	};
	const timeOf = (at: string): number => new Date(at).getTime() || 0;
	const newestFirst = $derived(2 > entries.length || timeOf(entries[0]?.at ?? '') >= timeOf(entries.at(-1)?.at ?? ''));
	const latestEdge = (element: HTMLDivElement): number => (newestFirst ? 0 : element.scrollHeight);

	const startIndex = $derived(
		Math.min(entries.length, Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN))
	);
	const endIndex = $derived(
		Math.min(entries.length, Math.max(startIndex, Math.ceil((scrollTop + viewportHeight) / ROW_HEIGHT) + OVERSCAN))
	);
	const renderedEntries = $derived(entries.slice(startIndex, endIndex));
	const leadingSpace = $derived(startIndex * ROW_HEIGHT);
	const trailingSpace = $derived((entries.length - endIndex) * ROW_HEIGHT);
	const newCount = $derived(pinnedToLatest ? 0 : Math.max(0, entries.length - seenCount));
	const showSkeleton = $derived('loading' === status && 0 === entries.length);
	const showEmpty = $derived('loading' !== status && 'error' !== status && 0 === entries.length);

	$effect(() => {
		const element = viewport;
		if (undefined === element || 'undefined' === typeof ResizeObserver) {
			return;
		}
		const measure = (): void => {
			if (0 < element.clientHeight) {
				viewportHeight = element.clientHeight;
			}
		};
		measure();
		const observer = new ResizeObserver(measure);
		observer.observe(element);
		return () => observer.disconnect();
	});

	$effect(() => {
		const count = entries.length;
		if (pinnedToLatest && undefined !== viewport && 0 < count && (live || newestFirst)) {
			viewport.scrollTop = latestEdge(viewport);
		}
	});

	$effect(() => {
		const count = entries.length;
		if (pinnedToLatest) {
			seenCount = count;
		}
	});

	function handleScroll(): void {
		if (undefined === viewport) {
			return;
		}
		scrollTop = viewport.scrollTop;
		pinnedToLatest = newestFirst
			? viewport.scrollTop <= ROW_HEIGHT
			: viewport.scrollTop + viewport.clientHeight >= viewport.scrollHeight - ROW_HEIGHT;
	}

	function jumpToLatest(): void {
		if (undefined === viewport) {
			return;
		}
		viewport.scrollTop = latestEdge(viewport);
		pinnedToLatest = true;
	}

	function toggleExpanded(id: string): void {
		const next = new Set(expandedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		expandedIds = next;
	}

	function toggleLevel(name: LogLevel, pressed: boolean): void {
		level = pressed ? [...level.filter((item) => item !== name), name] : level.filter((item) => item !== name);
	}

	function describeValue(value: unknown): string {
		if ('string' === typeof value) {
			return value;
		}
		if ('number' === typeof value || 'boolean' === typeof value) {
			return String(value);
		}
		return JSON.stringify(value) ?? DASH;
	}

	function serverOf(entry: LogLine): string {
		const instance = entry.fields.instance_id;
		return 'string' === typeof instance ? instance : DASH;
	}

	function detailItems(entry: LogLine): DescriptionItem[] {
		const items: DescriptionItem[] = [
			{ term: t('u106logs.field.time'), value: formatTimestamp(entry.at) },
			{ term: t('u106logs.field.logger'), value: entry.logger },
		];
		if (null !== entry.request_id) {
			items.push({ term: t('u106logs.field.request_id'), value: entry.request_id, copy: entry.request_id });
		}
		for (const [key, value] of Object.entries(entry.fields)) {
			items.push({ term: key, value: describeValue(value) });
		}
		return items;
	}
</script>

{#snippet jumpButton()}
	<Button variant="secondary" size="sm" onclick={jumpToLatest}>
		{t('u106logs.jump_new', { count: formatNumber(newCount) })}
	</Button>
{/snippet}

<div class={['log-viewer', className]} {...rest}>
	<Cluster gap="3">
		<div class="search">
			<Input
				id={searchId}
				type="search"
				bind:value={query}
				clearable
				placeholder={t('u106logs.search.label')}
				aria-label={t('u106logs.search.label')}
			/>
		</div>
		<Cluster gap="2" role="group" aria-label={t('u106logs.levels.label')}>
			{#each LOG_LEVELS as name (name)}
				<ToggleButton
					label={t(`u106logs.filter.${name}`)}
					bind:pressed={() => level.includes(name), (pressed) => toggleLevel(name, pressed)}
				/>
			{/each}
		</Cluster>
		<ToggleButton label={t('u106logs.live')} bind:pressed={live} />
	</Cluster>

	{#if 'error' === status}
		<InlineAlert tone="danger" title={t('u106logs.error.title')} body={t('u106logs.error.body')} />
	{:else if 'partial' === status}
		<InlineAlert tone="warn" title={t('u106logs.partial.title')} body={t('u106logs.partial.body')}>
			{#snippet actions()}
				{#if undefined !== onloadolder}
					<Button variant="secondary" size="sm" onclick={onloadolder}>{t('u106logs.load_older')}</Button>
				{/if}
			{/snippet}
		</InlineAlert>
	{/if}

	<div
		class="viewport"
		bind:this={viewport}
		role="region"
		tabindex="0"
		aria-label={t('u106logs.label')}
		aria-busy={showSkeleton}
		onscroll={handleScroll}
	>
		{#if 0 < newCount && newestFirst}
			<div class="jump" data-edge="start">{@render jumpButton()}</div>
		{/if}
		{#if showSkeleton}
			<Skeleton shape="text" lines={8} />
		{:else if showEmpty}
			<EmptyState
				variant="no-results"
				icon="logs"
				title={t('u106logs.empty.title')}
				body={t('u106logs.empty.body')}
			/>
		{:else}
			<ol class="rows">
				<li class="spacer" aria-hidden="true" style:block-size="{leadingSpace}px"></li>
				{#each renderedEntries as entry (entry.id)}
					{@const open = expandedIds.has(entry.id)}
					{@const detailId = `${uid}-${entry.id}`}
					<li class="row" data-state={open ? 'open' : 'closed'} style:min-block-size="{ROW_HEIGHT}px">
						<button
							type="button"
							class="line"
							aria-expanded={open}
							aria-controls={open ? detailId : undefined}
							onclick={() => toggleExpanded(entry.id)}
						>
							<time class="time" datetime={entry.at} title={formatTimestamp(entry.at)}>{shortTime(entry.at)}</time>
							<span class="level">
								<StatusGlyph tone={LEVEL_TONE[entry.level]} />
								<span>{t(`u106logs.level.${entry.level}`)}</span>
							</span>
							<span class="message">{entry.message}</span>
							<span class="server">{serverOf(entry)}</span>
						</button>
						{#if open}
							<div class="detail" id={detailId}>
								<DescriptionList items={detailItems(entry)} columns={2} />
								<CopyButton value={JSON.stringify(entry)} label={t('u106logs.copy_json')} variant="button" />
							</div>
						{/if}
					</li>
				{/each}
				<li class="spacer" aria-hidden="true" style:block-size="{trailingSpace}px"></li>
			</ol>
		{/if}
		{#if 0 < newCount && !newestFirst}
			<div class="jump" data-edge="end">{@render jumpButton()}</div>
		{/if}
	</div>
</div>

<style>
	.log-viewer {
		display: grid;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.viewport {
		position: relative;
		block-size: var(--log-viewer-h, calc(var(--viewport-h) * 0.7));
		overflow: auto;
		contain: strict;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface);
	}

	.rows {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.row {
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.line {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-2);
		inline-size: 100%;
		padding-inline: var(--cell-px);
		padding-block: var(--space-1);
		border: 0;
		border-radius: 0;
		background: transparent;
		color: var(--text);
		font: inherit;
		text-align: start;
		cursor: pointer;
	}

	.line:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.line:hover {
			background: var(--surface-hover);
		}
	}

	.line:active {
		background: var(--surface-pressed);
	}

	.time,
	.server {
		font-family: var(--font-mono);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		color: var(--text-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.level {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-caption-size);
		font-weight: 500;
		white-space: nowrap;
	}

	.message {
		min-inline-size: 0;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row[data-state='open'] .message {
		overflow-wrap: anywhere;
		white-space: normal;
	}

	.detail {
		display: grid;
		gap: var(--space-2);
		justify-items: start;
		padding-inline: var(--cell-px);
		padding-block: var(--space-2) var(--space-3);
		background: var(--surface-sunken);
	}

	.search {
		flex: 1 1 calc(var(--space-4) * 16);
		min-inline-size: 0;
		max-inline-size: calc(var(--space-4) * 28);
	}

	.jump[data-edge='start'] {
		z-index: var(--z-sticky);
		inset-block: var(--space-2) auto;
		block-size: 0;
		padding-block: 0;
		overflow: visible;
	}

	.jump {
		position: sticky;
		inset-block-end: var(--space-2);
		display: flex;
		justify-content: center;
		padding-block: var(--space-2);
	}
</style>
