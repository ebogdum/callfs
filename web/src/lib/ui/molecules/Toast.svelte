<script lang="ts" module>
	export interface ToastAction {
		label: string;
		run: () => void;
	}

	export const TOKENS = [
		'--toast-bg',
		'--toast-border',
		'--toast-radius',
		'--toast-shadow',
		'--toast-pad',
		'--toast-w'
	] as const;

	const DEFAULT_LIFETIME_MS = 6000;
	const ACTION_LIFETIME_MS = 10000;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import type { Tone } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		tone: Tone;
		title: string;
		body?: string;
		action?: ToastAction;
		onclose: () => void;
		onpause?: () => void;
		onresume?: () => void;
		duration?: number;
	}

	let { tone, title, body, action, onclose, onpause, onresume, duration, class: className, ...rest }: Props = $props();

	let hovered = $state(false);
	let focused = $state(false);
	let remainingMs: number | undefined;

	const lifetime = $derived(duration ?? (undefined === action ? DEFAULT_LIFETIME_MS : ACTION_LIFETIME_MS));
	const paused = $derived(hovered || focused);

	let wasPaused = false;

	$effect(() => {
		const isPaused = paused;
		if (isPaused === wasPaused) {
			return;
		}
		wasPaused = isPaused;
		if (isPaused) {
			onpause?.();
		} else {
			onresume?.();
		}
	});

	$effect(() => {
		if (paused) {
			return;
		}
		const budget = remainingMs ?? lifetime;
		const startedAt = Date.now();
		const timer = setTimeout(() => {
			onclose();
		}, budget);
		return () => {
			clearTimeout(timer);
			remainingMs = budget - (Date.now() - startedAt);
		};
	});
</script>

<div
	{...rest}
	class={['toast', className]}
	role={'danger' === tone ? 'alert' : 'status'}
	data-tone={tone}
	onpointerenter={() => (hovered = true)}
	onpointerleave={() => (hovered = false)}
	onfocusin={() => (focused = true)}
	onfocusout={() => (focused = false)}
>
	<span class="glyph"><StatusGlyph {tone} /></span>
	<div class="text">
		<p class="title">{title}</p>
		{#if undefined !== body}<p class="body">{body}</p>{/if}
	</div>
	<div class="actions">
		{#if undefined !== action}
			<Button variant="ghost" size="sm" onclick={action.run}>{action.label}</Button>
		{/if}
		<Button variant="ghost" size="sm" iconStart="x" onclick={onclose}>{t('common.close')}</Button>
	</div>
</div>

<style>
	.toast {
		--tone: var(--text-muted);
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--space-2);
		inline-size: min(100%, var(--toast-w, var(--dialog-sm)));
		padding: var(--toast-pad, var(--space-3));
		background: var(--toast-bg, var(--surface-overlay));
		border: var(--hairline) solid var(--toast-border, var(--border-subtle));
		border-radius: var(--toast-radius, var(--radius-lg));
		box-shadow: var(--toast-shadow, var(--shadow-md));
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
	}

	.toast[data-tone='accent'] {
		--tone: var(--accent);
	}

	.toast[data-tone='ok'] {
		--tone: var(--ok);
	}

	.toast[data-tone='warn'] {
		--tone: var(--warn);
	}

	.toast[data-tone='danger'] {
		--tone: var(--danger);
	}

	.toast[data-tone='info'] {
		--tone: var(--info);
	}

	.glyph {
		display: inline-flex;
		color: var(--tone);
	}

	.text {
		display: grid;
		gap: var(--space-1);
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
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
</style>
