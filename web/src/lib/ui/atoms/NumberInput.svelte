<script lang="ts" module>
	export const TOKENS = [
		'--number-input-bg',
		'--number-input-fg',
		'--number-input-border',
		'--number-input-radius',
		'--number-input-step-hover-bg'
	] as const;
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<
		HTMLInputAttributes,
		'type' | 'value' | 'min' | 'max' | 'step' | 'class' | 'onkeydown' | 'onblur' | 'oninput'
	> {
		value?: number | null;
		min?: number;
		max?: number;
		step?: number;
		unit?: string;
		precision?: number;
		class?: string;
		onkeydown?: (event: KeyboardEvent) => void;
		onblur?: (event: FocusEvent) => void;
		oninput?: (event: Event) => void;
	}

	let {
		value = $bindable(null),
		min,
		max,
		step = 1,
		unit,
		precision,
		class: className = '',
		onkeydown,
		onblur,
		oninput,
		...rest
	}: Props = $props();

	const SHIFT_MULTIPLIER = 10;

	let draft = $state('');

	function parseDraft(text: string): number | null | undefined {
		const trimmed = text.trim();
		if ('' === trimmed) {
			return null;
		}
		const parsed = Number(trimmed);
		return Number.isFinite(parsed) ? parsed : undefined;
	}

	function formatNumber(next: number | null): string {
		if (null === next) {
			return '';
		}
		return undefined === precision ? String(next) : next.toFixed(precision);
	}

	function normalize(next: number): number {
		const bounded = undefined === min ? next : Math.max(min, next);
		const limited = undefined === max ? bounded : Math.min(max, bounded);
		return Number(limited.toFixed(precision ?? 6));
	}

	function commit(next: number | null): void {
		value = next;
		draft = formatNumber(next);
	}

	function handleDraft(text: string): void {
		draft = text;
		const parsed = parseDraft(text);
		if (undefined !== parsed) {
			value = parsed;
		}
	}

	function handleBlur(): void {
		const parsed = parseDraft(draft);
		const next = undefined === parsed ? value : parsed;
		commit(null === next ? null : normalize(next));
	}

	function stepBy(direction: 1 | -1, multiplier: number): void {
		const base = null === value ? 0 : value;
		commit(normalize(base + direction * step * multiplier));
	}

	function handleKeydown(event: KeyboardEvent): void {
		const multiplier = event.shiftKey ? SHIFT_MULTIPLIER : 1;
		if ('ArrowUp' === event.key) {
			event.preventDefault();
			stepBy(1, multiplier);
		} else if ('ArrowDown' === event.key) {
			event.preventDefault();
			stepBy(-1, multiplier);
		}
	}

	$effect(() => {
		const external = value;
		untrack(() => {
			if (external !== parseDraft(draft)) {
				draft = formatNumber(external);
			}
		});
	});
</script>

<div class={['number-input', className]}>
	<button type="button" class="step" aria-label={t('common.decrease')} onclick={() => stepBy(-1, 1)}>
		<Icon name="minus" />
	</button>
	<input
		{...rest}
		class="field"
		type="text"
		inputmode="decimal"
		value={draft}
		onkeydown={(event) => {
			onkeydown?.(event);
			handleKeydown(event);
		}}
		onblur={(event) => {
			onblur?.(event);
			handleBlur();
		}}
		oninput={(event) => {
			oninput?.(event);
			handleDraft(event.currentTarget.value);
		}}
	/>
	<button type="button" class="step" aria-label={t('common.increase')} onclick={() => stepBy(1, 1)}>
		<Icon name="plus" />
	</button>
	{#if undefined !== unit && '' !== unit}<span class="unit">{unit}</span>{/if}
</div>

<style>
	.number-input {
		display: flex;
		align-items: center;
		gap: var(--space-1-5);
		min-inline-size: 0;
	}

	.field {
		box-sizing: border-box;
		flex: 1 1 auto;
		min-inline-size: 0;
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		font-family: var(--font-sans);
		font-size: var(--text-input);
		font-variant-numeric: tabular-nums;
		color: var(--number-input-fg, var(--text));
		background: var(--number-input-bg, var(--surface));
		border: var(--hairline) solid var(--number-input-border, var(--border-control));
		border-radius: var(--number-input-radius, var(--radius-md));
	}

	.field:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.field[aria-invalid='true'] {
		border-color: var(--danger);
	}

	.field:disabled {
		background: var(--surface-sunken);
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.step {
		display: inline-flex;
		flex: 0 0 auto;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		min-inline-size: var(--control-h);
		block-size: var(--control-h);
		padding: 0;
		color: var(--text);
		background: transparent;
		border: 0;
		border-radius: var(--radius-md);
		cursor: pointer;
	}

	@media (hover: hover) {
		.step:hover {
			background: var(--number-input-step-hover-bg, var(--surface-hover));
		}
	}

	.step:active {
		background: var(--surface-pressed);
	}

	.step:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.unit {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
		white-space: nowrap;
	}
</style>
