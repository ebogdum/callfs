<script lang="ts" module>
	export const TOKENS = ['--tag-bg', '--tag-fg', '--tag-border', '--tag-radius', '--tag-px'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import type { Tone } from '../types';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		label: string;
		tone?: Tone;
		removable?: boolean;
		onremove?: () => void;
	}

	let {
		label,
		tone = 'neutral',
		removable = false,
		onremove,
		class: className,
		...rest
	}: Props = $props();

	function handleKeydown(event: KeyboardEvent) {
		if (true === removable && 'Backspace' === event.key) {
			event.preventDefault();
			onremove?.();
		}
	}
</script>

<span
	class={['tag', className]}
	data-tone={tone}
	data-removable={removable}
	role={removable ? 'group' : undefined}
	aria-label={removable ? label : undefined}
	tabindex={removable ? 0 : undefined}
	{...rest}
	onkeydown={handleKeydown}
>
	<span class="text">{label}</span>
	{#if removable}
		<button
			type="button"
			class="remove"
			aria-label={t('u052atoms.remove_tag', { label })}
			onclick={() => onremove?.()}
		>
			<Icon name="x" size="xs" />
		</button>
	{/if}
</span>

<style>
	.tag {
		--tone-soft: var(--surface-sunken);
		--tone-border: var(--border);
		--tone-text: var(--text);
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		max-inline-size: 100%;
		min-block-size: var(--target-min);
		padding-block: var(--space-0-5);
		padding-inline: var(--tag-px, var(--space-2));
		border: var(--hairline) solid var(--tag-border, var(--tone-border));
		border-radius: var(--tag-radius, var(--radius-xs));
		background: var(--tag-bg, var(--tone-soft));
		color: var(--tag-fg, var(--tone-text));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.tag[data-tone='accent'] {
		--tone-soft: var(--accent-soft);
		--tone-border: var(--accent-border);
		--tone-text: var(--accent-text);
	}

	.tag[data-tone='ok'] {
		--tone-soft: var(--ok-soft);
		--tone-border: var(--ok-border);
		--tone-text: var(--ok-text);
	}

	.tag[data-tone='warn'] {
		--tone-soft: var(--warn-soft);
		--tone-border: var(--warn-border);
		--tone-text: var(--warn-text);
	}

	.tag[data-tone='danger'] {
		--tone-soft: var(--danger-soft);
		--tone-border: var(--danger-border);
		--tone-text: var(--danger-text);
	}

	.tag[data-tone='info'] {
		--tone-soft: var(--info-soft);
		--tone-border: var(--info-border);
		--tone-text: var(--info-text);
	}

	.tag:focus-visible,
	.remove:focus-visible {
		box-shadow: var(--focus-ring);
		outline: var(--focus-width) solid transparent;
	}

	.text {
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.remove {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		inline-size: var(--target-min);
		block-size: var(--target-min);
		padding: var(--space-0);
		border: 0;
		border-radius: var(--radius-xs);
		background: transparent;
		color: inherit;
		cursor: pointer;
	}

	.remove:active {
		background: var(--surface-pressed);
	}

	@media (hover: hover) {
		.remove:hover {
			background: var(--surface-hover);
		}
	}

	@media (pointer: coarse) {
		.tag {
			min-block-size: var(--touch-target);
		}

		.remove {
			inline-size: var(--touch-target);
			block-size: var(--touch-target);
		}
	}
</style>
