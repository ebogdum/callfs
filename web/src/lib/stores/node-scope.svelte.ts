export const NODE_SCOPE_KEY = 'callfs.node-scope';

export interface ScopeStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
	removeItem(key: string): void;
}

function browserStorage(): ScopeStorage | null {
	try {
		return globalThis.localStorage ?? null;
	} catch {
		return null;
	}
}

function normalized(id: string | null | undefined): string | null {
	return undefined === id || null === id || '' === id.trim() ? null : id;
}

export function readScope(storage: ScopeStorage | null): string | null {
	if (null === storage) {
		return null;
	}
	try {
		return normalized(storage.getItem(NODE_SCOPE_KEY));
	} catch {
		return null;
	}
}

export function writeScope(storage: ScopeStorage | null, id: string | null): boolean {
	if (null === storage) {
		return false;
	}
	try {
		if (null === id) {
			storage.removeItem(NODE_SCOPE_KEY);
		} else {
			storage.setItem(NODE_SCOPE_KEY, id);
		}
		return true;
	} catch {
		return false;
	}
}

export class NodeScopeStore {
	selected = $state<string | null>(null);
	readonly #storage: () => ScopeStorage | null;

	constructor(storage: () => ScopeStorage | null = browserStorage) {
		this.#storage = storage;
		this.selected = readScope(storage());
	}

	get remote(): boolean {
		return null !== this.selected;
	}

	select(id: string | null): boolean {
		const next = normalized(id);
		if (next === this.selected) {
			return false;
		}
		this.selected = next;
		writeScope(this.#storage(), next);
		return true;
	}

	retain(known: readonly string[], selfId: string | null): boolean {
		const current = this.selected;
		if (null === current) {
			return false;
		}
		if (current === selfId || false === known.includes(current)) {
			return this.select(null);
		}
		return false;
	}
}

export const nodeScope = new NodeScopeStore();
