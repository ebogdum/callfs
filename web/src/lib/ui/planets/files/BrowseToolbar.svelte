<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import type { MenuItem } from '$lib/ui/types';
	import Button from '../../atoms/Button.svelte';
	import IconButton from '../../atoms/IconButton.svelte';
	import Input from '../../atoms/Input.svelte';
	import Icon from '../../quarks/Icon.svelte';
	import Menu from '../../molecules/Menu.svelte';
	import SegmentedControl from '../../molecules/SegmentedControl.svelte';
	import type { BrowseState } from './browse-state.svelte';
	import type { FileActions } from './file-actions.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		state: BrowseState;
		actions: FileActions;
		treeOpen: boolean;
		class?: string;
	}

	let { state: browse, actions, treeOpen = $bindable(), class: className, ...rest }: Props = $props();

	let filesInput = $state<HTMLInputElement | null>(null);
	let folderInput = $state<HTMLInputElement | null>(null);

	const filterId = 'browse-filter';
	const hasFilter = $derived(0 < browse.q.trim().length);
	const searchHref = $derived(`${base}/files/search?q=${encodeURIComponent(browse.q.trim())}`);

	const uploadItems: MenuItem[] = [
		{ id: 'upload-files', label: t('fileactions.upload_files'), icon: 'file', shortcut: ['u'], onselect: () => filesInput?.click() },
		{ id: 'upload-folder', label: t('fileactions.upload_folder'), icon: 'folder-open', onselect: () => folderInput?.click() }
	];

	const moreItems = $derived<MenuItem[]>([
		{
			id: 'show-hidden',
			label: t('fileactions.show_hidden'),
			icon: browse.showHidden ? 'eye' : 'eye-off',
			shortcut: ['.'],
			checked: browse.showHidden,
			group: t('fileactions.group.view'),
			onselect: () => (browse.showHidden = !browse.showHidden)
		},
		...actions.toolbarItems().map((item) => ({ ...item, group: t('fileactions.group.folder') }))
	]);

	function pick(event: Event): void {
		const input = event.currentTarget as HTMLInputElement;
		const files = Array.from(input.files ?? []);
		input.value = '';
		actions.upload(files);
	}

	function onFilterKeydown(event: KeyboardEvent): void {
		if ('Enter' === event.key && (event.metaKey || event.ctrlKey) && hasFilter) {
			event.preventDefault();
			void goto(searchHref);
		} else if ('Escape' === event.key && hasFilter) {
			event.preventDefault();
			browse.q = '';
		}
	}

	const viewOptions = [
		{ value: 'list', label: t('fileactions.view.list'), icon: 'list' as const },
		{ value: 'grid', label: t('fileactions.view.grid'), icon: 'grid' as const }
	];
</script>

<div class={['browse-toolbar', className]} role="group" aria-label={t('fileactions.toolbar.label')} {...rest}>
	<div class="filter">
		<Input id={filterId} type="search" aria-label={t('fileactions.filter.label')} clearable bind:value={browse.q} placeholder={t('fileactions.filter.placeholder')} onkeydown={onFilterKeydown}>
			{#snippet prefix()}
				<Icon name="filter" size="sm" />
			{/snippet}
		</Input>
		{#if hasFilter}
			<a class="search-everywhere" href={searchHref}>
				<Icon name="search" size="sm" />
				<span>{t('fileactions.search_everywhere')}</span>
			</a>
		{/if}
	</div>

	<div class="end">
		<div class="view">
			<IconButton
				icon="sidebar"
				label={treeOpen ? t('fileactions.tree.hide') : t('fileactions.tree.show')}
				pressed={treeOpen}
				class="tree-toggle"
				onclick={() => (treeOpen = !treeOpen)}
			/>
			<SegmentedControl bind:value={browse.view} options={viewOptions} label={t('fileactions.view.label')} />
			<Menu items={moreItems} label={t('fileactions.more')}>
				{#snippet trigger(props)}
					<IconButton {...props} icon="more" label={t('fileactions.more')} variant="secondary" />
				{/snippet}
			</Menu>
		</div>

		<div class="create">
			<span class="wide">
				<Button variant="secondary" iconStart="folder-plus" onclick={() => void actions.newFolder()}>
					{t('fileactions.new_folder')}
				</Button>
			</span>
			<span class="narrow">
				<IconButton icon="folder-plus" label={t('fileactions.new_folder')} variant="secondary" onclick={() => void actions.newFolder()} />
			</span>
			<Menu items={uploadItems} label={t('fileactions.upload')}>
				{#snippet trigger(props)}
					<Button {...props} variant="primary" iconStart="upload" iconEnd="chevron-down">{t('fileactions.upload')}</Button>
				{/snippet}
			</Menu>
		</div>
	</div>

	<input bind:this={filesInput} type="file" multiple hidden class="picker" onchange={pick} />
	<input bind:this={folderInput} type="file" multiple webkitdirectory hidden class="picker" onchange={pick} />
</div>

<style>
	.browse-toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
		flex: 1 1 100%;
		min-inline-size: 0;
	}

	.filter {
		display: flex;
		flex: 1 1 calc(var(--space-16) * 4);
		align-items: center;
		gap: var(--space-3);
		min-inline-size: 0;
		max-inline-size: var(--inspector-w);
	}

	.filter > :global(*:first-of-type) {
		flex: 1 1 auto;
		min-inline-size: 0;
	}

	.view,
	.create {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.end {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-4);
	}

	.create {
		padding-inline-start: var(--space-4);
		border-inline-start: var(--hairline) solid var(--border-subtle);
	}

	.picker,
	.narrow {
		display: none;
	}

	.wide {
		display: contents;
	}

	.search-everywhere {
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: var(--space-1-5);
		min-block-size: var(--target-min);
		padding-inline: var(--space-1);
		border-radius: var(--radius-sm);
		font-size: var(--text-sm-size);
		font-weight: var(--text-label-weight);
		color: var(--text-link);
		text-decoration: none;
		white-space: nowrap;
	}

	@media (hover: hover) {
		.search-everywhere:hover {
			text-decoration: underline;
		}
	}

	.search-everywhere:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (width < 1024px) {
		.view :global(.tree-toggle) {
			display: none;
		}
	}

	@media (width < 640px) {
		.filter {
			flex-basis: 100%;
			max-inline-size: none;
		}

		.end {
			flex: 1 1 100%;
			justify-content: space-between;
			gap: var(--space-2);
		}

		.wide {
			display: none;
		}

		.narrow {
			display: contents;
		}

		.create {
			padding-inline-start: 0;
			border-inline-start: 0;
		}
	}
</style>
