<script lang="ts" module>
	export type TextSize = 'caption' | 'sm' | 'body' | 'title-sm' | 'title' | 'heading' | 'display';
	export type TextTone = 'default' | 'muted' | 'faint' | 'accent' | 'ok' | 'warn' | 'danger';
	export type TextWeight = 400 | 500 | 600;
	const TONE_COLORS: Record<TextTone, string> = {
		default: 'var(--text)',
		muted: 'var(--text-muted)',
		faint: 'var(--text-faint)',
		accent: 'var(--accent-text)',
		ok: 'var(--ok)',
		warn: 'var(--warn)',
		danger: 'var(--danger)'
	};
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLElement> {
		as?: string;
		size?: TextSize;
		tone?: TextTone;
		weight?: TextWeight;
		mono?: boolean;
		tabular?: boolean;
		truncate?: boolean | 2;
		balance?: boolean;
		children: Snippet;
	}

	let {
		as = 'span',
		size = 'body',
		tone = 'default',
		weight,
		mono = false,
		tabular = false,
		truncate = false,
		balance = false,
		class: className,
		children,
		...rest
	}: Props = $props();
</script>

<svelte:element
	this={as}
	class={['text', className]}
	class:mono
	class:tabular
	class:balance
	class:truncate={true === truncate}
	class:clamp={2 === truncate}
	style:--text-tone-color={TONE_COLORS[tone]}
	style:--text-size-value={`var(--text-${size}-size)`}
	style:--text-line-value={`var(--text-${size}-line)`}
	style:--text-tracking-value={`var(--text-${size}-tracking)`}
	style:--text-weight-value={weight ?? `var(--text-${size}-weight)`}
	style:--text-clamp={2 === truncate ? 2 : undefined}
	{...rest}
>
	{@render children()}
</svelte:element>

<style>
	.text {
		min-inline-size: 0;
		overflow-wrap: anywhere;
		color: var(--text-color, var(--text-tone-color));
		font-family: var(--font-sans);
		font-size: var(--text-size-value);
		line-height: var(--text-line-value);
		font-weight: var(--text-weight, var(--text-weight-value));
		letter-spacing: var(--text-tracking-value);
	}

	.mono {
		font-family: var(--font-mono);
	}

	.tabular {
		font-variant-numeric: tabular-nums;
	}

	.balance {
		text-wrap: balance;
	}

	.truncate {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.clamp {
		display: -webkit-box;
		overflow: hidden;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: var(--text-clamp);
	}
</style>
