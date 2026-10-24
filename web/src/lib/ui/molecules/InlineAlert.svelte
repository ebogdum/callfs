<script lang="ts" module>
	export const TOKENS = ['--inline-alert-bg', '--inline-alert-border', '--inline-alert-radius', '--inline-alert-pad'] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import type { Tone } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		tone: Tone;
		title: string;
		body?: string;
		actions?: Snippet;
	}

	let { tone, title, body, actions, class: className, ...rest }: Props = $props();
</script>

<div
	{...rest}
	class={['inline-alert', className]}
	role={'danger' === tone ? 'alert' : 'status'}
	data-tone={tone}
>
	<span class="glyph"><StatusGlyph {tone} /></span>
	<div class="text">
		<p class="title">{title}</p>
		{#if undefined !== body}<p class="body">{body}</p>{/if}
		{#if undefined !== actions}<div class="actions">{@render actions()}</div>{/if}
	</div>
</div>

<style>
	.inline-alert {
		--tone: var(--text-muted);
		--tone-soft: var(--surface-sunken);
		--tone-border: var(--border-subtle);
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: start;
		gap: var(--space-2);
		padding: var(--inline-alert-pad, var(--space-3));
		background: var(--inline-alert-bg, var(--tone-soft));
		border: var(--hairline) solid var(--inline-alert-border, var(--tone-border));
		border-radius: var(--inline-alert-radius, var(--radius-lg));
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
	}

	.inline-alert[data-tone='accent'] {
		--tone: var(--accent-text);
		--tone-soft: var(--accent-soft);
		--tone-border: var(--accent-border);
	}

	.inline-alert[data-tone='ok'] {
		--tone: var(--ok);
		--tone-soft: var(--ok-soft);
		--tone-border: var(--ok-border);
	}

	.inline-alert[data-tone='warn'] {
		--tone: var(--warn);
		--tone-soft: var(--warn-soft);
		--tone-border: var(--warn-border);
	}

	.inline-alert[data-tone='danger'] {
		--tone: var(--danger);
		--tone-soft: var(--danger-soft);
		--tone-border: var(--danger-border);
	}

	.inline-alert[data-tone='info'] {
		--tone: var(--info);
		--tone-soft: var(--info-soft);
		--tone-border: var(--info-border);
	}

	.glyph {
		display: inline-flex;
		color: var(--tone);
	}

	.text {
		display: grid;
		gap: var(--space-1-5);
		min-inline-size: 0;
	}

	.title {
		margin: 0;
		font-weight: 600;
		text-wrap: balance;
	}

	.body {
		margin: 0;
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
</style>
