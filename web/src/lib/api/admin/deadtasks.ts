import { request } from '../http';
import type { ApiFailure } from '../errors';
import type { DeadTask, Result } from '../types';
import { dataOf } from './telemetry';

export type DeadTaskList = { items: DeadTask[]; next_before: number | null };

export async function deadTasks(before?: number): Promise<Result<DeadTaskList, ApiFailure>> {
	return dataOf(await request<DeadTaskList>('/v1/admin/tasks/dead', { query: { before } }));
}

export async function retryDeadTask(id: number): Promise<Result<void, ApiFailure>> {
	return dataOf(await request<void>(`/v1/admin/tasks/dead/${id}/retry`, { method: 'POST' }));
}

export async function dropDeadTask(id: number): Promise<Result<void, ApiFailure>> {
	return dataOf(await request<void>(`/v1/admin/tasks/dead/${id}`, { method: 'DELETE' }));
}
