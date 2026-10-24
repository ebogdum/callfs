<script lang="ts" module>
	export type CopyButtonVariant = 'icon' | 'button';

	type CopyState = 'idle' | 'copied' | 'failed';

	const COPY_FEEDBACK_MS = 1500;
</script>

<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { announce } from '../quarks/announce';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';
	import Tooltip from './Tooltip.svelte';

	const GLYPHS: Record<CopyState, IconName> = {
		idle: 'copy',
		copied: 'check',
		failed: 'fail'
	};

	interface Props extends Omit<HTMLButtonAttributes, 'value'> {
		value: string;
		label?: string;
		variant?: CopyButtonVariant;
	}

	let { value, label = t('common.copy'), variant = 'icon', class: className, ...rest }: Props = $props();

	let copyState = $state<CopyState>('idle');
	const glyph = $derived(GLYPHS[copyState]);

	$effect(() => {
		if ('idle' === copyState) {
			return;
		}
		const timer = setTimeout(() => {
			copyState = 'idle';
		}, COPY_FEEDBACK_MS);
		return () => clearTimeout(timer);
	});

	const reportFailure = (): void => {
		copyState = 'failed';
		announce(t('text.copy_failed'), 'polite');
	};

	const copy = (): void => {
		if (undefined === navigator.clipboard) {
			reportFailure();
			return;
		}
		navigator.clipboard.writeText(value).then(
			() => {
				copyState = 'copied';
				announce(t('common.copied'), 'polite');
			},
			reportFailure
		);
	};
</script>

{#snippet control()}
	<button
		type="button"
		{...rest}
		class={['copy-button', className]}
		data-variant={variant}
		data-state={copyState}
		aria-label={'icon' === variant ? label : undefined}
		onclick={copy}
	>
		<Icon name={glyph} size="sm" />
		{#if 'button' === variant}
			<span class="label">{label}</span>
		{/if}
	</button>
{/snippet}

{#if 'icon' === variant}
	<Tooltip content={label}>
		{@render control()}
	</Tooltip>
{:else}
	{@render control()}
{/if}

<style>
	.copy-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		font: inherit;
		font-size: var(--text-sm-size);
		color: var(--copy-button-fg, var(--text-muted));
		background: transparent;
		border: var(--hairline) solid transparent;
		border-radius: var(--radius-sm);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	.copy-button[data-variant='icon'] {
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
	}

	.copy-button[data-variant='button'] {
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		border-color: var(--border-control);
		background: var(--surface-raised);
		color: var(--text);
	}

	.copy-button[data-state='copied'] {
		color: var(--ok-text);
	}

	.copy-button[data-state='failed'] {
		color: var(--danger-text);
	}

	@media (hover: hover) {
		.copy-button:hover {
			background: var(--copy-button-hover-bg, var(--surface-hover));
		}
	}

	.copy-button:active {
		background: var(--surface-pressed);
	}

	.copy-button:focus-visible {
		box-shadow: var(--focus-ring);
	}

	@media (pointer: coarse) {
		.copy-button[data-variant='icon'] {
			min-inline-size: var(--touch-target);
			min-block-size: var(--touch-target);
		}
	}
</style>
