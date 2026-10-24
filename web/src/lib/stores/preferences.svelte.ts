import { toUiError } from '$lib/api/errors';
import type { FileSort, SortOrder } from '$lib/api/files';
import { filePath, request } from '$lib/api/http';
import type { Result } from '$lib/api/types';
import type { Density, Threshold, UiError } from '$lib/ui/types';

export const PREFERENCES_MIRROR_KEY = 'callfs.ui.prefs.v1';
export const PREFERENCES_SAVE_DELAY_MS = 1000;

export type ThemeMode = 'system' | 'light' | 'dark';
export type ContrastMode = 'standard' | 'more';
export type MotionMode = 'full' | 'reduced' | 'none';
export type RadiusPersonality = 'sharp' | 'medium' | 'round';
export type UiFont = 'inter' | 'system';
export type CodeFont = 'jetbrains-mono' | 'system';
export type FileView = 'list' | 'grid';

export interface FileSortState {
	key: FileSort;
	dir: SortOrder;
}

export interface SavedSearch {
	name: string;
	query: string;
}

export interface TablePreference {
	visible: string[];
	order: string[];
	widths: Record<string, number>;
	density: Density;
	pageSize: number;
}

export interface DashboardPanel {
	id: string;
	title: string;
	metric: string;
	measure: string;
	group: string;
	unit: string;
	chart: 'line' | 'bar' | 'area' | 'stat';
	thresholds: Threshold[];
}

export interface Preferences {
	theme: ThemeMode;
	contrast: ContrastMode;
	density: Density;
	motion: MotionMode;
	fontScale: number;
	accentHue: number;
	neutralChroma: number;
	radius: RadiusPersonality;
	uiFont: UiFont;
	codeFont: CodeFont;
	singleKeyShortcuts: boolean;
	keymapOverrides: Record<string, string[]>;
	sidebarCollapsed: boolean;
	showTechnical: boolean;
	files: {
		view: FileView;
		hidden: boolean;
		columns: string[];
		sort: FileSortState;
		pinned: string[];
		recent: string[];
		savedSearches: SavedSearch[];
	};
	tables: Record<string, TablePreference>;
	dashboards: {
		custom: DashboardPanel[];
		order: string[];
		hidden: string[];
	};
	thresholds: Record<string, Threshold[]>;
	dismissed: string[];
}

export type DeepPartial<T> = T extends unknown[] ? T : T extends object ? { [K in keyof T]?: DeepPartial<T[K]> } : T;

export type PreferencesPatch = DeepPartial<Preferences>;

export type PreferencesStatus = 'loading' | 'ready' | 'error';

const DEFAULT_PREFERENCES: Preferences = {
	theme: 'system',
	contrast: 'standard',
	density: 'default',
	motion: 'full',
	fontScale: 1,
	accentHue: 192,
	neutralChroma: 0.004,
	radius: 'medium',
	uiFont: 'inter',
	codeFont: 'jetbrains-mono',
	singleKeyShortcuts: true,
	keymapOverrides: {},
	sidebarCollapsed: false,
	showTechnical: false,
	files: {
		view: 'list',
		hidden: false,
		columns: [],
		sort: { key: 'name', dir: 'asc' },
		pinned: [],
		recent: [],
		savedSearches: [],
	},
	tables: {},
	dashboards: { custom: [], order: [], hidden: [] },
	thresholds: {},
	dismissed: [],
};

export function isPlainObject(value: unknown): value is Record<string, unknown> {
	return 'object' === typeof value && null !== value && !Array.isArray(value);
}

function conform(fallback: unknown, raw: unknown): unknown {
	if (Array.isArray(fallback)) {
		return Array.isArray(raw) ? [...raw] : [...fallback];
	}
	if (!isPlainObject(fallback)) {
		return typeof raw === typeof fallback ? raw : fallback;
	}
	const source = isPlainObject(raw) ? raw : {};
	if (0 === Object.keys(fallback).length) {
		return { ...source };
	}
	const result: Record<string, unknown> = {};
	for (const key of Object.keys(fallback)) {
		result[key] = conform(fallback[key], source[key]);
	}
	return result;
}

const PERCENT_SCALE_FLOOR = 4;

export function migrate(raw: unknown): Preferences {
	const prefs = conform(DEFAULT_PREFERENCES, raw) as Preferences;
	if (PERCENT_SCALE_FLOOR < prefs.fontScale) {
		return { ...prefs, fontScale: prefs.fontScale / 100 };
	}
	return prefs;
}

export function mergeValues(base: unknown, patch: unknown): unknown {
	if (isPlainObject(base) && isPlainObject(patch)) {
		const merged: Record<string, unknown> = { ...base };
		for (const key of Object.keys(patch)) {
			merged[key] = mergeValues(base[key], patch[key]);
		}
		return merged;
	}
	return undefined === patch ? base : patch;
}

export function diffValues(base: unknown, next: unknown): unknown {
	if (isPlainObject(base) && isPlainObject(next)) {
		const patch: Record<string, unknown> = {};
		for (const key of Object.keys(next)) {
			const change = diffValues(base[key], next[key]);
			if (undefined !== change) {
				patch[key] = change;
			}
		}
		return 0 < Object.keys(patch).length ? patch : undefined;
	}
	return JSON.stringify(base) === JSON.stringify(next) ? undefined : next;
}

export function preferencesPath(userId: string): string {
	return `/v1/files/${filePath(`/.callfs/ui/users/${userId}/preferences.json`)}`;
}

function readMirror(): unknown {
	if ('undefined' === typeof localStorage) {
		return undefined;
	}
	const text = localStorage.getItem(PREFERENCES_MIRROR_KEY);
	if (null === text) {
		return undefined;
	}
	try {
		return JSON.parse(text);
	} catch {
		return undefined;
	}
}

function writeMirror(prefs: Preferences): void {
	if ('undefined' === typeof localStorage) {
		return;
	}
	try {
		localStorage.setItem(PREFERENCES_MIRROR_KEY, JSON.stringify(prefs));
	} catch {
		return;
	}
}

interface RemoteState {
	preferences: Preferences;
	etag: string | null;
}

export class PreferencesStore {
	value = $state<Preferences>(migrate(readMirror()));
	status = $state<PreferencesStatus>('loading');
	error = $state<UiError | null>(null);
	#userId: string | null = null;
	#etag: string | null = null;
	#base: Preferences = migrate(undefined);
	#timer: ReturnType<typeof setTimeout> | null = null;

	async load(userId: string): Promise<Result<Preferences, UiError>> {
		this.#userId = userId;
		this.status = 'loading';
		const remote = await this.#fetchRemote(userId);
		if (false === remote.ok) {
			this.status = 'error';
			this.error = remote.error;
			return remote;
		}
		this.#etag = remote.value.etag;
		this.#base = remote.value.preferences;
		this.error = null;
		this.status = 'ready';
		this.#commit(remote.value.preferences, false);
		return { ok: true, value: this.value };
	}

	update(patch: PreferencesPatch): void {
		this.#commit(migrate(mergeValues(this.value, patch)), true);
	}

	reset(): void {
		this.#commit(migrate(undefined), true);
	}

	async save(): Promise<Result<void, UiError>> {
		this.#clearTimer();
		if (null === this.#userId) {
			return { ok: true, value: undefined };
		}
		return this.#write(this.#userId, false);
	}

	async #fetchRemote(userId: string): Promise<Result<RemoteState, UiError>> {
		const result = await request<unknown>(preferencesPath(userId), { node: null });
		if (false === result.ok) {
			if ('http' === result.error.kind && 404 === result.error.status) {
				return { ok: true, value: { preferences: migrate(undefined), etag: null } };
			}
			return { ok: false, error: toUiError(result.error, { action: 'preferences.load', read: true }) };
		}
		return { ok: true, value: { preferences: migrate(result.value.data), etag: result.value.etag } };
	}

	async #write(userId: string, retried: boolean): Promise<Result<void, UiError>> {
		const snapshot = this.value;
		const headers: Record<string, string> = null === this.#etag ? {} : { 'If-Match': this.#etag };
		const result = await request<unknown>(preferencesPath(userId), { method: 'PUT', body: snapshot, headers, node: null });
		if (true === result.ok) {
			this.#etag = result.value.etag;
			this.#base = snapshot;
			this.error = null;
			return { ok: true, value: undefined };
		}
		if ('http' === result.error.kind && 412 === result.error.status && false === retried) {
			return this.#mergeAfterConflict(userId);
		}
		const error = toUiError(result.error, { action: 'preferences.save' });
		this.error = error;
		return { ok: false, error };
	}

	async #mergeAfterConflict(userId: string): Promise<Result<void, UiError>> {
		const local = diffValues(this.#base, this.value);
		const remote = await this.#fetchRemote(userId);
		if (false === remote.ok) {
			this.error = remote.error;
			return { ok: false, error: remote.error };
		}
		this.#etag = remote.value.etag;
		this.#base = remote.value.preferences;
		this.#commit(migrate(mergeValues(remote.value.preferences, local)), true);
		return this.#write(userId, true);
	}

	#commit(next: Preferences, scheduleSave: boolean): void {
		this.value = next;
		writeMirror(next);
		if (scheduleSave && null !== this.#userId) {
			this.#scheduleSave();
		}
	}

	#scheduleSave(): void {
		this.#clearTimer();
		this.#timer = setTimeout(() => {
			this.#timer = null;
			void this.save();
		}, PREFERENCES_SAVE_DELAY_MS);
	}

	#clearTimer(): void {
		if (null === this.#timer) {
			return;
		}
		clearTimeout(this.#timer);
		this.#timer = null;
	}
}

export const preferences = new PreferencesStore();
