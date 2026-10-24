import { request } from '../http';
import { toUiError } from '../errors';
import type { ClusterInfo, Result, Snapshot, StateStats } from '../types';
import type { UiError } from '$lib/ui/types';

export interface JoinConfigRequest {
	node_id: string;
	api_endpoint: string;
	cluster_addr: string;
}

export interface JoinConfig {
	yaml: string;
	steps: string[];
}

export interface RemoveResult {
	status: 'removed';
	remaining: number;
}

export interface TransferResult {
	status: 'transferred';
	leader_id: string | null;
}

export interface SnapshotResult {
	id: string;
	seq: number;
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

export async function joinConfig(req: JoinConfigRequest): Promise<Result<JoinConfig, UiError>> {
	const response = await request<JoinConfig>('/v1/admin/cluster/join-config', {
		query: { node_id: req.node_id, api_endpoint: req.api_endpoint, cluster_addr: req.cluster_addr }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'cluster.joinConfig', object: req.node_id }) };
	}
	return { ok: true, value: response.value.data };
}

export async function removeNode(id: string): Promise<Result<RemoveResult, UiError>> {
	const response = await request<RemoveResult>(`/v1/admin/cluster/nodes/${encodeURIComponent(id)}`, {
		method: 'DELETE'
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
