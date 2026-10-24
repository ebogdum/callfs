<script lang="ts" module>
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
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface ItemRow {
		id: number;
		value: string;
	}

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		items?: string[];
		label: string;
		itemLabel: string;
		validate?: (item: string) => string | null;
		addLabel: string;
	}

	let {
		items = $bindable([]),
		label,
		itemLabel,
		validate,
		addLabel,
		class: className,
		...rest
	}: Props = $props();

	const idPrefix = `list-editor-${++instanceCount}`;

	let nextRowId = 0;

	function createRow(value = ''): ItemRow {
		nextRowId += 1;
		return { id: nextRowId, value };
	}

	let rows = $state<ItemRow[]>(untrack(() => items.map((item) => createRow(item))));

	const issues = $derived(
		rows.map((row): string | null => {
			const value = row.value.trim();
			if ('' === value) {
				return null;
			}
			return validate?.(value) ?? null;
		})
	);
	const hasErrors = $derived(issues.some((issue) => null !== issue));

	function commit(): void {
		if (hasErrors) {
			return;
		}
		items = rows.filter((row) => '' !== row.value.trim()).map((row) => row.value.trim());
	}

	function addRow(): void {
		rows = [...rows, createRow()];
		commit();
	}

	function removeRow(id: number): void {
		rows = rows.filter((row) => id !== row.id);
		commit();
	}

	function moveRow(index: number, direction: -1 | 1): void {
		const target = index + direction;
		if (0 > target || rows.length <= target) {
			return;
		}
		const next = [...rows];
		next[index] = rows[target];
		next[target] = rows[index];
		rows = next;
		commit();
	}
</script>

<div
	class={['list-editor', className]}
	role="group"
	aria-label={label}
	data-state={hasErrors ? 'invalid' : 'valid'}
	{...rest}
>
	<Stack gap="2">
		<Text size="caption" tone="muted">{label}</Text>
		{#each rows as row, index (row.id)}
			{@const issue = issues[index]}
			{@const errorId = `${idPrefix}-${row.id}-error`}
			{@const itemName = row.value.trim() || t('editors.list.new_item')}
			<Stack gap="1">
				<Cluster gap="2" align="center">
					<Input
						bind:value={row.value}
						autocomplete="off"
						aria-label={t('editors.list.item_label', { label: itemLabel, row: index + 1 })}
						invalid={null !== issue}
						aria-describedby={null !== issue ? errorId : undefined}
						oninput={commit}
					/>
					<IconButton
						icon="arrow-up"
						label={t('editors.list.move_up', { item: itemName })}
						disabledReason={0 === index ? t('editors.list.first_item') : undefined}
						onclick={() => moveRow(index, -1)}
					/>
					<IconButton
						icon="arrow-down"
						label={t('editors.list.move_down', { item: itemName })}
						disabledReason={rows.length - 1 === index ? t('editors.list.last_item') : undefined}
						onclick={() => moveRow(index, 1)}
					/>
					<IconButton
						icon="x"
						label={t('editors.list.remove', { item: itemName })}
						onclick={() => removeRow(row.id)}
					/>
				</Cluster>
				{#if null !== issue}
					<FieldError id={errorId} message={issue} />
				{/if}
			</Stack>
		{:else}
			<Text tone="muted">{t('editors.list.empty')}</Text>
		{/each}
		<div>
			<Button variant="secondary" iconStart="plus" onclick={addRow}>{addLabel}</Button>
		</div>
	</Stack>
</div>
