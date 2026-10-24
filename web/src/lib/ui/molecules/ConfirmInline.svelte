<script lang="ts" module>
	export type ConfirmTone = 'danger' | 'warn';

	export const TOKENS = ['--confirm-inline-pad'] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Button from '../atoms/Button.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		question: string;
		consequence: string | Snippet;
		confirmLabel: string;
		cancelLabel?: string;
		typedWord?: string;
		tone?: ConfirmTone;
		busy?: boolean;
		onconfirm?: () => void;
		oncancel?: () => void;
		class?: string;
	}

	let {
		question,
		consequence,
		confirmLabel,
		cancelLabel = t('common.cancel'),
		typedWord,
		tone = 'danger',
		busy = false,
		onconfirm,
		oncancel,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const questionId = $derived(`${uid}-question`);
	const consequenceId = $derived(`${uid}-consequence`);
	const typedId = $derived(`${uid}-typed`);
	const errorId = $derived(`${uid}-error`);

	let typed = $state('');
	let attempted = $state(false);
	let root: HTMLDivElement | undefined = $state();

	const requiresWord = $derived(undefined !== typedWord && '' !== typedWord);
	const mismatch = $derived(attempted && requiresWord && typedWord !== typed);
	const confirmVariant = $derived('warn' === tone ? 'primary' : 'danger');

	$effect(() => {
		root?.querySelector<HTMLButtonElement>('button')?.focus();
	});

	function confirm(): void {
		attempted = true;
		if (busy) return;
		if (requiresWord && typedWord !== typed) return;
		onconfirm?.();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if ('Escape' === event.key) {
			oncancel?.();
		}
	}
</script>

<div
	bind:this={root}
	role="group"
	aria-labelledby={questionId}
	aria-describedby={consequenceId}
	data-tone={tone}
	class={['confirm-inline', className]}
	onkeydown={handleKeydown}
	{...rest}
>
	<Stack gap="2">
		<Cluster gap="2" align="start">
			<StatusGlyph tone={tone} size="md" />
			<Text as="p" id={questionId} size="title-sm" weight={600}>{question}</Text>
		</Cluster>
		<div id={consequenceId}>
			{#if 'string' === typeof consequence}
				<Text as="p" tone="muted">{consequence}</Text>
			{:else}
				{@render consequence()}
			{/if}
		</div>
		{#if requiresWord}
			<Stack gap="1-5">
				<Label for={typedId}>{t('confirm.type_word', { word: typedWord ?? '' })}</Label>
				<Input
					id={typedId}
					bind:value={typed}
					autocomplete="off"
					invalid={mismatch}
					aria-describedby={mismatch ? errorId : undefined}
				/>
				<div aria-live="polite">
					{#if mismatch}
						<FieldError id={errorId} message={t('confirm.word_mismatch', { word: typedWord ?? '' })} />
					{/if}
				</div>
			</Stack>
		{/if}
		<Cluster gap="2" justify="end">
			<Button onclick={() => oncancel?.()}>{cancelLabel}</Button>
			<Button variant={confirmVariant} loading={busy} onclick={confirm}>{confirmLabel}</Button>
		</Cluster>
	</Stack>
</div>

<style>
	.confirm-inline {
		display: grid;
		min-inline-size: 0;
		padding: var(--confirm-inline-pad, var(--space-4));
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
	}

	.confirm-inline[data-tone='danger'] {
		border-color: var(--danger-border);
	}
</style>
