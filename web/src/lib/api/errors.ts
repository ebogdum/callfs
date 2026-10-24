import { formatBytes } from '$lib/format/bytes';
import { t } from '$lib/copy/t';
import type { UiError } from '$lib/ui/types';

export interface ErrorContext {
	action: string;
	object?: string;
	read?: boolean;
}

export interface HttpFailure {
	kind: 'http';
	status: number;
	code: string | null;
	message: string | null;
	details: Record<string, unknown> | null;
	requestId: string | null;
	retryAfterS: number | null;
}

export type ApiFailure = { kind: 'network' | 'aborted' | 'timeout' | 'malformed' } | HttpFailure;

type Described = Omit<UiError, 'status' | 'code' | 'requestId'>;

function asRecord(value: unknown): Record<string, unknown> | null {
	if ('object' !== typeof value || null === value || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
}

function parseRetryAfter(value: string | null): number | null {
	if (null === value) {
		return null;
	}
	const seconds = Number(value);
	if (Number.isFinite(seconds) && 0 <= seconds) {
		return seconds;
	}
	const at = Date.parse(value);
	return Number.isFinite(at) ? Math.max(0, (at - Date.now()) / 1000) : null;
}

function parseEnvelope(body: string): Record<string, unknown> | null {
	try {
		return asRecord(JSON.parse(body));
	} catch {
		return null;
	}
}

function stringField(record: Record<string, unknown> | null, key: string): string | null {
	const value = null === record ? undefined : record[key];
	return 'string' === typeof value ? value : null;
}

export function httpFailure(status: number, body: string, retryAfter: string | null, requestId: string | null): HttpFailure {
	const envelope = parseEnvelope(body);
	const details = null === envelope ? null : asRecord(envelope.details);
	return {
		kind: 'http',
		status,
		code: stringField(envelope, 'code'),
		message: stringField(envelope, 'message'),
		details,
		requestId,
		retryAfterS: parseRetryAfter(retryAfter)
	};
}

function detailString(details: Record<string, unknown> | null, key: string): string | null {
	const value = null === details ? undefined : details[key];
	if ('string' === typeof value) {
		return value;
	}
	if ('number' === typeof value) {
		return String(value);
	}
	return null;
}

function detailNumber(details: Record<string, unknown> | null, key: string): number | null {
	const value = null === details ? undefined : details[key];
	return 'number' === typeof value ? value : null;
}

function bytesText(value: number | null): string {
	return null === value ? t('common.unknown') : formatBytes(value).text;
}

function validationCount(details: Record<string, unknown> | null): number {
	const errors = null === details ? undefined : details.errors;
	return Array.isArray(errors) && 0 < errors.length ? errors.length : 1;
}

function fix(key: string, href?: string): { label: string; href?: string } {
	return { label: t(key), href };
}

function serverDescription(action: string, retryable: boolean): Described {
	return {
		title: t('errors.server.title', { action }),
		body: t('errors.server.body'),
		retryable,
		layer: 'server',
		fixAction: fix('errors.server.fix')
	};
}

function conflictDescription(failure: HttpFailure, context: ErrorContext): Described {
	const object = context.object ?? t('common.unknown');
	switch (failure.code) {
		case 'RESOURCE_LOCKED':
			return {
				title: t('errors.locked.title'),
				body: t('errors.locked.body'),
				retryable: true,
				layer: 'locked',
				fixAction: fix('errors.locked.fix')
			};
		default:
			return {
				title: t('errors.conflict.title', { object }),
				body: t('errors.conflict.body'),
				retryable: false,
				layer: 'conflict',
				fixAction: fix('errors.conflict.replace')
			};
	}
}

function notFoundDescription(failure: HttpFailure, context: ErrorContext): Described {
	const object = context.object ?? t('common.unknown');
	if ('NODE_NOT_FOUND' === failure.code) {
		return {
			title: t('errors.node_not_found.title'),
			body: t('errors.node_not_found.body'),
			retryable: false,
			layer: 'peer',
			fixAction: fix('errors.node_not_found.fix', '/cluster/servers')
		};
	}
	if ('FEATURE_DISABLED' === failure.code) {
		return {
			title: t('errors.feature_disabled.title', { feature: object }),
			body: t('errors.feature_disabled.body'),
			retryable: false,
			layer: 'not_found',
			fixAction: fix('errors.feature_disabled.fix', '/settings')
		};
	}
	return {
		title: t('errors.not_found.title', { object }),
		body: t('errors.not_found.body'),
		retryable: false,
		layer: 'not_found',
		fixAction: fix('errors.not_found.fix')
	};
}

function describeHttp(failure: HttpFailure, context: ErrorContext): Described {
	const action = t(context.action);
	const { details, retryAfterS } = failure;
	switch (failure.status) {
		case 400:
			return {
				title: t('errors.bad_request.title', { action }),
				body: null === failure.code || null === failure.message ? t('errors.bad_request.body') : failure.message,
				retryable: false,
				layer: 'server'
			};
		case 401:
			return {
				title: t('errors.unauthenticated.title'),
				body: t('errors.unauthenticated.body'),
				retryable: true,
				layer: 'auth',
				fixAction: fix('errors.unauthenticated.fix')
			};
		case 403:
			if ('ADMIN_REQUIRED' === failure.code) {
				return {
					title: t('errors.admin_required.title'),
					body: t('errors.admin_required.body', { action }),
					retryable: false,
					layer: 'permission'
				};
			}
			return {
				title: t('errors.permission.title', { action }),
				body: t('errors.permission.body', {
					owner: detailString(details, 'owner') ?? t('common.unknown')
				}),
				retryable: false,
				layer: 'permission',
				fixAction: fix('errors.permission.fix')
			};
		case 404:
			return notFoundDescription(failure, context);
		case 409:
			return conflictDescription(failure, context);
		case 410:
			return {
				title: t('errors.gone.title'),
				body: t('errors.gone.body'),
				retryable: false,
				layer: 'not_found'
			};
		case 412:
			return {
				title: t('errors.precondition.title'),
				body: t('errors.precondition.body'),
				retryable: false,
				layer: 'precondition',
				fixAction: fix('errors.precondition.keep_theirs')
			};
		case 413:
			return {
				title: t('errors.too_large.title'),
				body: t('errors.too_large.body', { max: bytesText(detailNumber(details, 'max_bytes')) }),
				retryable: false,
				layer: 'limit'
			};
		case 416:
			return {
				title: t('errors.range.title'),
				body: t('errors.range.body', { size: bytesText(detailNumber(details, 'size')) }),
				retryable: false,
				layer: 'limit'
			};
		case 422:
			return {
				title: t('errors.validation.title'),
				body: t('errors.validation.body', {
					first: failure.message ?? t('common.unknown'),
					count: validationCount(details)
				}),
				retryable: false,
				layer: 'server',
				fixAction: fix('errors.validation.fix')
			};
		case 429:
			return {
				title: t('errors.rate_limited.title'),
				body: t('errors.rate_limited.body', { n: retryAfterS ?? 1 }),
				retryable: true,
				layer: 'limit'
			};
		case 500:
			return serverDescription(action, true === context.read);
		case 502:
			if ('NODE_UNREACHABLE' === failure.code) {
				return {
					title: t('errors.node_unreachable.title'),
					body: t('errors.node_unreachable.body'),
					retryable: true,
					layer: 'peer',
					fixAction: fix('errors.node_unreachable.fix', '/cluster/servers')
				};
			}
			return {
				title: t('errors.peer.title'),
				body: t('errors.peer.body', { instance: detailString(details, 'instance') ?? t('common.unknown') }),
				retryable: true,
				layer: 'peer',
				fixAction: fix('errors.peer.fix', '/cluster/servers')
			};
		case 503:
			return serverDescription(action, true);
		case 508:
			return {
				title: t('errors.forward_loop.title'),
				body: t('errors.forward_loop.body'),
				retryable: false,
				layer: 'peer',
				fixAction: fix('errors.forward_loop.fix', '/cluster/servers')
			};
		default:
			if (500 <= failure.status) {
				return serverDescription(action, true === context.read);
			}
			return {
				title: t('errors.bad_request.title', { action }),
				body: t('errors.bad_request.body'),
				retryable: false,
				layer: 'server'
			};
	}
}

export function toUiError(failure: ApiFailure, context: ErrorContext): UiError {
	if ('http' === failure.kind) {
		const { status, code, requestId } = failure;
		return { ...describeHttp(failure, context), status, code: code ?? undefined, requestId: requestId ?? undefined };
	}
	if ('malformed' === failure.kind) {
		return serverDescription(t(context.action), true === context.read);
	}
	return {
		title: t('errors.network.title'),
		body: t('errors.network.body'),
		retryable: true,
		layer: 'network',
		fixAction: fix('errors.network.fix')
	};
}
