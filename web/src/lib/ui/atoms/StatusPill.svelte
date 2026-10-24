<script lang="ts" module>
	export type StatusPillSize = 'sm' | 'md';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import type { StatusKey, Tone } from '../types';
	import Tooltip from './Tooltip.svelte';
	import { STATUS } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		status: StatusKey;
		label?: string;
		size?: StatusPillSize;
		detail?: string;
	}

	let { status, label, size = 'md', detail, class: className, ...rest }: Props = $props();

	const entry = $derived(STATUS[status]);

	const pillTone: Tone = $derived.by(() => {
		if ('progress' === entry.tone) {
			return 'info';
		}
		if ('muted' === entry.tone) {
			return 'neutral';
		}
		return entry.tone;
	});

	const text = $derived(undefined === label ? t(entry.word) : label);
</script>

{#snippet pill()}
	<span class={['pill', className]} data-tone={pillTone} data-size={size} data-status={status} {...rest}>
		<StatusGlyph tone={entry.tone} {size} />
		<span class="word">{text}</span>
	</span>
{/snippet}

{#if undefined === detail}
	{@render pill()}
{:else}
	<Tooltip content={detail}>
		{#snippet children()}
			{@render pill()}
		{/snippet}
	</Tooltip>
{/if}

<style>
	.pill {
		--tone-soft: var(--surface-sunken);
		--tone-border: var(--border);
		--tone-text: var(--text-muted);
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		padding-block: var(--space-0-5);
		padding-inline: var(--pill-px, var(--space-2));
		border: var(--hairline) solid var(--pill-border, var(--tone-border));
		border-radius: var(--pill-radius, var(--radius-full));
		background: var(--pill-bg, var(--tone-soft));
		color: var(--pill-fg, var(--tone-text));
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.pill[data-size='md'] {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-sm-tracking);
	}

	.pill[data-tone='accent'] {
		--tone-soft: var(--accent-soft);
		--tone-border: var(--accent-border);
		--tone-text: var(--accent-text);
	}

	.pill[data-tone='ok'] {
		--tone-soft: var(--ok-soft);
		--tone-border: var(--ok-border);
		--tone-text: var(--ok-text);
	}

	.pill[data-tone='warn'] {
		--tone-soft: var(--warn-soft);
		--tone-border: var(--warn-border);
		--tone-text: var(--warn-text);
	}

	.pill[data-tone='danger'] {
		--tone-soft: var(--danger-soft);
		--tone-border: var(--danger-border);
		--tone-text: var(--danger-text);
	}

	.pill[data-tone='info'] {
		--tone-soft: var(--info-soft);
		--tone-border: var(--info-border);
		--tone-text: var(--info-text);
	}

	.word {
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}
</style>
