<script lang="ts" module>
	import type { FileKind } from '$lib/format/mime';

	export type PreviewKind = FileKind;

	export interface PreviewStat {
		path: string;
		name: string;
		type: 'file' | 'directory';
		size: number;
	}

	export const TOKENS = ['--preview-bg', '--preview-pad', '--preview-gap', '--preview-pdf-h'] as const;

	const TEXT_KINDS: readonly PreviewKind[] = ['text', 'code', 'json', 'log', 'csv'];
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { kindFromName } from '$lib/format/mime';
	import Button from '../../atoms/Button.svelte';
	import CopyButton from '../../atoms/CopyButton.svelte';
	import Label from '../../atoms/Label.svelte';
	import Link from '../../atoms/Link.svelte';
	import NumberInput from '../../atoms/NumberInput.svelte';
	import Text from '../../atoms/Text.svelte';
	import ToggleButton from '../../atoms/ToggleButton.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import HexViewer from './HexViewer.svelte';
	import MediaViewer from './MediaViewer.svelte';
	import TextViewer, { fetchRange, type RangeLoader } from './TextViewer.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		stat: PreviewStat;
		src: string;
		loadRange: RangeLoader;
		kind?: PreviewKind;
		siblings?: PreviewStat[];
		onnavigate?: (path: string) => void;
		search?: string;
	}

	let { stat, src, loadRange, kind, siblings = [], onnavigate, search, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const fromId = `${uid}-from`;
	const toId = `${uid}-to`;

	let root: HTMLElement | undefined = $state();
	let fullscreen = $state(false);
	let wrap = $state(false);
	let loaded = $state(0);
	let rangeOpen = $state(false);
	let rangeFrom = $state<number | null>(0);
	let rangeTo = $state<number | null>(null);
	let rangeBusy = $state(false);
	let rangeMessage = $state<string | null>(null);

	const resolved: PreviewKind = $derived('directory' === stat.type ? 'archive' : (kind ?? kindFromName(stat.name)));
	const isText = $derived(TEXT_KINDS.includes(resolved));
	const position = $derived(siblings.findIndex((sibling) => sibling.path === stat.path));
	const previous = $derived(0 < position ? siblings[position - 1] : undefined);
	const next = $derived(0 <= position && position < siblings.length - 1 ? siblings[position + 1] : undefined);
	const showsReadout = $derived(isText || 'binary' === resolved);
	const sizeReadout = $derived(
		loaded < stat.size
			? t('viewer.showing', { loaded: formatBytes(loaded).text, total: formatBytes(stat.size).text })
			: t('viewer.showing_all', { total: formatBytes(stat.size).text })
	);

	const toggleFullscreen = (): void => {
		fullscreen = !fullscreen;
		root?.focus();
	};

	const downloadRange = async (): Promise<void> => {
		const from = rangeFrom;
		const to = rangeTo;
		if (null === from || null === to || !Number.isSafeInteger(from) || !Number.isSafeInteger(to) || from < 0 || to > stat.size || from >= to) {
			rangeMessage = t('viewer.range.invalid');
			return;
		}
		rangeMessage = null;
		rangeBusy = true;
		const buffer = await fetchRange(loadRange, from, to);
		rangeBusy = false;
		if (null === buffer) {
			rangeMessage = t('viewer.range.failed');
			return;
		}
		const url = URL.createObjectURL(new Blob([buffer]));
		const anchor = document.createElement('a');
		anchor.href = url;
		anchor.download = `${from}-${to}-${stat.name}`;
		anchor.click();
		setTimeout(() => URL.revokeObjectURL(url), 0);
	};

	const onKeydown = (event: KeyboardEvent): void => {
		if (event.target instanceof HTMLInputElement) return;
		if ('Escape' === event.key && fullscreen) {
			event.preventDefault();
			fullscreen = false;
			root?.focus();
			return;
		}
		if (' ' === event.key && event.target === root && !fullscreen) {
			event.preventDefault();
			fullscreen = true;
			return;
		}
		if (event.target !== root || undefined === onnavigate) return;
		if ('ArrowLeft' === event.key && previous) {
			event.preventDefault();
			onnavigate(previous.path);
		} else if ('ArrowRight' === event.key && next) {
			event.preventDefault();
			onnavigate(next.path);
		}
	};
</script>

<section
	{...rest}
	bind:this={root}
	class={['preview-pane', className]}
	data-fullscreen={fullscreen ? 'true' : 'false'}
	tabindex="0"
	aria-label={t('viewer.pane.label', { name: stat.name })}
	onkeydown={onKeydown}
>
	<Cluster gap="2" class="toolbar">
		<Link href={src} external>{t('viewer.open_new_tab')}</Link>
		<Link href={src} download={stat.name}>{t('viewer.download')}</Link>
		<CopyButton value={stat.path} label={t('viewer.copy_path')} variant="button" />
		{#if isText}
			<ToggleButton bind:pressed={wrap} label={t('viewer.wrap')} />
		{/if}
		<Button size="sm" variant="ghost" aria-pressed={rangeOpen} onclick={() => (rangeOpen = !rangeOpen)}
			>{t('viewer.download_range')}</Button
		>
		<Button size="sm" variant="ghost" onclick={toggleFullscreen}
			>{fullscreen ? t('viewer.exit_full_screen') : t('viewer.full_screen')}</Button
		>
		{#if showsReadout}
			<Text size="sm" tone="muted" tabular>{sizeReadout}</Text>
		{/if}
	</Cluster>

	{#if rangeOpen}
		<Stack gap="2">
			<Cluster gap="2" align="end">
				<Stack gap="1-5">
					<Label for={fromId}>{t('viewer.range.from')}</Label>
					<NumberInput id={fromId} bind:value={rangeFrom} min={0} max={stat.size} step={1} precision={0} />
				</Stack>
				<Stack gap="1-5">
					<Label for={toId}>{t('viewer.range.to')}</Label>
					<NumberInput id={toId} bind:value={rangeTo} min={0} max={stat.size} step={1} precision={0} />
				</Stack>
				<Button size="sm" variant="secondary" loading={rangeBusy} onclick={downloadRange}
					>{t('viewer.range.submit')}</Button
				>
			</Cluster>
			{#if null !== rangeMessage}
				<Text size="sm" tone="danger">{rangeMessage}</Text>
			{/if}
		</Stack>
	{/if}

	{#if fullscreen && siblings.length > 1}
		<nav aria-label={t('viewer.filmstrip')}>
			<Cluster gap="2" wrap>
				{#if undefined !== previous && undefined !== onnavigate}
					<Button size="sm" variant="ghost" iconStart="chevron-left" onclick={() => onnavigate?.(previous.path)}
						>{t('viewer.previous_file')}</Button
					>
				{/if}
				{#each siblings as sibling (sibling.path)}
					<Button
						size="sm"
						variant={sibling.path === stat.path ? 'secondary' : 'ghost'}
						aria-current={sibling.path === stat.path ? 'true' : undefined}
						onclick={() => onnavigate?.(sibling.path)}>{sibling.name}</Button
					>
				{/each}
				{#if undefined !== next && undefined !== onnavigate}
					<Button size="sm" variant="ghost" iconEnd="chevron-right" onclick={() => onnavigate?.(next.path)}
						>{t('viewer.next_file')}</Button
					>
				{/if}
			</Cluster>
		</nav>
	{/if}

	{#if 'pdf' === resolved}
		<object class="pdf" type="application/pdf" data={src} aria-label={t('viewer.pdf.label')}>
			<Stack gap="2">
				<Text>{t('viewer.pdf.fallback')}</Text>
				<Link href={src} external>{t('viewer.open_new_tab')}</Link>
			</Stack>
		</object>
	{:else if isText}
		<TextViewer size={stat.size} {loadRange} bind:loaded {wrap} {search} tail={'log' === resolved} />
	{:else if 'binary' === resolved}
		<HexViewer size={stat.size} {loadRange} bind:loaded />
	{:else if 'image' === resolved || 'video' === resolved || 'audio' === resolved}
		<MediaViewer kind={resolved} {src} alt={stat.name} />
	{:else}
		<Stack gap="2">
			<Text size="title-sm" as="p">{t('viewer.unsupported.title')}</Text>
			<Text tone="muted">{t('viewer.unsupported.body')}</Text>
			<Link href={src} download={stat.name}>{t('viewer.download')}</Link>
		</Stack>
	{/if}
</section>

<style>
	.preview-pane {
		display: grid;
		gap: var(--preview-gap, var(--space-4));
		padding: var(--preview-pad, var(--space-4));
		background: var(--preview-bg, var(--surface));
		min-inline-size: 0;
	}

	.preview-pane[data-fullscreen='true'] {
		position: fixed;
		inset: 0;
		z-index: var(--z-dialog);
		overflow: auto;
		align-content: start;
	}

	.preview-pane:focus-visible {
		box-shadow: var(--focus-ring);
		outline: var(--focus-width) solid transparent;
	}

	.pdf {
		inline-size: 100%;
		block-size: var(--preview-pdf-h, calc(var(--space-16) * 10));
	}
</style>
