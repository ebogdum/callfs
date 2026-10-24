import type { ColumnDef, Density, Selection } from '../../types';

export const VIRTUAL_THRESHOLD = 200;
export const DEFAULT_OVERSCAN = 8;
export const DEFAULT_VIEWPORT_PX = 480;
export const DEFAULT_COLUMN_WIDTH_PX = 160;
export const MIN_COLUMN_WIDTH_PX = 64;
export const MAX_COLUMN_WIDTH_PX = 1200;

const ROW_HEIGHT_PX: Record<Density, number> = { compact: 32, default: 40, comfortable: 48 };
const COARSE_ROW_HEIGHT_PX = 48;

export class TableCursor {
	active = $state(0);
	anchor = $state<number | null>(null);
}

export const emptySelection = (): Selection => ({ mode: 'none', ids: new Set<string>(), allMatching: false });

export const rowHeightPx = (density: Density, coarse: boolean): number =>
	coarse ? Math.max(ROW_HEIGHT_PX[density], COARSE_ROW_HEIGHT_PX) : ROW_HEIGHT_PX[density];

export const isCoarsePointer = (): boolean =>
	'undefined' !== typeof window &&
	'function' === typeof window.matchMedia &&
	window.matchMedia('(pointer: coarse)').matches;

export const clampIndex = (index: number, count: number): number =>
	Math.min(Math.max(0, index), Math.max(0, count - 1));

export const destinationFor = (key: string, index: number, count: number): number | null => {
	if ('ArrowDown' === key || 'j' === key) {
		return clampIndex(index + 1, count);
	}
	if ('ArrowUp' === key || 'k' === key) {
		return clampIndex(index - 1, count);
	}
	if ('Home' === key) {
		return clampIndex(0, count);
	}
	if ('End' === key) {
		return clampIndex(count - 1, count);
	}
	return null;
};

export const visibleRange = (
	count: number,
	rowHeight: number,
	overscan: number,
	scrollTop: number,
	viewport: number
): { first: number; last: number } => {
	const height = 0 < viewport ? viewport : DEFAULT_VIEWPORT_PX;
	const first = Math.min(count, Math.max(0, Math.floor(scrollTop / rowHeight) - overscan));
	const last = Math.min(count, Math.ceil((scrollTop + height) / rowHeight) + overscan);
	return { first, last: Math.max(first, last) };
};

export const orderedVisibleColumns = <T>(
	columns: ColumnDef<T>[],
	order: string[],
	visible: string[]
): ColumnDef<T>[] => {
	const byId = new Map(columns.map((column) => [column.id, column] as const));
	const ids = [
		...order.filter((id) => byId.has(id)),
		...columns.map((column) => column.id).filter((id) => !order.includes(id))
	];
	return ids
		.filter((id) => visible.includes(id))
		.map((id) => byId.get(id))
		.filter((column): column is ColumnDef<T> => undefined !== column);
};

export const isPageSelected = (selection: Selection, keys: string[]): boolean =>
	0 < keys.length && keys.every((key) => selection.ids.has(key));

export const toggleRow = (selection: Selection, key: string): Selection => {
	if ('none' === selection.mode) {
		return selection;
	}
	const ids = new Set(selection.ids);
	if (ids.has(key)) {
		ids.delete(key);
	} else {
		if ('single' === selection.mode) {
			ids.clear();
		}
		ids.add(key);
	}
	return { mode: selection.mode, ids, allMatching: false };
};

export const selectRange = (selection: Selection, keys: string[], from: number, to: number): Selection => {
	if ('multi' !== selection.mode) {
		return selection;
	}
	const ids = new Set(selection.ids);
	for (let index = Math.min(from, to); index <= Math.max(from, to); index += 1) {
		const key = keys[index];
		if (undefined !== key) {
			ids.add(key);
		}
	}
	return { mode: selection.mode, ids, allMatching: false };
};

export const selectPage = (selection: Selection, keys: string[]): Selection => {
	if ('multi' !== selection.mode) {
		return selection;
	}
	return { mode: selection.mode, ids: new Set([...selection.ids, ...keys]), allMatching: false };
};

export const deselectPage = (selection: Selection, keys: string[]): Selection => {
	if ('multi' !== selection.mode) {
		return selection;
	}
	const ids = new Set(selection.ids);
	for (const key of keys) {
		ids.delete(key);
	}
	return { mode: selection.mode, ids, allMatching: false };
};

export const selectAllMatching = (selection: Selection): Selection =>
	'multi' === selection.mode ? { ...selection, allMatching: true } : selection;

export const clearSelection = (selection: Selection): Selection => ({
	mode: selection.mode,
	ids: new Set<string>(),
	allMatching: false
});
