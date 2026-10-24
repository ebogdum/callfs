<script lang="ts" module>
	export const TOKENS = [
		'--textarea-bg',
		'--textarea-fg',
		'--textarea-border',
		'--textarea-radius',
		'--textarea-count-fg'
	] as const;
</script>

<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<HTMLTextareaAttributes, 'value' | 'rows' | 'maxlength' | 'class' | 'onkeydown'> {
		value?: string;
		rows?: number;
		maxRows?: number;
		maxLength?: number;
		class?: string;
		onkeydown?: (event: KeyboardEvent) => void;
		onsubmit?: () => void;
	}

	let {
		value = $bindable(''),
		rows = 3,
		maxRows = 12,
		maxLength,
		class: className = '',
		onkeydown,
		onsubmit,
		...rest
	}: Props = $props();

	let element: HTMLTextAreaElement | undefined = $state();

	const countText = $derived(
		undefined !== maxLength && 0 < maxLength && maxLength * 0.8 <= value.length
			? t('form.char_count', { count: value.length, max: maxLength })
			: ''
	);

	function grow(target: HTMLTextAreaElement): void {
		target.style.blockSize = 'auto';
		const style = getComputedStyle(target);
		const chrome =
			Number.parseFloat(style.paddingBlockStart) +
			Number.parseFloat(style.paddingBlockEnd) +
			Number.parseFloat(style.borderBlockStartWidth) +
			Number.parseFloat(style.borderBlockEndWidth);
		const ceiling = Number.parseFloat(style.lineHeight) * maxRows + chrome;
		const natural = target.scrollHeight;
		const height = Number.isFinite(ceiling) && natural > ceiling ? ceiling : natural;
		target.style.blockSize = `${height}px`;
	}

	$effect(() => {
		if ('string' === typeof value && undefined !== element) {
			grow(element);
		}
	});

	function handleKeydown(event: KeyboardEvent): void {
		onkeydown?.(event);
		if ('Enter' === event.key && (event.metaKey || event.ctrlKey)) {
			event.preventDefault();
			onsubmit?.();
		}
	}
</script>

<div class={['textarea-field', className]}>
	<textarea
		{...rest}
		bind:this={element}
		bind:value
		{rows}
		maxlength={maxLength}
		class="textarea"
		onkeydown={handleKeydown}
	></textarea>
	{#if '' !== countText}<span class="count">{countText}</span>{/if}
</div>

<style>
	.textarea-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.textarea {
		box-sizing: border-box;
		inline-size: 100%;
		min-block-size: var(--control-h);
		padding-block: var(--space-2);
		padding-inline: var(--control-px);
		overflow-y: auto;
		resize: none;
		font-family: var(--font-sans);
		font-size: var(--text-input);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		color: var(--textarea-fg, var(--text));
		background: var(--textarea-bg, var(--surface));
		border: var(--hairline) solid var(--textarea-border, var(--border-control));
		border-radius: var(--textarea-radius, var(--radius-md));
	}

	@media (hover: hover) {
		.textarea:hover:not(:disabled) {
			border-color: var(--border-strong);
		}
	}

	.textarea:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.textarea[aria-invalid='true'] {
		border-color: var(--danger);
	}

	.textarea:disabled {
		background: var(--surface-sunken);
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.count {
		align-self: flex-end;
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		color: var(--textarea-count-fg, var(--text-muted));
		font-variant-numeric: tabular-nums;
	}
</style>
