<script lang="ts">
	import { Popover } from 'bits-ui';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Checkbox from '../atoms/Checkbox.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import Surface from '../quarks/Surface.svelte';
	import type { ColumnDef } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		columns: ColumnDef<unknown>[];
		visible: string[];
		order: string[];
		class?: string;
	}

	let { columns, visible = $bindable(), order = $bindable(), class: className, ...rest }: Props = $props();

	const orderedColumns = $derived.by((): ColumnDef<unknown>[] => {
		const listed = order
			.map((id) => columns.find((column) => id === column.id))
			.filter((column): column is ColumnDef<unknown> => undefined !== column);
		const unlisted = columns.filter((column) => !order.includes(column.id));
		return [...listed, ...unlisted];
	});

	const orderedIds = $derived(orderedColumns.map((column) => column.id));

	function setShown(id: string, shown: boolean): void {
		visible = shown ? [...visible, id] : visible.filter((candidate) => candidate !== id);
	}

	function move(id: string, delta: -1 | 1): void {
		const ids = [...orderedIds];
		const index = ids.indexOf(id);
		const target = index + delta;
		if (0 > target || target >= ids.length) {
			return;
		}
		ids.splice(index, 1);
		ids.splice(target, 0, id);
		order = ids;
	}

	function resetColumns(): void {
		order = columns.map((column) => column.id);
		visible = columns.filter((column) => !column.hidden).map((column) => column.id);
	}
</script>

<div class={['column-chooser', className]} {...rest}>
	<Popover.Root>
		<Popover.Trigger>
			{#snippet child({ props })}
				<Button variant="secondary" size="sm" iconStart="columns" {...props}>
					{t('tablecontrols.columns.trigger')}
				</Button>
			{/snippet}
		</Popover.Trigger>
		<Popover.Portal>
			<Popover.Content sideOffset={8} align="end">
				<Surface level="overlay" pad="2" radius="lg">
					<Stack gap="2">
						<Stack gap="1" role="group" aria-label={t('tablecontrols.columns.group')}>
							{#each orderedColumns as column, index (column.id)}
								<Cluster gap="2" justify="space-between" wrap={false}>
									<Checkbox
										bind:checked={() => visible.includes(column.id), (shown) => setShown(column.id, shown)}
										label={column.header}
									/>
									<Cluster gap="1">
										<Button
											variant="ghost"
											size="sm"
											disabled={0 === index}
											aria-label={t('tablecontrols.columns.move_up_label', { name: column.header })}
											onclick={() => move(column.id, -1)}
										>
											{t('tablecontrols.columns.move_up')}
										</Button>
										<Button
											variant="ghost"
											size="sm"
											disabled={index === orderedIds.length - 1}
											aria-label={t('tablecontrols.columns.move_down_label', { name: column.header })}
											onclick={() => move(column.id, 1)}
										>
											{t('tablecontrols.columns.move_down')}
										</Button>
									</Cluster>
								</Cluster>
							{/each}
						</Stack>
						<Button variant="ghost" size="sm" onclick={resetColumns}>
							{t('tablecontrols.columns.reset')}
						</Button>
					</Stack>
				</Surface>
			</Popover.Content>
		</Popover.Portal>
	</Popover.Root>
</div>
