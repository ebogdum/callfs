import type { UiError } from '$lib/ui/types';
import { toUiError } from './errors';
import { request } from './http';
import type { Result } from './types';

export type NodeStatus = 'up' | 'down';

export type NodeRole = 'leader' | 'follower' | 'candidate' | 'shutdown' | 'none';

export interface NodeInfo {
	id: string;
	endpoint: string;
	self: boolean;
	status: NodeStatus;
	latency_ms: number;
	detail?: string;
	raft_state?: string;
	leader_id?: string;
	version?: string;
	uptime_s?: number;
	started_at?: string;
}

export interface NodeList {
	self: string;
	nodes: NodeInfo[];
}

const NODES_PATH = '/v1/nodes';
const SINGLE_SERVER: NodeList = { self: '', nodes: [] };
const ROLES: readonly NodeRole[] = ['leader', 'follower', 'candidate', 'shutdown'];

export function roleOf(node: NodeInfo, all: readonly NodeInfo[] = []): NodeRole {
	const state = (node.raft_state ?? '').toLowerCase();
	const reported = ROLES.find((role) => role === state);
	if (undefined !== reported) {
		return reported;
	}
	const leaders = all.map((peer) => peer.leader_id ?? '').filter((id) => '' !== id);
	if (leaders.includes(node.id)) {
		return 'leader';
	}
	return 0 < leaders.length ? 'follower' : 'none';
}

export async function listNodes(): Promise<Result<NodeList, UiError>> {
	const result = await request<NodeList>(NODES_PATH, { node: null });
	if (false === result.ok) {
		if ('http' === result.error.kind && 404 === result.error.status) {
			return { ok: true, value: SINGLE_SERVER };
		}
		return { ok: false, error: toUiError(result.error, { action: 'nodes.load', read: true }) };
	}
	const data = result.value.data;
	if (undefined === data || null === data || false === Array.isArray(data.nodes)) {
		return { ok: true, value: SINGLE_SERVER };
	}
	return { ok: true, value: { self: data.self ?? '', nodes: data.nodes } };
}
