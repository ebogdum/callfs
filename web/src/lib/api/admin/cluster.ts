import { request } from '../http';
import { toUiError } from '../errors';
import type { ClusterInfo, Preflight, Result, Snapshot, StateStats } from '../types';
import type { UiError } from '$lib/ui/types';

export interface NodeRequest {
	node_id: string;
	raft_addr: string;
	api_endpoint: string;
}

export interface JoinResult {
	status: 'joined' | 'already_member';
	leader_id: string | null;
	replaced: string[];
}

export interface RemoveResult {
	status: 'removed';
	leader_id: string | null;
	voters_after: number;
}

export interface TransferResult {
	status: 'transferred';
	leader_id: string | null;
}

export interface SnapshotResult {
	index: number;
	term: number;
	bytes: number;
	took_ms: number;
}

export interface SnapshotPage {
	retain: number;
	items: Snapshot[];
}

export async function cluster(): Promise<Result<ClusterInfo, UiError>> {
	const response = await request<ClusterInfo>('/v1/admin/cluster');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.load' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function preflight(req: NodeRequest): Promise<Result<Preflight, UiError>> {
	const response = await request<Preflight>('/v1/admin/cluster/preflight', { method: 'POST', body: req });
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.preflight' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function join(req: NodeRequest): Promise<Result<JoinResult, UiError>> {
	const response = await request<JoinResult>('/v1/admin/cluster/nodes', { method: 'POST', body: req });
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.join', object: req.node_id }) };
	}
	return { ok: true, value: response.value.data };
}

export async function removeNode(id: string, force = false): Promise<Result<RemoveResult, UiError>> {
	const response = await request<RemoveResult>(`/v1/admin/cluster/nodes/${encodeURIComponent(id)}`, {
		method: 'DELETE',
		query: { force: true === force ? true : undefined }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.remove', object: id }) };
	}
	return { ok: true, value: response.value.data };
}

export async function transferLeadership(nodeId: string | null): Promise<Result<TransferResult, UiError>> {
	const response = await request<TransferResult>('/v1/admin/cluster/leadership-transfer', {
		method: 'POST',
		body: { node_id: nodeId }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.transfer' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function snapshot(): Promise<Result<SnapshotResult, UiError>> {
	const response = await request<SnapshotResult>('/v1/admin/cluster/snapshot', { method: 'POST' });
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.snapshot' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function snapshots(): Promise<Result<SnapshotPage, UiError>> {
	const response = await request<SnapshotPage>('/v1/admin/cluster/snapshots');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.snapshots' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function stateStats(): Promise<Result<StateStats, UiError>> {
	const response = await request<StateStats>('/v1/admin/cluster/state-stats');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.stateStats' }) };
	}
	return { ok: true, value: response.value.data };
}
