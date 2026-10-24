import { describe, expect, it, vi } from 'vitest';
import type { NodeInfo, NodeList } from '$lib/api/nodes';
import type { Capabilities, Result } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';
import { NODE_SCOPE_KEY, NodeScopeStore, readScope, writeScope, type ScopeStorage } from './node-scope.svelte';
import { NodeDirectory, type ScopeEffects } from './nodes.svelte';

vi.mock('$lib/live/hub.svelte', () => ({ liveHub: { rescope: vi.fn() } }));

class MemoryStorage implements ScopeStorage {
	readonly values = new Map<string, string>();

	getItem(key: string): string | null {
		return this.values.get(key) ?? null;
	}

	setItem(key: string, value: string): void {
		this.values.set(key, value);
	}

	removeItem(key: string): void {
		this.values.delete(key);
	}
}

const brokenStorage: ScopeStorage = {
	getItem: () => {
		throw new Error('blocked');
	},
	setItem: () => {
		throw new Error('blocked');
	},
	removeItem: () => {
		throw new Error('blocked');
	}
};

const LOAD_ERROR: UiError = { title: 'nodes.load', body: 'The request failed.', retryable: true };

function node(id: string, overrides: Partial<NodeInfo> = {}): NodeInfo {
	return { id, endpoint: `https://${id}.example.com`, self: false, status: 'up', latency_ms: 3, ...overrides };
}

function listOf(self: string, nodes: NodeInfo[]): Result<NodeList, UiError> {
	return { ok: true, value: { self, nodes } };
}

function effects(): ScopeEffects & { calls: string[] } {
	const calls: string[] = [];
	return {
		calls,
		clear: () => {
			calls.push('clear');
		},
		rescope: () => {
			calls.push('rescope');
		},
		reload: () => {
			calls.push('reload');
			return Promise.resolve({ ok: true, value: {} as Capabilities });
		}
	};
}

function directoryWith(storage: MemoryStorage, result: Result<NodeList, UiError>) {
	const scope = new NodeScopeStore(() => storage);
	const spy = effects();
	const fetch = vi.fn(() => Promise.resolve(result));
	return { scope, spy, fetch, directory: new NodeDirectory(scope, fetch, spy) };
}

describe('scope storage helpers', () => {
	it('reads nothing from missing, empty or blocked storage', () => {
		const storage = new MemoryStorage();
		expect(readScope(null)).toBeNull();
		expect(readScope(storage)).toBeNull();
		storage.setItem(NODE_SCOPE_KEY, '  ');
		expect(readScope(storage)).toBeNull();
		expect(readScope(brokenStorage)).toBeNull();
	});

	it('reports a failed write instead of throwing', () => {
		expect(writeScope(brokenStorage, 'node-2')).toBe(false);
		expect(writeScope(null, 'node-2')).toBe(false);
		const storage = new MemoryStorage();
		expect(writeScope(storage, 'node-2')).toBe(true);
		expect(storage.getItem(NODE_SCOPE_KEY)).toBe('node-2');
		expect(writeScope(storage, null)).toBe(true);
		expect(storage.getItem(NODE_SCOPE_KEY)).toBeNull();
	});
});

describe('NodeScopeStore', () => {
	it('starts on this server and restores a saved node', () => {
		const storage = new MemoryStorage();
		expect(new NodeScopeStore(() => storage).selected).toBeNull();
		storage.setItem(NODE_SCOPE_KEY, 'node-2');
		const restored = new NodeScopeStore(() => storage);
		expect(restored.selected).toBe('node-2');
		expect(restored.remote).toBe(true);
	});

	it('saves the selection and clears it when going back', () => {
		const storage = new MemoryStorage();
		const scope = new NodeScopeStore(() => storage);
		expect(scope.select('node-2')).toBe(true);
		expect(storage.getItem(NODE_SCOPE_KEY)).toBe('node-2');
		expect(scope.select('node-2')).toBe(false);
		expect(scope.select(null)).toBe(true);
		expect(scope.selected).toBeNull();
		expect(storage.getItem(NODE_SCOPE_KEY)).toBeNull();
	});

	it('keeps working in memory when storage is blocked', () => {
		const scope = new NodeScopeStore(() => brokenStorage);
		expect(scope.selected).toBeNull();
		expect(scope.select('node-2')).toBe(true);
		expect(scope.selected).toBe('node-2');
	});

	it('drops a selection that is gone or that names this server', () => {
		const storage = new MemoryStorage();
		const scope = new NodeScopeStore(() => storage);
		scope.select('node-2');
		expect(scope.retain(['node-0', 'node-2'], 'node-0')).toBe(false);
		expect(scope.selected).toBe('node-2');
		expect(scope.retain(['node-0', 'node-1'], 'node-0')).toBe(true);
		expect(scope.selected).toBeNull();
		scope.select('node-0');
		expect(scope.retain(['node-0', 'node-1'], 'node-0')).toBe(true);
		expect(scope.selected).toBeNull();
	});
});

describe('NodeDirectory', () => {
	it('loads the nodes and keeps a selection that still exists', async () => {
		const storage = new MemoryStorage();
		storage.setItem(NODE_SCOPE_KEY, 'node-2');
		const { directory, spy } = directoryWith(
			storage,
			listOf('node-0', [node('node-0', { self: true }), node('node-2', { status: 'down' })])
		);
		await directory.load();
		expect(directory.items.map((entry) => entry.id)).toEqual(['node-0', 'node-2']);
		expect(directory.selfId).toBe('node-0');
		expect(directory.multi).toBe(true);
		expect(directory.upCount).toBe(1);
		expect(directory.selected).toBe('node-2');
		expect(directory.active?.id).toBe('node-2');
		expect(spy.calls).toEqual([]);
	});

	it('clears a selection that the server no longer lists and resets node data', async () => {
		const storage = new MemoryStorage();
		storage.setItem(NODE_SCOPE_KEY, 'node-9');
		const { directory, spy } = directoryWith(storage, listOf('node-0', [node('node-0', { self: true }), node('node-1')]));
		await directory.load();
		expect(directory.selected).toBeNull();
		expect(storage.getItem(NODE_SCOPE_KEY)).toBeNull();
		expect(spy.calls).toEqual(['clear', 'rescope', 'reload']);
	});

	it('treats a server without the nodes list as a single server', async () => {
		const storage = new MemoryStorage();
		storage.setItem(NODE_SCOPE_KEY, 'node-2');
		const { directory } = directoryWith(storage, listOf('', []));
		await directory.load();
		expect(directory.multi).toBe(false);
		expect(directory.selfId).toBeNull();
		expect(directory.selected).toBeNull();
	});

	it('keeps the selection and the last list when loading fails', async () => {
		const storage = new MemoryStorage();
		storage.setItem(NODE_SCOPE_KEY, 'node-2');
		const { directory, fetch, spy } = directoryWith(storage, { ok: false, error: LOAD_ERROR });
		const result = await directory.load();
		expect(result.ok).toBe(false);
		expect(directory.error).toEqual(LOAD_ERROR);
		expect(directory.selected).toBe('node-2');
		expect(fetch).toHaveBeenCalledTimes(1);
		expect(spy.calls).toEqual([]);
	});

	it('switches nodes, resets node data and maps this server to no selection', async () => {
		const storage = new MemoryStorage();
		const { directory, spy } = directoryWith(storage, listOf('node-0', [node('node-0', { self: true }), node('node-2')]));
		await directory.load();
		expect(directory.isActive(directory.items[0] as NodeInfo)).toBe(true);
		const pending = directory.switchTo('node-2');
		expect(pending).not.toBeNull();
		expect(directory.selected).toBe('node-2');
		expect(storage.getItem(NODE_SCOPE_KEY)).toBe('node-2');
		expect(directory.isActive(directory.items[1] as NodeInfo)).toBe(true);
		expect(spy.calls).toEqual(['clear', 'rescope', 'reload']);
		expect(directory.switchTo('node-2')).toBeNull();
		expect(directory.switchTo('node-0')).not.toBeNull();
		expect(directory.selected).toBeNull();
		expect(spy.calls).toHaveLength(6);
	});

	it('shares one request between overlapping loads', async () => {
		const { directory, fetch } = directoryWith(new MemoryStorage(), listOf('node-0', [node('node-0', { self: true })]));
		await Promise.all([directory.load(), directory.load()]);
		expect(fetch).toHaveBeenCalledTimes(1);
	});
});

describe('roleOf', () => {
	it('prefers the reported role in any case', async () => {
		const { roleOf } = await vi.importActual<typeof import('$lib/api/nodes')>('$lib/api/nodes');
		expect(roleOf(node('node-1', { role: 'Leader' }))).toBe('leader');
		expect(roleOf(node('node-1', { role: 'follower' }))).toBe('follower');
		expect(roleOf(node('node-1'))).toBe('none');
	});

	it('infers the role from the leader other servers report', async () => {
		const { roleOf } = await vi.importActual<typeof import('$lib/api/nodes')>('$lib/api/nodes');
		const all = [node('node-0', { self: true }), node('node-1', { leader_id: 'node-0' }), node('node-2', { leader_id: 'node-0' })];
		expect(roleOf(all[0] as NodeInfo, all)).toBe('leader');
		const other = [node('node-0', { self: true }), node('node-1', { leader_id: 'node-2' })];
		expect(roleOf(other[0] as NodeInfo, other)).toBe('follower');
	});
});
