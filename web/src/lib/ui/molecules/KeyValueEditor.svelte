<script lang="ts" module>
	export const TOKENS = ['--key-value-editor-gap'] as const;

	let instanceCount = 0;
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import IconButton from '../atoms/IconButton.svelte';
	import Input from '../atoms/Input.svelte';
	import Text from '../atoms/Text.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface EntryRow {
		id: number;
		key: string;
		value: string;
	}

	interface RowIssue {
		keyError: string | null;
		valueError: string | null;
	}

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		entries?: Record<string, string>;
		keyLabel: string;
		valueLabel: string;
		validateKey?: (key: string) => string | null;
		validateValue?: (value: string) => string | null;
		addLabel: string;
	}

	let {
		entries = $bindable({}),
		keyLabel,
		valueLabel,
		validateKey,
		validateValue,
		addLabel,
		class: className,
		...rest
	}: Props = $props();

	const idPrefix = `key-value-${++instanceCount}`;

	let nextRowId = 0;

	function createRow(key = '', value = ''): EntryRow {
		nextRowId += 1;
		return { id: nextRowId, key, value };
	}

	let rows = $state<EntryRow[]>(
		untrack(() => Object.entries(entries).map(([key, value]) => createRow(key, value)))
	);

	function issuesFor(current: EntryRow[]): RowIssue[] {
		const keyCounts = new Map<string, number>();
		for (const row of current) {
			const key = row.key.trim();
			if ('' !== key) {
				keyCounts.set(key, (keyCounts.get(key) ?? 0) + 1);
			}
		}
		return current.map((row): RowIssue => {
			const key = row.key.trim();
			if ('' === key) {
				return { keyError: '' === row.value.trim() ? null : t('editors.kv.key_required'), valueError: null };
			}
			const customKeyError = validateKey?.(key) ?? null;
			if (null !== customKeyError) {
				return { keyError: customKeyError, valueError: null };
			}
			if (1 < (keyCounts.get(key) ?? 0)) {
				return { keyError: t('editors.kv.duplicate', { key }), valueError: null };
			}
			return { keyError: null, valueError: validateValue?.(row.value.trim()) ?? null };
		});
	}

	const issues = $derived(issuesFor(rows));
	const hasErrors = $derived(issues.some((issue) => null !== issue.keyError || null !== issue.valueError));

	function commit(): void {
		if (hasErrors) {
			return;
		}
		entries = Object.fromEntries(
			rows
				.filter((row) => '' !== row.key.trim())
				.map((row): [string, string] => [row.key.trim(), row.value.trim()])
		);
	}

	function addRow(): void {
		rows = [...rows, createRow()];
		commit();
	}

	function removeRow(id: number): void {
		rows = rows.filter((row) => id !== row.id);
		commit();
	}
</script>

<div class={['key-value-editor', className]} data-state={hasErrors ? 'invalid' : 'valid'} {...rest}>
	<Stack gap="2">
		{#if 0 < rows.length}
			<div class="entry">
				<Text size="caption" tone="muted">{keyLabel}</Text>
				<Text size="caption" tone="muted">{valueLabel}</Text>
				<span aria-hidden="true"></span>
			</div>
		{/if}
		{#each rows as row, index (row.id)}
			{@const issue = issues[index]}
			{@const keyErrorId = `${idPrefix}-key-${row.id}-error`}
			{@const valueErrorId = `${idPrefix}-value-${row.id}-error`}
			<div class="entry">
				<Stack gap="1">
					<Input
						bind:value={row.key}
						mono
						autocomplete="off"
						aria-label={t('editors.kv.row_label', { label: keyLabel, row: index + 1 })}
						invalid={null !== issue.keyError}
						aria-describedby={null !== issue.keyError ? keyErrorId : undefined}
						oninput={commit}
					/>
					{#if null !== issue.keyError}
						<FieldError id={keyErrorId} message={issue.keyError} />
					{/if}
				</Stack>
				<Stack gap="1">
					<Input
						bind:value={row.value}
						autocomplete="off"
						aria-label={t('editors.kv.row_label', { label: valueLabel, row: index + 1 })}
						invalid={null !== issue.valueError}
						aria-describedby={null !== issue.valueError ? valueErrorId : undefined}
						oninput={commit}
					/>
					{#if null !== issue.valueError}
						<FieldError id={valueErrorId} message={issue.valueError} />
					{/if}
				</Stack>
				<IconButton
					icon="x"
					label={t('editors.kv.remove', { key: row.key.trim() || t('editors.kv.new_row') })}
					onclick={() => removeRow(row.id)}
				/>
			</div>
		{:else}
			<Text tone="muted">{t('editors.kv.empty')}</Text>
		{/each}
		<div>
			<Button variant="secondary" iconStart="plus" onclick={addRow}>{addLabel}</Button>
		</div>
	</Stack>
</div>

<style>
	.key-value-editor {
		min-inline-size: 0;
	}

	.entry {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--key-value-editor-gap, var(--space-2));
	}
</style>
