<script lang="ts" module>
	export interface BreadcrumbSegment {
		label: string;
		href: string;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';
	import Menu from './Menu.svelte';
	import PathInput from './PathInput.svelte';
	import { t } from '$lib/copy/t';
	import { validatePath } from '$lib/format/path';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'ondrop' | 'children'> {
		segments: BreadcrumbSegment[];
		label: string;
		editable?: boolean;
		onpath?: (path: string) => void;
		droppable?: boolean;
		ondrop?: (targetPath: string, event: DragEvent) => void;
	}

	let { segments, label, editable = false, onpath, droppable = false, ondrop, class: className = '', ...rest }: Props = $props();

	const collapsed = $derived(4 < segments.length);
	const head = $derived(collapsed ? segments.slice(0, 1) : []);
	const tail = $derived(collapsed ? segments.slice(-2) : segments);
	const hidden = $derived(collapsed ? segments.slice(1, -2) : []);
	const last = $derived(segments.at(-1));

	const hiddenItems = $derived(
		hidden.map((segment) => ({
			id: segment.href,
			label: segment.label,
			onselect: () => {
				window.location.assign(segment.href);
			}
		}))
	);

	let editing = $state(false);
	let draft = $state('');
	let dropTarget = $state<string | null>(null);

	const startEdit = () => {
		if (editable) {
			draft = undefined === last ? '' : last.href;
			editing = true;
		}
	};

	const submit = () => {
		const check = validatePath(draft);
		if (true === check.ok) {
			onpath?.(draft);
			editing = false;
		}
	};

	const onTrailClick = (event: MouseEvent) => {
		if (event.target === event.currentTarget) {
			startEdit();
		}
	};

	const onNavKey = (event: KeyboardEvent) => {
		if (editable && !editing && 'l' === event.key.toLowerCase() && (event.metaKey || event.ctrlKey)) {
			event.preventDefault();
			startEdit();
		}
	};

	const onEditorKey = (event: KeyboardEvent) => {
		if ('Escape' === event.key) {
			editing = false;
		}
	};

	const acceptDrop = (event: DragEvent) => {
		if (droppable) {
			event.preventDefault();
		}
	};
</script>

<nav {...rest} aria-label={label} class="breadcrumbs {className}" onkeydown={onNavKey}>
	{#if editing}
		<div class="editor" onkeydown={onEditorKey}>
			<PathInput bind:value={draft} {label} onsubmit={submit} />
		</div>
	{:else}
		<ol class="trail" onclick={onTrailClick}>
			{#each head as segment, index (segment.href)}
				{@render crumb(segment, 0 < index)}
			{/each}
			{#if collapsed}
				<li class="item">
					<Icon name="chevron-right" size="xs" />
					<Menu items={hiddenItems} label={t('common.more')}>
						{#snippet trigger(props)}
							<button {...props} type="button" class="crumb">
								<span aria-hidden="true">…</span>
								<VisuallyHidden>{t('common.more')}</VisuallyHidden>
							</button>
						{/snippet}
					</Menu>
				</li>
			{/if}
			{#each tail as segment, index (segment.href)}
				{@render crumb(segment, 0 < head.length || 0 < index)}
			{/each}
		</ol>
	{/if}
</nav>

{#snippet crumb(segment: BreadcrumbSegment, separated: boolean)}
	<li
		class="item"
		data-state={dropTarget === segment.href ? 'drop-target' : undefined}
		ondragenter={(event) => {
			acceptDrop(event);
			if (droppable) {
				dropTarget = segment.href;
			}
		}}
		ondragover={acceptDrop}
		ondragleave={() => {
			dropTarget = null;
		}}
		ondrop={(event) => {
			if (droppable) {
				event.preventDefault();
				dropTarget = null;
				ondrop?.(segment.href, event);
			}
		}}
	>
		{#if separated}<Icon name="chevron-right" size="xs" />{/if}
		{#if segment === last}
			<span class="crumb current" aria-current="page">{segment.label}</span>
		{:else}
			<a class="crumb" href={segment.href}>{segment.label}</a>
		{/if}
	</li>
{/snippet}

<style>
	.breadcrumbs {
		display: flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
	}

	.trail {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		flex: 1 1 auto;
		min-block-size: var(--target-min, var(--control-h));
		min-inline-size: 0;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
		cursor: text;
	}

	.item {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-inline-size: 0;
		border-radius: var(--radius-md);
	}

	.item[data-state='drop-target'] {
		background: var(--accent-soft);
	}

	.crumb {
		display: inline-flex;
		align-items: center;
		min-block-size: var(--target-min, var(--control-h));
		padding-inline: var(--space-1);
		border: 0;
		border-radius: var(--radius-md);
		background: transparent;
		color: var(--text-muted);
		font: inherit;
		text-decoration: none;
		overflow-wrap: anywhere;
		cursor: pointer;
		transition: background-color var(--dur-micro) var(--ease-in-out);
	}

	@media (hover: hover) {
		.crumb:hover {
			background: var(--surface-hover);
			color: var(--text);
		}
	}

	.crumb:active {
		background: var(--surface-pressed);
	}

	.crumb.current {
		color: var(--text);
		font-weight: 600;
		cursor: default;
	}

	.crumb:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.editor {
		flex: 1 1 auto;
		min-inline-size: 0;
	}
</style>
