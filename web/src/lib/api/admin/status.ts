import { request } from '../http';
import { toUiError } from '../errors';
import type { AdminStatus, Result } from '../types';
import type { UiError } from '$lib/ui/types';

export interface RestartResult {
	restarting: boolean;
}

export async function status(): Promise<Result<AdminStatus, UiError>> {
	const response = await request<AdminStatus>('/v1/admin/status');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'status.load' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function restart(reason: string): Promise<Result<RestartResult, UiError>> {
	const response = await request<RestartResult>('/v1/admin/restart', { method: 'POST', body: { reason } });
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'status.restart' }) };
	}
	return { ok: true, value: response.value.data };
}
