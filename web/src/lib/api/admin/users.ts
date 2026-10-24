import type { ApiFailure } from '$lib/api/errors';
import { request } from '$lib/api/http';
import type { AuthFailure, OwnerStat, Result, TlsInfo, UserInfo } from '$lib/api/types';
import { dataOf, type JobAccepted } from './topology';

export interface UserList {
	items: UserInfo[];
	positional_count: number;
}

export interface CreatedUser {
	user_id: string;
	api_key: string;
}

export interface UserPatch {
	is_admin: boolean;
}

export interface PositionalMapping {
	index: number;
	user_id: string;
}

export interface OwnerPage {
	items: OwnerStat[];
	next_cursor: string | null;
}

export interface AuthFailureList {
	total_24h: number;
	items: AuthFailure[];
}

export async function users(): Promise<Result<UserList, ApiFailure>> {
	return dataOf(await request<UserList>('/v1/admin/users'));
}

export async function createUser(id: string, isAdmin: boolean): Promise<Result<CreatedUser, ApiFailure>> {
	return dataOf(
		await request<CreatedUser>('/v1/admin/users', { method: 'POST', body: { user_id: id, is_admin: isAdmin } })
	);
}

export async function rotateKey(id: string): Promise<Result<CreatedUser, ApiFailure>> {
	return dataOf(await request<CreatedUser>(`/v1/admin/users/${encodeURIComponent(id)}/rotate`, { method: 'POST' }));
}

export async function updateUser(id: string, patch: UserPatch): Promise<Result<UserInfo, ApiFailure>> {
	return dataOf(
		await request<UserInfo>(`/v1/admin/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: patch })
	);
}

export async function deleteUser(id: string): Promise<Result<void, ApiFailure>> {
	return dataOf(await request<void>(`/v1/admin/users/${encodeURIComponent(id)}`, { method: 'DELETE' }));
}

export async function migratePositional(mapping: PositionalMapping[]): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(
		await request<JobAccepted>('/v1/admin/users/migrate-positional', {
			method: 'POST',
			body: { mapping, reassign_owner: true }
		})
	);
}

export async function owners(cursor: string | null): Promise<Result<OwnerPage, ApiFailure>> {
	return dataOf(await request<OwnerPage>('/v1/admin/owners', { query: { cursor } }));
}

export async function reassignOwner(from: string, to: string, prefix: string): Promise<Result<JobAccepted, ApiFailure>> {
	return dataOf(
		await request<JobAccepted>('/v1/admin/owners/reassign', { method: 'POST', body: { from, to, prefix } })
	);
}

export async function authFailures(since: string | null): Promise<Result<AuthFailureList, ApiFailure>> {
	return dataOf(await request<AuthFailureList>('/v1/admin/auth/failures', { query: { since } }));
}

export async function tls(): Promise<Result<TlsInfo, ApiFailure>> {
	return dataOf(await request<TlsInfo>('/v1/admin/tls'));
}
