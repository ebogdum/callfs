<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { IconName, Tone } from '../types';
	import Icon from './Icon.svelte';
	import { iconPixels, type IconSize } from './icons';

	type GlyphTone = Tone | 'progress' | 'muted';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		tone: GlyphTone;
		size?: IconSize;
	}

	const iconNames: Record<GlyphTone, IconName | undefined> = {
		ok: 'ok',
		warn: 'warn',
		danger: 'danger',
		info: 'info',
		neutral: 'circle',
		accent: 'circle-dot',
		progress: undefined,
		muted: undefined
	};

	let { tone, size = 'md', class: className, ...rest }: Props = $props();

	const iconName = $derived(iconNames[tone]);
</script>

<span
	class={['glyph', className]}
	data-tone={tone}
	aria-hidden="true"
	style:--glyph-size={`${iconPixels[size]}px`}
	{...rest}
>
	{#if undefined === iconName}
		<span class="ring"></span>
	{:else}
		<Icon name={iconName} {size} />
	{/if}
</span>

<style>
	.glyph {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
	}

	.glyph[data-tone='ok'] {
		color: var(--ok);
	}

	.glyph[data-tone='warn'] {
		color: var(--warn);
	}

	.glyph[data-tone='danger'] {
		color: var(--danger);
	}

	.glyph[data-tone='info'] {
		color: var(--info);
	}

	.glyph[data-tone='accent'],
	.glyph[data-tone='progress'] {
		color: var(--accent);
	}

	.ring {
		display: block;
		inline-size: var(--glyph-size, var(--icon-size));
		block-size: var(--glyph-size, var(--icon-size));
		border: var(--hairline) solid currentColor;
		border-radius: var(--radius-full);
	}

	.glyph[data-tone='progress'] .ring {
		border-block-start-color: transparent;
		animation: spin var(--dur-shimmer) linear infinite;
	}

	.glyph[data-tone='muted'] .ring {
		border-style: dashed;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.glyph[data-tone='progress'] .ring {
			animation: none;
		}
	}

	:global([data-motion='reduced']) .glyph[data-tone='progress'] .ring {
		animation: none;
	}
</style>
