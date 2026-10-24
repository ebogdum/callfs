import { request } from '../http';
import { toUiError } from '../errors';
import type { ConfigSchema, ConfigView, Result, ValidateResult } from '../types';
import type { UiError } from '$lib/ui/types';

export type ConfigChanges = Record<string, unknown>;

export type ExportFormat = 'yaml' | 'env' | 'json';

export interface SaveResult {
	written: boolean;
	backup_path: string;
	revision: string;
	restart_required: string[];
	applied_now: string[];
}

export interface PatchRuntimeResult {
	applied: string[];
	persisted: boolean;
	revision: string;
}

export async function getConfig(): Promise<Result<ConfigView, UiError>> {
	const response = await request<ConfigView>('/v1/admin/config');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.load' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function schema(): Promise<Result<ConfigSchema, UiError>> {
	const response = await request<ConfigSchema>('/v1/admin/config/schema');
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.schema' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function validate(changes: ConfigChanges): Promise<Result<ValidateResult, UiError>> {
	const response = await request<ValidateResult>('/v1/admin/config/validate', {
		method: 'POST',
		body: { changes }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.validate' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function save(changes: ConfigChanges, revision: string): Promise<Result<SaveResult, UiError>> {
	const response = await request<SaveResult>('/v1/admin/config', {
		method: 'PUT',
		body: { changes, expected_revision: revision }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.save' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function patchRuntime(changes: ConfigChanges): Promise<Result<PatchRuntimeResult, UiError>> {
	const response = await request<PatchRuntimeResult>('/v1/admin/config/runtime', {
		method: 'PATCH',
		body: { ...changes, persist: true }
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.runtime' }) };
	}
	return { ok: true, value: response.value.data };
}

export async function exportConfig(format: ExportFormat, effective = false): Promise<Result<string, UiError>> {
	const response = await request<string>('/v1/admin/config/export', {
		query: { format, effective },
		raw: true
	});
	if (!response.ok) {
		return { ok: false, error: toUiError(response.error, { action: 'config.export' }) };
	}
	return { ok: true, value: response.value.data };
}
