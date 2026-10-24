import { request } from './http';
import type { BatchOp, BatchResult } from './types';

export type BatchMode = 'sync' | 'job';

export function batch(ops: BatchOp[], mode: BatchMode) {
	return request<BatchResult>('/v1/batch', {
		method: 'POST',
		body: { ops, mode, stop_on_error: false }
	});
}
