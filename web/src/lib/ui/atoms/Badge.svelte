<script lang="ts" module>
	export type BadgeVariant = 'soft' | 'solid' | 'outline';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName, Tone } from '../types';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		tone?: Tone;
		variant?: BadgeVariant;
		icon?: IconName;
		children: Snippet;
	}

	let {
		tone = 'neutral',
		variant = 'soft',
		icon,
		class: className,
		children,
		...rest
	}: Props = $props();
</script>

<span class={['badge', className]} data-tone={tone} data-variant={variant} {...rest}>
	{#if undefined !== icon}<Icon name={icon} size="xs" />{/if}{@render children()}
</span>

<style>
	.badge {
		--tone-solid: var(--text-muted);
		--tone-soft: var(--surface-sunken);
		--tone-border: var(--border);
		--tone-text: var(--text-muted);
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		max-inline-size: 100%;
		padding-block: var(--space-0-5);
		padding-inline: var(--badge-px, var(--space-2));
		border: var(--hairline) solid transparent;
		border-radius: var(--badge-radius, var(--radius-xs));
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.badge[data-tone='accent'] {
		--tone-solid: var(--accent);
		--tone-soft: var(--accent-soft);
		--tone-border: var(--accent-border);
		--tone-text: var(--accent-text);
	}

	.badge[data-tone='ok'] {
		--tone-solid: var(--ok);
		--tone-soft: var(--ok-soft);
		--tone-border: var(--ok-border);
		--tone-text: var(--ok-text);
	}

	.badge[data-tone='warn'] {
		--tone-solid: var(--warn);
		--tone-soft: var(--warn-soft);
		--tone-border: var(--warn-border);
		--tone-text: var(--warn-text);
	}

	.badge[data-tone='danger'] {
		--tone-solid: var(--danger);
		--tone-soft: var(--danger-soft);
		--tone-border: var(--danger-border);
		--tone-text: var(--danger-text);
	}

	.badge[data-tone='info'] {
		--tone-solid: var(--info);
		--tone-soft: var(--info-soft);
		--tone-border: var(--info-border);
		--tone-text: var(--info-text);
	}

	.badge[data-variant='soft'] {
		background: var(--badge-bg, var(--tone-soft));
		color: var(--badge-fg, var(--tone-text));
	}

	.badge[data-variant='solid'] {
		background: var(--badge-bg, var(--tone-solid));
		color: var(--badge-fg, var(--text-on-accent));
	}

	.badge[data-variant='outline'] {
		background: var(--badge-bg, transparent);
		color: var(--badge-fg, var(--tone-text));
		border-color: var(--badge-border, var(--tone-border));
	}
</style>
