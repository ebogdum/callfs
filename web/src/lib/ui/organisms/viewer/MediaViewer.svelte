<script lang="ts" module>
	export type MediaKind = 'image' | 'video' | 'audio';
	export type MediaZoom = 'fit' | number;

	export const TOKENS = ['--media-viewer-bg', '--media-viewer-h'] as const;

	const ZOOM_STEP = 1.25;
	const ZOOM_MIN = 0.1;
	const ZOOM_MAX = 4;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Button from '../../atoms/Button.svelte';
	import Text from '../../atoms/Text.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Scroll from '../../quarks/Scroll.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		kind: MediaKind;
		src: string;
		alt: string;
	}

	let { kind, src, alt, class: className, ...rest }: Props = $props();

	let zoom = $state<MediaZoom>('fit');

	const scale = $derived('fit' === zoom ? 1 : zoom);
	const level = $derived(formatNumber(Math.round(scale * 100), 'int'));

	const zoomIn = (): void => {
		zoom = Math.min(ZOOM_MAX, scale * ZOOM_STEP);
	};

	const zoomOut = (): void => {
		zoom = Math.max(ZOOM_MIN, scale / ZOOM_STEP);
	};
</script>

<div {...rest} class={['media-viewer', className]} data-kind={kind}>
	{#if 'image' === kind}
		<Cluster gap="2">
			<Button size="sm" variant="ghost" aria-pressed={'fit' === zoom} onclick={() => (zoom = 'fit')}
				>{t('viewer.media.fit')}</Button
			>
			<Button size="sm" variant="ghost" aria-pressed={1 === zoom} onclick={() => (zoom = 1)}
				>{t('viewer.media.actual')}</Button
			>
			<Button size="sm" variant="ghost" iconStart="minus" onclick={zoomOut}>{t('viewer.media.zoom_out')}</Button>
			<Button size="sm" variant="ghost" iconStart="plus" onclick={zoomIn}>{t('viewer.media.zoom_in')}</Button>
			<Text size="sm" tone="muted" tabular>{t('viewer.media.zoom_level', { level })}</Text>
		</Cluster>
		<Scroll axis="both" label={alt} class="stage">
			<img
				class="image"
				class:fit={'fit' === zoom}
				{src}
				{alt}
				style:transform={'fit' === zoom ? undefined : `scale(${zoom})`}
			/>
		</Scroll>
	{:else if 'video' === kind}
		<video class="player" {src} controls preload="metadata" aria-label={t('viewer.media.video_label')}>
			{t('viewer.media.unsupported')}
		</video>
	{:else}
		<audio class="player" {src} controls preload="metadata" aria-label={t('viewer.media.audio_label')}>
			{t('viewer.media.unsupported')}
		</audio>
	{/if}
</div>

<style>
	.media-viewer {
		display: grid;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.media-viewer :global(.stage) {
		block-size: var(--media-viewer-h, calc(var(--space-16) * 6));
		padding: var(--space-3);
		border-radius: var(--radius-md);
		background: var(--media-viewer-bg, var(--surface-sunken));
	}

	.image {
		display: block;
		max-inline-size: none;
		transform-origin: top left;
	}

	.image.fit {
		max-inline-size: 100%;
		block-size: auto;
	}

	.player {
		inline-size: 100%;
		border-radius: var(--radius-md);
		background: var(--media-viewer-bg, var(--surface-sunken));
	}
</style>
