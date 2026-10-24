import type { ApiFailure } from '$lib/api/errors';
import { request, type ApiResponse } from '$lib/api/http';
import type {
	Backend,
	ErasureSummary,
	Hygiene,
	Migrations,
	PlacementPlan,
	Replication,
	Result,
	StagingItem
} from '$lib/api/types';

export interface InstanceItem {
	instance_id: string;
	endpoint: string;
	source: 'config' | 'joined' | 'self';
	is_self: boolean;
	scheme: 'https' | 'http';
	reachable: boolean | null;
	rtt_ms: number | null;
	checked_at: string | null;
	files: number;
	bytes: number;
	pieces: number;
	disk_free_bytes: number | null;
	disk_total_bytes: number | null;
}

export interface InstanceList {
	self: { instance_id: string; external_url: string | null; started_at: string };
	items: InstanceItem[];
}

export interface InstanceProbe {
	reachable: boolean;
	rtt_ms: number | null;
	status_code: number | null;
	error: string | null;
}

export interface BackendList {
	default: string;
	items: Backend[];
}

export interface BackendProbe {
	ok: boolean;
	write_ms: number | null;
	read_ms: number | null;
	delete_ms: number | null;
	error: string | null;
}

export interface StagingList {
	items: StagingItem[];
	total_bytes: number;
}

export interface ErasurePlanRequest {
	data_shards: number;
	parity_shards: number;
	instances: string[];
}

export interface JobAccepted {
	job_id: string;
}

export function dataOf<T>(response: Result<ApiResponse<T>, ApiFailure>): Result<T, ApiFailure> {
	if (!response.ok) {
		return response;
	}
	return { ok: true, value: response.value.data };
}

const LONG_TIMEOUT_MS = 120_000;

export async function instances(): Promise<Result<InstanceList, ApiFailure>> {
	return dataOf(await request<InstanceList>('/v1/admin/instances'));
}

export async function setEndpoint(id: string, url: string, persist: boolean): Promise<Result<InstanceItem, ApiFailure>> {
	return dataOf(
		await request<InstanceItem>(`/v1/admin/instances/${encodeURIComponent(id)}/endpoint`, {
			method: 'PUT',
			body: { endpoint: url, persist }
		})
	);
}

export async function removeEndpoint(id: string): Promise<Result<InstanceItem, ApiFailure>> {
	return dataOf(
		await request<InstanceItem>(`/v1/admin/instances/${encodeURIComponent(id)}/endpoint`, { method: 'DELETE' })
	);
}

export async function probeInstance(id: string): Promise<Result<InstanceProbe, ApiFailure>> {
	return dataOf(
		await request<InstanceProbe>(`/v1/admin/instances/${encodeURIComponent(id)}/probe`, {
			method: 'POST',
			timeoutMs: LONG_TIMEOUT_MS
		})
	);
}

export async function backends(): Promise<Result<BackendList, ApiFailure>> {
	return dataOf(await request<BackendList>('/v1/admin/backends'));
}

export async function probeBackend(name: string): Promise<Result<BackendProbe, ApiFailure>> {
	return dataOf(
		await request<BackendProbe>(`/v1/admin/backends/${encodeURIComponent(name)}/probe`, {
			method: 'POST',
			timeoutMs: LONG_TIMEOUT_MS
		})
	);
}

export async function erasureSummary(): Promise<Result<ErasureSummary, ApiFailure>> {
	return dataOf(await request<ErasureSummary>('/v1/admin/erasure'));
}

export async function erasurePlan(req: ErasurePlanRequest): Promise<Result<PlacementPlan, ApiFailure>> {
	return dataOf(await request<PlacementPlan>('/v1/admin/erasure/plan', { method: 'POST', body: req }));
}

export async function verifyAll(prefix: string): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(await request<JobAccepted>('/v1/admin/erasure/verify-all', { method: 'POST', body: { prefix } }));
}

export async function replication(): Promise<Result<Replication, ApiFailure>> {
	return dataOf(await request<Replication>('/v1/admin/replication'));
}

export async function retryReplication(paths: string[] | null): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(await request<JobAccepted>('/v1/admin/replication/retry', { method: 'POST', body: { paths } }));
}

export async function staging(): Promise<Result<StagingList, ApiFailure>> {
	return dataOf(await request<StagingList>('/v1/admin/staging'));
}

export async function cleanStaging(olderThanS: number): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(
		await request<JobAccepted>('/v1/admin/staging', { method: 'DELETE', query: { older_than_s: olderThanS } })
	);
}

export async function hygiene(): Promise<Result<Hygiene, ApiFailure>> {
	return dataOf(await request<Hygiene>('/v1/admin/hygiene'));
}

export async function migrations(): Promise<Result<Migrations, ApiFailure>> {
	return dataOf(await request<Migrations>('/v1/admin/migrations'));
}

export async function retag(prefix: string): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(await request<JobAccepted>('/v1/admin/retag', { method: 'POST', body: { prefix } }));
}
