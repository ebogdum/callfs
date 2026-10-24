<script lang="ts" module>
	export type RangeLoader = (from: number, to: number) => Promise<ArrayBuffer>;

	export const VIEWER_CHUNK_BYTES = 262144;
	export const VIEWER_PRETTY_MAX_BYTES = 5242880;
	export const VIEWER_TAIL_INTERVAL_MS = 2000;
	export const VIEWER_OVERSCAN_ROWS = 8;
	export const VIEWER_FALLBACK_LINE_PX = 20;

	export const TOKENS = [
		'--text-viewer-bg',
		'--text-viewer-match',
		'--text-viewer-current',
		'--text-viewer-h'
	] as const;

	export const fetchRange = async (loadRange: RangeLoader, from: number, to: number): Promise<ArrayBuffer | null> => {
		try {
			return await loadRange(from, to);
		} catch {
			return null;
		}
	};

	export const measureLinePx = (element: HTMLElement, fallback: number): number => {
		const px = Number.parseFloat(getComputedStyle(element).lineHeight);
		return Number.isFinite(px) && 0 < px ? px : fallback;
	};

	const stripCarriage = (line: string): string => line.replace(/\r$/, '');

	const prettyPrint = (text: string): string | null => {
		try {
			return JSON.stringify(JSON.parse(text), null, 2);
		} catch {
			return null;
		}
	};
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Button from '../../atoms/Button.svelte';
	import Input from '../../atoms/Input.svelte';
	import Label from '../../atoms/Label.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Text from '../../atoms/Text.svelte';
	import ToggleButton from '../../atoms/ToggleButton.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Spinner from '../../quarks/Spinner.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import type { QueryStatus, UiError } from '../../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		size: number;
		loadRange: RangeLoader;
		loaded?: number;
		wrap?: boolean;
		tail?: boolean;
		search?: string;
	}

	let {
		size,
		loadRange,
		loaded = $bindable(0),
		wrap = false,
		tail = false,
		search,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const findId = `${uid}-find`;

	let root: HTMLDivElement | undefined = $state();
	let viewport: HTMLDivElement | undefined = $state();
	let lines = $state.raw<string[]>([]);
	let loadedBytes = $state(0);
	let failed = $state(false);
	let pending = $state(false);
	let following = $state(true);
	let pretty = $state(false);
	let prettyLines = $state.raw<string[]>([]);
	let prettyStatus = $state<'idle' | 'loading' | 'ready' | 'invalid'>('idle');
	let query = $state(untrack(() => search ?? ''));
	let matchIndex = $state(0);
	let scrollTop = $state(0);
	let viewportHeight = $state(0);
	let rowPx = $state(VIEWER_FALLBACK_LINE_PX);
	let generation = 0;
	let remainder = '';
	let decoder = new TextDecoder();
	let tailBusy = false;

	const shown = $derived(pretty && 'ready' === prettyStatus ? prettyLines : lines);
	const status: QueryStatus = $derived(
		0 === size ? 'empty' : 0 === lines.length && !failed ? 'loading' : loadedBytes < size ? 'partial' : 'success'
	);
	const canPretty = $derived(!tail && size <= VIEWER_PRETTY_MAX_BYTES);
	const matches = $derived.by((): number[] => {
		const needle = query.toLowerCase();
		if ('' === needle) return [];
		return shown.flatMap((line, index) => (line.toLowerCase().includes(needle) ? [index] : []));
	});
	const activeMatch = $derived(0 === matches.length ? -1 : (matches[matchIndex % matches.length] ?? -1));
	const matchSummary = $derived(
		'' === query ? '' : 0 === matches.length ? t('viewer.text.no_matches') : t('viewer.text.matches', { count: matches.length })
	);
	const range = $derived.by(() => {
		const first = Math.max(0, Math.floor(scrollTop / rowPx) - VIEWER_OVERSCAN_ROWS);
		const end = Math.min(shown.length, first + Math.ceil(viewportHeight / rowPx) + VIEWER_OVERSCAN_ROWS * 2);
		return { first, end };
	});
	const rows = $derived(
		shown.slice(range.first, range.end).map((text, offset) => {
			const index = range.first + offset;
			const kind = index === activeMatch ? 'current' : matches.includes(index) ? 'match' : 'plain';
			return { index, text, kind };
		})
	);
	const topPx = $derived(range.first * rowPx);
	const bottomPx = $derived(Math.max(0, shown.length - range.end) * rowPx);
	const loadError: UiError = $derived({
		title: t('viewer.load_failed.title'),
		body: t('viewer.load_failed.body'),
		retryable: true
	});

	const appendText = (text: string, final: boolean): void => {
		const parts = (remainder + text).split('\n');
		remainder = parts.pop() ?? '';
		if (final && '' !== remainder) {
			parts.push(remainder);
			remainder = '';
		}
		lines = [...lines, ...parts.map(stripCarriage)];
	};

	const loadNext = async (): Promise<void> => {
		if (pending || failed || loadedBytes >= size) return;
		const current = generation;
		const from = loadedBytes;
		const to = Math.min(size, from + VIEWER_CHUNK_BYTES);
		pending = true;
		const buffer = await fetchRange(loadRange, from, to);
		if (current !== generation) return;
		pending = false;
		if (null === buffer) {
			failed = true;
			return;
		}
		loadedBytes = to;
		appendText(decoder.decode(buffer, { stream: to < size }), to >= size);
	};

	const restart = (): void => {
		generation += 1;
		decoder = new TextDecoder();
		remainder = '';
		lines = [];
		loadedBytes = 0;
		failed = false;
		pending = false;
		prettyStatus = 'idle';
		prettyLines = [];
		void loadNext();
	};

	const tailTick = async (): Promise<void> => {
		if (tailBusy) return;
		tailBusy = true;
		const current = generation;
		const total = size;
		const from = Math.max(0, total - VIEWER_CHUNK_BYTES);
		const buffer = await fetchRange(loadRange, from, total);
		tailBusy = false;
		if (current !== generation) return;
		if (null === buffer) {
			failed = true;
			return;
		}
		const parts = new TextDecoder().decode(buffer).split('\n').map(stripCarriage);
		if (0 < from) parts.shift();
		if ('' === parts[parts.length - 1]) parts.pop();
		lines = parts;
		loadedBytes = total;
		if (viewport) viewport.scrollTop = viewport.scrollHeight;
	};

	const loadPretty = async (): Promise<void> => {
		const current = generation;
		prettyStatus = 'loading';
		const buffer = await fetchRange(loadRange, 0, size);
		if (current !== generation) return;
		if (null === buffer) {
			failed = true;
			pretty = false;
			prettyStatus = 'idle';
			return;
		}
		const formatted = prettyPrint(new TextDecoder().decode(buffer));
		if (null === formatted) {
			prettyStatus = 'invalid';
			return;
		}
		prettyLines = formatted.split('\n');
		prettyStatus = 'ready';
	};

	const step = (delta: number): void => {
		if (0 === matches.length) return;
		matchIndex = (matchIndex + delta + matches.length) % matches.length;
	};

	const retry = (): void => {
		failed = false;
		if (!tail) void loadNext();
	};

	const onFindInput = (): void => {
		matchIndex = 0;
	};

	const onFindKeydown = (event: KeyboardEvent): void => {
		if ('Enter' !== event.key) return;
		event.preventDefault();
		step(event.shiftKey ? -1 : 1);
	};

	const onKeydown = (event: KeyboardEvent): void => {
		if ('f' !== event.key.toLowerCase() || !(event.metaKey || event.ctrlKey)) return;
		event.preventDefault();
		root?.querySelector<HTMLInputElement>('input')?.focus();
	};

	const onScroll = (): void => {
		if (!viewport) return;
		scrollTop = viewport.scrollTop;
		if (viewport.scrollTop + viewport.clientHeight + rowPx * VIEWER_OVERSCAN_ROWS >= lines.length * rowPx) {
			void loadNext();
		}
	};

	$effect(() => {
		loaded = loadedBytes;
	});

	$effect(() => {
		if (viewport) rowPx = measureLinePx(viewport, VIEWER_FALLBACK_LINE_PX);
	});

	$effect(() => {
		const source = { size, loadRange, tail };
		if (!source.tail) {
			untrack(() => {
				restart();
			});
		}
	});

	$effect(() => {
		if (!tail && lines.length * rowPx < viewportHeight) {
			untrack(() => {
				void loadNext();
			});
		}
	});

	$effect(() => {
		if (!tail || !following) return;
		untrack(() => {
			void tailTick();
		});
		const id = setInterval(() => {
			void tailTick();
		}, VIEWER_TAIL_INTERVAL_MS);
		return () => {
			clearInterval(id);
		};
	});

	$effect(() => {
		if (pretty && 'idle' === prettyStatus) {
			untrack(() => {
				void loadPretty();
			});
		}
	});

	$effect(() => {
		const target = activeMatch;
		if (viewport && 0 <= target) viewport.scrollTop = target * rowPx;
	});
</script>

<div
	{...rest}
	bind:this={root}
	class={['text-viewer', className]}
	data-state={status}
	data-wrap={wrap ? 'true' : 'false'}
	onkeydown={onKeydown}
>
	<Cluster gap="2" align="end">
		<Stack gap="1-5">
			<Label for={findId}>{t('viewer.text.find')}</Label>
			<Input id={findId} type="search" bind:value={query} oninput={onFindInput} onkeydown={onFindKeydown} />
		</Stack>
		<Button
			size="sm"
			variant="ghost"
			disabledReason={0 === matches.length ? t('viewer.text.no_matches') : undefined}
			onclick={() => step(-1)}>{t('viewer.text.find_previous')}</Button
		>
		<Button
			size="sm"
			variant="ghost"
			disabledReason={0 === matches.length ? t('viewer.text.no_matches') : undefined}
			onclick={() => step(1)}>{t('viewer.text.find_next')}</Button
		>
		<Text size="sm" tone="muted" aria-live="polite">{matchSummary}</Text>
		{#if tail}
			<ToggleButton bind:pressed={following} label={t('viewer.text.follow')} />
		{/if}
		{#if canPretty}
			<ToggleButton bind:pressed={pretty} label={t('viewer.text.pretty')} />
		{/if}
	</Cluster>
	{#if pretty && 'invalid' === prettyStatus}
		<Text size="sm" tone="warn">{t('viewer.text.json_invalid')}</Text>
	{/if}
	{#if failed}
		<ErrorState error={loadError} onretry={retry} />
	{/if}
	{#if 'empty' === status}
		<Text tone="muted">{t('viewer.empty')}</Text>
	{:else if 'loading' === status}
		<Stack gap="2">
			<Skeleton lines={6} />
			<Spinner label={t('common.loading_label')} />
		</Stack>
	{:else}
		<div
			class="viewport"
			bind:this={viewport}
			bind:clientHeight={viewportHeight}
			onscroll={onScroll}
			tabindex="0"
			role="region"
			aria-label={t('viewer.text.label')}
		>
			<div class="spacer" style:block-size={`${topPx}px`}></div>
			{#each rows as row (row.index)}
				<div class="row" data-match={row.kind}>
					<span class="gutter" aria-hidden="true">{formatNumber(row.index + 1, 'int')}</span>
					<span class="content">{row.text}</span>
				</div>
			{/each}
			<div class="spacer" style:block-size={`${bottomPx}px`}></div>
		</div>
		{#if pending}
			<Spinner size="sm" label={t('viewer.load_more')} />
		{/if}
	{/if}
</div>

<style>
	.text-viewer {
		display: grid;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.viewport {
		block-size: var(--text-viewer-h, calc(var(--space-16) * 6));
		min-block-size: 0;
		overflow: auto;
		padding-block: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--text-viewer-bg, var(--surface-sunken));
		color: var(--text);
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		tab-size: 4;
	}

	.viewport:focus-visible {
		box-shadow: var(--focus-ring);
		outline: var(--focus-width) solid transparent;
	}

	.row {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		padding-inline: var(--space-3);
		white-space: pre;
	}

	.text-viewer[data-wrap='true'] .content {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}

	.gutter {
		flex: none;
		min-inline-size: var(--space-10);
		text-align: end;
		color: var(--text-faint);
		user-select: none;
	}

	.content {
		min-inline-size: 0;
	}

	.row[data-match='match'] {
		background: var(--text-viewer-match, var(--accent-soft));
	}

	.row[data-match='current'] {
		background: var(--text-viewer-current, var(--accent-soft-hover));
	}
</style>
