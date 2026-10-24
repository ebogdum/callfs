<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import ColumnChooser from '../../molecules/ColumnChooser.svelte';
	import DensityToggle from '../../molecules/DensityToggle.svelte';
	import Menu from '../../molecules/Menu.svelte';
	import type { ColumnDef, Density, MenuItem } from '../../types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		search?: Snippet;
		filters?: Snippet;
		columns: ColumnDef<unknown>[];
		visible: string[];
		order: string[];
		density?: Density;
		onexport?: (format: 'csv' | 'json') => void;
		extra?: Snippet;
		controls?: boolean;
		class?: string;
	}

	let {
		search,
		filters,
		columns,
		visible = $bindable(),
		order = $bindable(),
		density = $bindable('default'),
		onexport,
		extra,
		controls = true,
		class: className,
		...rest
	}: Props = $props();

	const exportItems: MenuItem[] = $derived([
		{ id: 'csv', label: t('datatable.toolbar.exportCsv'), onselect: () => onexport?.('csv') },
		{ id: 'json', label: t('datatable.toolbar.exportJson'), onselect: () => onexport?.('json') }
	]);
</script>

<div class={['table-toolbar', className]} {...rest}>
	{#if search || filters || extra}
		<div class="group lead">
			{#if search}{@render search()}{/if}
			{#if filters}{@render filters()}{/if}
			{#if extra}{@render extra()}{/if}
		</div>
	{/if}
	{#if controls || onexport}
		<div class="group trail">
			{#if controls}
				<ColumnChooser {columns} bind:visible bind:order />
				<DensityToggle bind:value={density} />
			{/if}
			{#if onexport}
				<Menu items={exportItems} label={t('datatable.toolbar.export')}>
					{#snippet trigger(props)}
						<button {...props} type="button" class="export-trigger">{t('datatable.toolbar.export')}</button>
					{/snippet}
				</Menu>
			{/if}
		</div>
	{/if}
</div>

<style>
	.table-toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		padding-inline: var(--table-edge-px);
	}

	.group {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.lead {
		flex: 1 1 auto;
	}

	.export-trigger {
		display: inline-flex;
		align-items: center;
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		border: var(--hairline) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface);
		color: var(--text);
		font: inherit;
		font-size: var(--text-sm-size);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	@media (hover: hover) {
		.export-trigger:hover {
			background: var(--surface-hover);
		}
	}

	.export-trigger:focus-visible {
		box-shadow: var(--focus-ring);
	}
</style>
