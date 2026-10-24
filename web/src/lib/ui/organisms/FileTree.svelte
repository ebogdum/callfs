<script lang="ts" module>
	export const TOKENS = ['--file-tree-current-bg'] as const;

	interface TreeRow {
		path: string;
		name: string;
		depth: number;
		expanded: boolean;
		loaded: boolean;
		leaf: boolean;
		state: 'idle' | 'loading' | 'error';
	}
</script>

<script lang="ts">
	import { tick, untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { FileEntry, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import Spinner from '../quarks/Spinner.svelte';
	import type { UiError } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children' | 'ondrop'> {
		root: string;
		load: (path: string) => Promise<Result<FileEntry[], UiError>>;
		current: string;
		onnavigate: (path: string) => void;
		ondrop?: (path: string, event: DragEvent) => void;
		class?: string;
	}

	let { root, load, current, onnavigate, ondrop, class: className, ...rest }: Props = $props();

	let rows = $state<TreeRow[]>([]);
	let focusedPath = $state('');
	let treeElement = $state<HTMLDivElement | null>(null);

	const rootRow = (path: string): TreeRow => ({
		path,
		name: t('files.tree.root'),
		depth: 0,
		expanded: false,
		loaded: false,
		leaf: false,
		state: 'idle'
	});

	const patchRow = (path: string, patch: Partial<TreeRow>): void => {
		const row = rows.find((candidate) => path === candidate.path);
		if (undefined !== row) {
			Object.assign(row, patch);
		}
	};

	const loadChildren = async (path: string): Promise<void> => {
		patchRow(path, { state: 'loading' });
		const result = await load(path);
		if (!result.ok) {
			patchRow(path, { state: 'error', expanded: false });
			return;
		}
		const index = rows.findIndex((candidate) => path === candidate.path);
		const parent = rows[index];
		if (undefined === parent) {
			return;
		}
		const children = result.value
			.filter((entry) => 'directory' === entry.type)
			.map(
				(entry): TreeRow => ({
					path: entry.path,
					name: entry.name,
					depth: parent.depth + 1,
					expanded: false,
					loaded: false,
					leaf: false,
					state: 'idle'
				})
			);
		rows.splice(index + 1, 0, ...children);
		patchRow(path, { state: 'idle', loaded: true, leaf: 0 === children.length });
	};

	const inflight = new Map<string, Promise<void>>();

	const trimSlash = (path: string): string => (1 < path.length && path.endsWith('/') ? path.slice(0, -1) : path);
	const samePath = (left: string, right: string): boolean => trimSlash(left) === trimSlash(right);

	const expandRow = async (path: string): Promise<void> => {
		const row = rows.find((candidate) => samePath(path, candidate.path));
		if (undefined === row || row.leaf) {
			return;
		}
		const pending = inflight.get(row.path);
		if (undefined !== pending) {
			await pending;
			return;
		}
		if (row.expanded) {
			return;
		}
		row.expanded = true;
		if (!row.loaded) {
			const job = loadChildren(row.path);
			inflight.set(row.path, job);
			await job;
			inflight.delete(row.path);
		}
	};

	const ancestorsOf = (rootPath: string, target: string): string[] => {
		const base = trimSlash(rootPath);
		const goal = trimSlash(target);
		if (!samePath(base, goal) && !goal.startsWith('/' === base ? '/' : `${base}/`)) {
			return [];
		}
		const segments = goal.slice(base.length).split('/').filter((segment) => '' !== segment);
		const prefix = '/' === base ? '' : base;
		return [base, ...segments.slice(0, -1).map((_, index) => `${prefix}/${segments.slice(0, index + 1).join('/')}`)];
	};

	const reveal = async (rootPath: string, target: string): Promise<void> => {
		for (const path of ancestorsOf(rootPath, target)) {
			await expandRow(path);
		}
		const match = rows.find((row) => samePath(target, row.path));
		if (undefined !== match) {
			focusedPath = match.path;
		}
	};

	$effect(() => {
		const rootPath = root;
		untrack(() => {
			rows = [rootRow(rootPath)];
			focusedPath = rootPath;
			void expandRow(rootPath);
		});
	});

	$effect(() => {
		const rootPath = root;
		const target = current;
		untrack(() => {
			void reveal(rootPath, target);
		});
	});

	const visibleRows = $derived.by(() => {
		const visible: TreeRow[] = [];
		let hiddenBelowDepth: number | null = null;
		for (const row of rows) {
			if (null !== hiddenBelowDepth && row.depth > hiddenBelowDepth) {
				continue;
			}
			hiddenBelowDepth = null;
			visible.push(row);
			if (!row.expanded) {
				hiddenBelowDepth = row.depth;
			}
		}
		return visible;
	});

	const activeIndex = $derived.by(() => {
		const index = visibleRows.findIndex((row) => focusedPath === row.path);
		return 0 > index ? 0 : index;
	});

	const focusRow = async (path: string): Promise<void> => {
		focusedPath = path;
		await tick();
		const items: HTMLElement[] = treeElement
			? Array.from(treeElement.querySelectorAll<HTMLElement>('[role="treeitem"]'))
			: [];
		items.find((item) => path === item.dataset.path)?.focus();
	};

	const moveFocus = (target: number): void => {
		const row = visibleRows[Math.min(visibleRows.length - 1, Math.max(0, target))];
		if (undefined !== row) {
			void focusRow(row.path);
		}
	};

	const parentOf = (index: number): TreeRow | undefined => {
		const depth = visibleRows[index]?.depth ?? 0;
		for (let candidateIndex = index - 1; 0 <= candidateIndex; candidateIndex -= 1) {
			const candidate = visibleRows[candidateIndex];
			if (undefined !== candidate && candidate.depth === depth - 1) {
				return candidate;
			}
		}
		return undefined;
	};

	const findByLetter = (index: number, letter: string): TreeRow | undefined => {
		const count = visibleRows.length;
		for (let offset = 1; offset <= count; offset += 1) {
			const candidate = visibleRows[(index + offset) % count];
			if (undefined !== candidate && candidate.name.toLowerCase().startsWith(letter)) {
				return candidate;
			}
		}
		return undefined;
	};

	const handleKeydown = (event: KeyboardEvent): void => {
		const index = activeIndex;
		const row = visibleRows[index];
		if (undefined === row) {
			return;
		}
		switch (event.key) {
			case 'ArrowDown':
				event.preventDefault();
				moveFocus(index + 1);
				return;
			case 'ArrowUp':
				event.preventDefault();
				moveFocus(index - 1);
				return;
			case 'Home':
				event.preventDefault();
				moveFocus(0);
				return;
			case 'End':
				event.preventDefault();
				moveFocus(visibleRows.length - 1);
				return;
			case 'ArrowRight':
				event.preventDefault();
				if (row.expanded) {
					moveFocus(index + 1);
				} else {
					void expandRow(row.path);
				}
				return;
			case 'ArrowLeft': {
				event.preventDefault();
				if (row.expanded) {
					patchRow(row.path, { expanded: false });
					return;
				}
				const parent = parentOf(index);
				if (undefined !== parent) {
					void focusRow(parent.path);
				}
				return;
			}
			case 'Enter':
				event.preventDefault();
				onnavigate(row.path);
				return;
			default: {
				if (1 === event.key.length && !event.ctrlKey && !event.metaKey) {
					const match = findByLetter(index, event.key.toLowerCase());
					if (undefined !== match) {
						void focusRow(match.path);
					}
				}
			}
		}
	};

	const toggleRow = (event: MouseEvent, row: TreeRow): void => {
		event.stopPropagation();
		if (row.expanded) {
			patchRow(row.path, { expanded: false });
		} else {
			void expandRow(row.path);
		}
	};

	const allowDrop = (event: DragEvent): void => {
		if (undefined !== ondrop) {
			event.preventDefault();
		}
	};
</script>

<div
	class={['file-tree', className]}
	role="tree"
	aria-label={t('files.tree.label')}
	bind:this={treeElement}
	onkeydown={handleKeydown}
	{...rest}
>
	{#each visibleRows as row, index (row.path)}
		<div
			class="row"
			role="treeitem"
			tabindex={index === activeIndex ? 0 : -1}
			aria-level={row.depth + 1}
			aria-expanded={row.leaf ? undefined : row.expanded}
			aria-selected={samePath(current, row.path)}
			data-path={row.path}
			data-state={samePath(current, row.path) ? 'selected' : undefined}
			style:padding-inline-start={`calc(var(--space-3) * ${row.depth} + var(--space-2))`}
			onclick={() => onnavigate(row.path)}
			onfocus={() => {
				focusedPath = row.path;
			}}
			ondragover={allowDrop}
			ondrop={(event: DragEvent) => ondrop?.(row.path, event)}
		>
			<span class="toggle" aria-hidden="true" onclick={(event: MouseEvent) => toggleRow(event, row)}>
				{#if !row.leaf}
					<Icon name={row.expanded ? 'chevron-down' : 'chevron-right'} size="xs" />
				{/if}
			</span>
			<Icon name="folder" size="sm" />
			<span class="name">{row.name}</span>
			{#if 'loading' === row.state}
				<Spinner size="sm" label={t('files.tree.loading')} />
			{:else if 'error' === row.state}
				<span class="error">{t('files.tree.error')}</span>
			{/if}
		</div>
	{/each}
</div>

<style>
	.file-tree {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
		overflow-y: auto;
	}

	.row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		min-inline-size: 0;
		padding-block: var(--space-1);
		padding-inline-end: var(--space-2);
		border-radius: var(--radius-sm);
		color: var(--text);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	@media (hover: hover) {
		.row:hover {
			background: var(--surface-hover);
		}
	}

	.row[data-state='selected'] {
		background: var(--file-tree-current-bg, var(--surface-selected));
	}

	.row:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex: none;
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
		color: var(--text-muted);
	}

	.name {
		flex: 1 1 auto;
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.error {
		flex: none;
		color: var(--danger-text);
	}
</style>
