<script lang="ts" module>
	export type ErrorStateVariant = 'region' | 'page';

	export const TOKENS = ['--error-state-pad', '--error-state-gap', '--error-state-fg'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatTimestamp } from '$lib/format/time';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Link from '../atoms/Link.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import type { UiError } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		error: UiError;
		onretry?: () => void;
		variant?: ErrorStateVariant;
	}

	let { error, onretry, variant = 'region', class: className, ...rest }: Props = $props();

	const fixAction = $derived(error.fixAction);
	const details = $derived(
		[error.status, error.code, error.requestId, formatTimestamp(Date.now())]
			.filter((part): part is string | number => undefined !== part)
			.join('\n')
	);
</script>

<div {...rest} class={['error-state', className]} data-state="error" data-variant={variant}>
	<div class="head">
		<span class="glyph"><StatusGlyph tone="danger" /></span>
		<h2 class="title">{error.title}</h2>
	</div>
	<p class="body">{error.body}</p>
	<div class="actions">
		{#if undefined !== onretry && error.retryable}
			<Button variant="primary" onclick={onretry}>{t('common.retry')}</Button>
		{/if}
		{#if undefined !== fixAction && t('errors.server.fix') !== fixAction.label}
			{#if undefined !== fixAction.href}
				<Link href={fixAction.href}>{fixAction.label}</Link>
			{:else}
				<Button variant="secondary" onclick={() => fixAction.run?.()}>{fixAction.label}</Button>
			{/if}
		{/if}
		<CopyButton value={details} label={t('errors.server.fix')} variant="button" />
	</div>
</div>

<style>
	.error-state {
		display: grid;
		gap: var(--error-state-gap, var(--space-3));
		justify-items: center;
		align-content: center;
		max-inline-size: var(--read-max);
		margin-inline: auto;
		text-align: center;
		padding: var(--error-state-pad, var(--space-4));
		color: var(--error-state-fg, var(--text));
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		letter-spacing: var(--text-body-tracking);
	}

	.error-state[data-variant='page'] {
		padding: var(--error-state-pad, var(--space-8));
	}

	.head {
		display: grid;
		grid-template-columns: auto minmax(0, auto);
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
	}

	.glyph {
		display: inline-flex;
		color: var(--danger);
	}

	.title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		text-wrap: balance;
	}

	.body {
		margin: 0;
		color: var(--text-muted);
		text-wrap: pretty;
		max-inline-size: calc(var(--ch) * 52);
		overflow-wrap: anywhere;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		padding-block-start: var(--space-1);
		gap: var(--space-2);
	}
</style>
