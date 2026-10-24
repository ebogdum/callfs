import { request } from '../http';
import { toUiError } from '../errors';
import type { Health, Result } from '../types';
import type { UiError } from '$lib/ui/types';

export async function health(fresh = false): Promise<Result<Health, UiError>> {
	const response = await request<Health>('/v1/admin/health', {
		query: { fresh: true === fresh ? true : undefined }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'health.load' }) };
	}
	return { ok: true, value: response.value.data };
}
