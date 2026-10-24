<script lang="ts" module>
	export const TOKENS = ['--hex-viewer-bg', '--hex-viewer-h', '--hex-offset-w', '--hex-bytes-w'] as const;

	const BYTES_PER_ROW = 16;
	const HEX_OFFSET_PATTERN = /^0x[0-9a-f]+$/i;
	const DECIMAL_OFFSET_PATTERN = /^\d+$/;

	interface HexRow {
		offset: number;
		offsetText: string;
		ready: boolean;
		hex: string;
		ascii: string;
	}

	const toHex = (value: number, width: number): string => value.toString(16).toUpperCase().padStart(width, '0');

	const toAscii = (byte: number): string => (0x20 <= byte && byte <= 0x7e ? String.fromCharCode(byte) : '.');

	const parseOffset = (text: string): number | null => {
		const trimmed = text.trim();
		let parsed = Number.NaN;
		if (true === HEX_OFFSET_PATTERN.test(trimmed)) {
			parsed = Number.parseInt(trimmed.slice(2), 16);
		} else if (true === DECIMAL_OFFSET_PATTERN.test(trimmed)) {
			parsed = Number.parseInt(trimmed, 10);
		}
		return Number.isSafeInteger(parsed) ? parsed : null;
	};

	const describeRow = (offset: number, count: number, chunk: Uint8Array | undefined): HexRow => {
		const offsetText = toHex(offset, 8);
		const start = offset % VIEWER_CHUNK_BYTES;
		if (undefined === chunk || chunk.length < start + count) {
			return { offset, offsetText, ready: false, hex: '', ascii: '' };
		}
		const bytes = chunk.subarray(start, start + count);
		const cells = Array.from(bytes, (byte) => toHex(byte, 2));
		const hex = Array.from({ length: BYTES_PER_ROW }, (_, index) => cells[index] ?? '  ').join(' ');
		const ascii = Array.from(bytes, toAscii).join('');
		return { offset, offsetText, ready: true, hex, ascii };
	};
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Button from '../../atoms/Button.svelte';
	import FieldError from '../../atoms/FieldError.svelte';
	import Input from '../../atoms/Input.svelte';
	import Label from '../../atoms/Label.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Text from '../../atoms/Text.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Spinner from '../../quarks/Spinner.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import type { QueryStatus, UiError } from '../../types';
	import {
		VIEWER_CHUNK_BYTES,
		VIEWER_FALLBACK_LINE_PX,
		VIEWER_OVERSCAN_ROWS,
		fetchRange,
		measureLinePx,
		type RangeLoader
	} from './TextViewer.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		size: number;
		loadRange: RangeLoader;
		loaded?: number;
	}

	let { size, loadRange, loaded = $bindable(0), class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const offsetId = `${uid}-offset`;
	const errorId = `${uid}-offset-error`;

	let viewport: HTMLDivElement | undefined = $state();
	let chunks = $state.raw<Record<number, Uint8Array>>({});
	let failed = $state(false);
	let scrollTop = $state(0);
	let viewportHeight = $state(0);
	let rowPx = $state(VIEWER_FALLBACK_LINE_PX);
	let offsetText = $state('');
	let offsetError = $state<string | null>(null);
	let generation = 0;
	let source: { size: number; loadRange: RangeLoader } | undefined;
	const requested = new Set<number>();

	const totalRows = $derived(Math.ceil(size / BYTES_PER_ROW));
	const rowRange = $derived.by(() => {
		const first = Math.max(0, Math.floor(scrollTop / rowPx) - VIEWER_OVERSCAN_ROWS);
		const end = Math.min(totalRows, first + Math.ceil(viewportHeight / rowPx) + VIEWER_OVERSCAN_ROWS * 2);
		return { first, end };
	});
	const firstChunk = $derived(Math.floor((rowRange.first * BYTES_PER_ROW) / VIEWER_CHUNK_BYTES));
	const lastChunk = $derived(Math.floor((Math.max(0, rowRange.end - 1) * BYTES_PER_ROW) / VIEWER_CHUNK_BYTES));
	const rows = $derived.by((): HexRow[] => {
		const built: HexRow[] = [];
		for (let row = rowRange.first; row < rowRange.end; row += 1) {
			const offset = row * BYTES_PER_ROW;
			const chunk: Uint8Array | undefined = chunks[Math.floor(offset / VIEWER_CHUNK_BYTES)];
			built.push(describeRow(offset, Math.min(BYTES_PER_ROW, size - offset), chunk));
		}
		return built;
	});
	const loadedBytes = $derived(Object.values(chunks).reduce((sum, bytes) => sum + bytes.length, 0));
	const status: QueryStatus = $derived(
		0 === size ? 'empty' : 0 === loadedBytes && !failed ? 'loading' : loadedBytes < size ? 'partial' : 'success'
	);
	const pendingRows = $derived(rows.some((row) => !row.ready));
	const topPx = $derived(rowRange.first * rowPx);
	const bottomPx = $derived(Math.max(0, totalRows - rowRange.end) * rowPx);
	const loadError: UiError = $derived({
		title: t('viewer.load_failed.title'),
		body: t('viewer.load_failed.body'),
		retryable: true
	});

	const requestChunk = async (index: number): Promise<void> => {
		const current = generation;
		const from = index * VIEWER_CHUNK_BYTES;
		const to = Math.min(size, from + VIEWER_CHUNK_BYTES);
		requested.add(index);
		const buffer = await fetchRange(loadRange, from, to);
		if (current !== generation) return;
		if (null === buffer) {
			requested.delete(index);
			failed = true;
			return;
		}
		chunks = { ...chunks, [index]: new Uint8Array(buffer) };
	};

	const requestMissing = (first: number, last: number): void => {
		for (let index = first; index <= last; index += 1) {
			if (!(index in chunks) && !requested.has(index)) {
				void requestChunk(index);
			}
		}
	};

	const retry = (): void => {
		failed = false;
		requestMissing(firstChunk, lastChunk);
	};

	const onScroll = (): void => {
		if (viewport) scrollTop = viewport.scrollTop;
	};

	const goToOffset = (): void => {
		const parsed = parseOffset(offsetText);
		if (null === parsed || size <= parsed) {
			offsetError = t('viewer.hex.offset_invalid', { last: formatNumber(Math.max(0, size - 1), 'int') });
			return;
		}
		offsetError = null;
		const target = Math.floor(parsed / BYTES_PER_ROW) * rowPx;
		scrollTop = target;
		if (viewport) viewport.scrollTop = target;
	};

	const onOffsetKeydown = (event: KeyboardEvent): void => {
		if ('Enter' !== event.key) return;
		event.preventDefault();
		goToOffset();
	};

	$effect(() => {
		loaded = loadedBytes;
	});

	$effect(() => {
		if (viewport) rowPx = measureLinePx(viewport, VIEWER_FALLBACK_LINE_PX);
	});

	$effect(() => {
		const total = size;
		const reader = loadRange;
		const first = firstChunk;
		const last = lastChunk;
		untrack(() => {
			if (undefined === source || source.size !== total || source.loadRange !== reader) {
				source = { size: total, loadRange: reader };
				generation += 1;
				chunks = {};
				requested.clear();
				failed = false;
			}
			if (0 < total) requestMissing(first, last);
		});
	});
</script>

<div {...rest} class={['hex-viewer', className]} data-state={status}>
	<Cluster gap="2" align="end">
		<Stack gap="1-5">
			<Label for={offsetId}>{t('viewer.hex.offset_label')}</Label>
			<Input
				id={offsetId}
				mono
				inputmode="numeric"
				invalid={null !== offsetError}
				aria-describedby={null === offsetError ? undefined : errorId}
				bind:value={offsetText}
				onkeydown={onOffsetKeydown}
			/>
		</Stack>
		<Button size="sm" variant="secondary" onclick={goToOffset}>{t('viewer.hex.go')}</Button>
	</Cluster>
	{#if null !== offsetError}
		<FieldError id={errorId} message={offsetError} />
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
		<div class="row header" aria-hidden="true">
			<span class="offset">{t('viewer.hex.column_offset')}</span>
			<span class="bytes">{t('viewer.hex.column_bytes')}</span>
			<span class="ascii">{t('viewer.hex.column_ascii')}</span>
		</div>
		<div
			class="viewport"
			bind:this={viewport}
			bind:clientHeight={viewportHeight}
			onscroll={onScroll}
			tabindex="0"
			role="region"
			aria-label={t('viewer.hex.label')}
		>
			<div class="spacer" style:block-size={`${topPx}px`}></div>
			{#each rows as row (row.offset)}
				<div class="row">
					<span class="offset">{row.offsetText}</span>
					<span class="bytes">{row.hex}</span>
					<span class="ascii">{row.ascii}</span>
				</div>
			{/each}
			<div class="spacer" style:block-size={`${bottomPx}px`}></div>
		</div>
		{#if pendingRows}
			<Spinner size="sm" label={t('viewer.load_more')} />
		{/if}
	{/if}
</div>

<style>
	.hex-viewer {
		display: grid;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.viewport {
		block-size: var(--hex-viewer-h, calc(var(--space-16) * 6));
		min-block-size: 0;
		overflow: auto;
		padding-block: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--hex-viewer-bg, var(--surface-sunken));
		color: var(--text);
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.viewport:focus-visible {
		box-shadow: var(--focus-ring);
		outline: var(--focus-width) solid transparent;
	}

	.row {
		display: flex;
		gap: var(--space-4);
		padding-inline: var(--space-3);
		white-space: pre;
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.header {
		color: var(--text-muted);
	}

	.offset {
		flex: none;
		inline-size: var(--hex-offset-w, calc(var(--space-12) * 2));
	}

	.bytes {
		flex: none;
		inline-size: var(--hex-bytes-w, calc(var(--space-16) * 8));
	}

	.ascii {
		min-inline-size: 0;
	}
</style>
