import { request } from '../http';
import type { ApiFailure } from '../errors';
import type { Job, Result, Worker } from '../types';
import { dataOf, joinList } from './telemetry';

export type JobState = Job['state'];
export type JobKind = Job['kind'];
export type WorkerName = Worker['name'];

export type JobQuery = { state?: readonly JobState[]; kind?: readonly JobKind[]; createdBy?: string };
export type JobList = { items: Job[] };
export type WorkerList = { items: Worker[] };
export type JobStarted = { job_id: string };

export async function jobs(query: JobQuery = {}): Promise<Result<JobList, ApiFailure>> {
	return dataOf(await request<JobList>('/v1/admin/jobs', { query: { state: joinList(query.state), kind: joinList(query.kind), created_by: query.createdBy } }));
}

export async function job(id: string): Promise<Result<Job, ApiFailure>> {
	return dataOf(await request<Job>(`/v1/admin/jobs/${encodeURIComponent(id)}`));
}

export async function cancel(id: string): Promise<Result<Job, ApiFailure>> {
	return dataOf(await request<Job>(`/v1/admin/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }));
}

export async function retryFailed(id: string): Promise<Result<Job, ApiFailure>> {
	return dataOf(await request<Job>(`/v1/admin/jobs/${encodeURIComponent(id)}/retry-failed`, { method: 'POST' }));
}

export async function workers(): Promise<Result<WorkerList, ApiFailure>> {
	return dataOf(await request<WorkerList>('/v1/admin/workers'));
}

export async function runWorker(name: WorkerName): Promise<Result<JobStarted, ApiFailure>> {
	return dataOf(await request<JobStarted>(`/v1/admin/workers/${name}/run`, { method: 'POST' }));
}
