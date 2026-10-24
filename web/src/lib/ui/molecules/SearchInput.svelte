<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Input from '../atoms/Input.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import Label from '../atoms/Label.svelte';
	import Icon from '../quarks/Icon.svelte';
	import Spinner from '../quarks/Spinner.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLInputAttributes, 'type' | 'value' | 'size' | 'prefix' | 'onsubmit'> {
		value?: string;
		label: string;
		shortcut?: string[];
		loading?: boolean;
		resultCount?: number | null;
		onsubmit?: (value: string) => void;
	}

	let {
		value = $bindable(''),
		label,
		shortcut = ['/'],
		loading = false,
		resultCount = null,
		onsubmit,
		onkeydown,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const fieldId = `${uid}-field`;
	const resultId = `${uid}-results`;

	function handleKeydown(event: KeyboardEvent & { currentTarget: EventTarget & HTMLInputElement }): void {
		onkeydown?.(event);
		if ('Escape' === event.key) {
			if ('' !== value) {
				value = '';
				event.preventDefault();
			} else if (event.currentTarget instanceof HTMLElement) {
				event.currentTarget.blur();
			}
		} else if ('Enter' === event.key) {
			onsubmit?.(value);
		}
	}
</script>

<Stack gap="1-5" class={className}>
	<Label for={fieldId}>{label}</Label>
	<Stack gap="1">
		<Input
			id={fieldId}
			type="search"
			role="searchbox"
			clearable
			bind:value
			aria-describedby={null === resultCount ? undefined : resultId}
			{...rest}
			onkeydown={handleKeydown}
		>
			{#snippet prefix()}
				<Icon name="search" size="sm" />
			{/snippet}
			{#snippet suffix()}
				{#if loading}
					<Spinner size="sm" label={t('common.loading_label')} />
				{:else if 0 < shortcut.length && '' === value}
					<Kbd keys={shortcut} />
				{/if}
			{/snippet}
		</Input>
		<p id={resultId} class="results" role="status">
			{#if null !== resultCount && '' !== value}{t('inputs.search.results', { count: resultCount })}{/if}
		</p>
	</Stack>
</Stack>

<style>
	.results {
		color: var(--search-results-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		letter-spacing: var(--text-sm-tracking);
		font-variant-numeric: tabular-nums;
		overflow-wrap: anywhere;
	}

	.results:empty {
		display: none;
	}
</style>
