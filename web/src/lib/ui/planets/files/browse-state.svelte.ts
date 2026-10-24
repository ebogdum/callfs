import type { FileEntry } from '$lib/api/types';
import type { Selection, SortState } from '$lib/ui/types';
import type { InspectorTabId } from '../../organisms/Inspector.svelte';

export type BrowseView = 'list' | 'grid';
export type BrowseSortKey = 'name' | 'size' | 'mtime' | 'type';
export type BrowseSortDir = 'asc' | 'desc';

export interface BrowseSort {
	key: BrowseSortKey;
	dir: BrowseSortDir;
}

export interface BrowseDefaults {
	view: BrowseView;
	hidden: boolean;
	sort: BrowseSort;
}

export const ROOT_PATH = '/';
export const DEFAULT_TAB: InspectorTabId = 'details';

const VIEWS: readonly BrowseView[] = ['list', 'grid'];
const SORT_KEYS: readonly BrowseSortKey[] = ['name', 'size', 'mtime', 'type'];
const SORT_DIRS: readonly BrowseSortDir[] = ['asc', 'desc'];
const TABS: readonly InspectorTabId[] = ['details', 'placement', 'durability', 'links', 'permissions', 'activity'];

export const oneOf = <T extends string>(options: readonly T[], value: string | null, fallback: T): T =>
	null !== value && (options as readonly string[]).includes(value) ? (value as T) : fallback;

export const hrefFor = (path: string): string =>
	ROOT_PATH === path ? '/files' : `/files${path.split('/').map(encodeURIComponent).join('/')}`;

export const pathFromRoute = (routePath: string | undefined): string => `/${routePath ?? ''}`;

export class BrowseState {
	path = $state(ROOT_PATH);
	entries = $state.raw<FileEntry[]>([]);
	selection = $state<Selection>({ mode: 'multi', ids: new Set<string>(), allMatching: false });
	q = $state('');
	view = $state<BrowseView>('list');
	showHidden = $state(false);
	sort = $state<BrowseSort>({ key: 'name', dir: 'asc' });
	inspect = $state<string | null>(null);
	tab = $state<InspectorTabId>(DEFAULT_TAB);
	preview = $state<string | null>(null);
	readonly #navigateTo: (href: string) => void;

	constructor(navigateTo: (href: string) => void) {
		this.#navigateTo = navigateTo;
	}

	navigate(path: string): void {
		const params = this.#params(false);
		this.#navigateTo(`${hrefFor(path)}?${params.toString()}`);
	}

	setSort(next: SortState | null): void {
		if (null === next) {
			return;
		}
		this.sort = { key: oneOf(SORT_KEYS, next.key, this.sort.key), dir: oneOf(SORT_DIRS, next.dir, this.sort.dir) };
	}

	setTab(next: string): void {
		this.tab = oneOf(TABS, next, DEFAULT_TAB);
	}

	hydrate(path: string, params: URLSearchParams, defaults: BrowseDefaults): void {
		this.path = path;
		this.q = params.get('q') ?? '';
		this.view = oneOf(VIEWS, params.get('view'), defaults.view);
		const hidden = params.get('hidden');
		this.showHidden = null === hidden ? defaults.hidden : 'true' === hidden;
		this.sort = {
			key: oneOf(SORT_KEYS, params.get('sort'), defaults.sort.key),
			dir: oneOf(SORT_DIRS, params.get('order'), defaults.sort.dir)
		};
		this.inspect = params.get('inspect');
		this.tab = oneOf(TABS, params.get('tab'), DEFAULT_TAB);
		this.preview = params.get('preview');
		const ids = params.getAll('sel');
		this.selection = { mode: 'multi', ids: new Set(ids), allMatching: false };
	}

	search(): string {
		return `?${this.#params(true).toString()}`;
	}

	#params(full: boolean): URLSearchParams {
		const params = new URLSearchParams();
		if (full && '' !== this.q) {
			params.set('q', this.q);
		}
		params.set('view', this.view);
		params.set('sort', this.sort.key);
		params.set('order', this.sort.dir);
		params.set('hidden', String(this.showHidden));
		if (!full) {
			return params;
		}
		if (null !== this.inspect) {
			params.set('inspect', this.inspect);
			if (DEFAULT_TAB !== this.tab) {
				params.set('tab', this.tab);
			}
		}
		if (null !== this.preview) {
			params.set('preview', this.preview);
		}
		for (const id of this.selection.ids) {
			params.append('sel', id);
		}
		return params;
	}
}
