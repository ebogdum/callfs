<script lang="ts" module>
	import type { ToastAction } from '../molecules/Toast.svelte';
	import type { Tone } from '../types';

	export interface ToastEntry {
		id: number;
		tone: Tone;
		title: string;
		body?: string;
		action?: ToastAction;
		duration: number;
	}

	export const TOKENS = ['--toaster-gap', '--toaster-inset', '--toaster-bottom'] as const;

	const VISIBLE_LIMIT = 3;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Toast from '../molecules/Toast.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		items: ToastEntry[];
		ondismiss: (id: number) => void;
	}

	let { items, ondismiss, class: className, ...rest }: Props = $props();

	const visible = $derived(items.slice(0, VISIBLE_LIMIT));
</script>

<div
	{...rest}
	class={['toaster', className]}
	role="region"
	aria-label={t('notifications.toasts.region')}
	aria-live="polite"
>
	{#each visible as toast (toast.id)}
		<Toast
			tone={toast.tone}
			title={toast.title}
			body={toast.body}
			action={toast.action}
			duration={toast.duration}
			onclose={() => ondismiss(toast.id)}
		/>
	{/each}
</div>

<style>
	.toaster {
		position: fixed;
		z-index: var(--z-toast);
		inset-block-end: var(--toaster-bottom, var(--space-4));
		inset-inline-end: var(--toaster-inset, var(--space-4));
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: var(--toaster-gap, var(--space-2));
	}

	@media (max-width: 639px) {
		.toaster {
			inset-block-end: calc(var(--dock-h) + var(--toaster-bottom, var(--space-4)));
			inset-inline: var(--toaster-inset, var(--space-4));
			align-items: center;
			pointer-events: none;
		}

		.toaster > * {
			pointer-events: auto;
		}
	}
</style>
