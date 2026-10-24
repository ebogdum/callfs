import { preferences, type FileView, type PreferencesStore } from '$lib/stores/preferences.svelte';

export const RECENT_PATH_LIMIT = 10;

type PreferencesSink = Pick<PreferencesStore, 'value' | 'update'>;

export class FilesStore {
	readonly #prefs: PreferencesSink;

	constructor(prefs: PreferencesSink) {
		this.#prefs = prefs;
	}

	get view(): FileView {
		return this.#prefs.value.files.view;
	}

	get hidden(): boolean {
		return this.#prefs.value.files.hidden;
	}

	get pinned(): readonly string[] {
		return this.#prefs.value.files.pinned;
	}

	get recent(): readonly string[] {
		return this.#prefs.value.files.recent;
	}

	setView(view: FileView): void {
		this.#prefs.update({ files: { view } });
	}

	toggleHidden(): void {
		this.#prefs.update({ files: { hidden: !this.hidden } });
	}

	pin(path: string): void {
		if (this.pinned.includes(path)) {
			return;
		}
		this.#prefs.update({ files: { pinned: [...this.pinned, path] } });
	}

	unpin(path: string): void {
		this.#prefs.update({ files: { pinned: this.pinned.filter((item) => path !== item) } });
	}

	remember(path: string): void {
		if (path === this.recent[0]) {
			return;
		}
		const recent = [path, ...this.recent.filter((item) => path !== item)].slice(0, RECENT_PATH_LIMIT);
		this.#prefs.update({ files: { recent } });
	}

}

export const filesStore = new FilesStore(preferences);
