import { describe, expect, it } from 'vitest';
import { STATUS, type StatusKey } from '$lib/status/catalog';
import copy from '$lib/copy/en/status';
import { t } from '$lib/copy/t';

const STATUS_TONES: readonly string[] = ['neutral', 'accent', 'ok', 'warn', 'danger', 'info', 'progress', 'muted'];

const EXPECTED_KEYS: StatusKey[] = [
	'health.ok', 'health.warn', 'health.down', 'health.disabled', 'health.unknown', 'health.checking',
	'member.leader', 'member.follower', 'member.shutdown', 'member.unreachable', 'member.lagging',
	'server.up', 'server.down', 'server.self', 'server.unknown',
	'quorum.ok', 'quorum.at_risk', 'quorum.lost',
	'link.active', 'link.used', 'link.expired', 'link.revoked',
	'job.queued', 'job.running', 'job.succeeded', 'job.partial', 'job.failed', 'job.cancelled', 'job.cancelling',
	'transfer.queued', 'transfer.uploading', 'transfer.downloading', 'transfer.paused', 'transfer.stalled', 'transfer.verifying', 'transfer.done', 'transfer.failed', 'transfer.cancelled', 'transfer.conflict',
	'piece.ok', 'piece.missing', 'piece.corrupt', 'piece.unreachable', 'piece.repaired',
	'durability.full', 'durability.reduced', 'durability.critical', 'durability.unreadable',
	'file.locked', 'file.split', 'file.remote', 'file.backup_ok', 'file.backup_failed', 'file.no_owner', 'file.no_tag', 'file.hidden',
	'source.default', 'source.file', 'source.env', 'source.runtime',
	'setting.saved', 'setting.pending_restart', 'setting.applies_now', 'setting.invalid', 'setting.locked_env',
	'conn.live', 'conn.stale', 'conn.reconnecting', 'conn.offline', 'conn.paused',
	'finding.critical', 'finding.warning', 'finding.info', 'finding.muted', 'finding.ignored',
	'backup.on_strict', 'backup.on_best_effort', 'backup.off',
	'tls.on', 'tls.off', 'tls.fallback', 'tls.expiring', 'tls.expired',
	'session.active', 'session.expiring', 'session.expired',
	'storage.on', 'storage.off', 'storage.error',
	'metric.reported', 'metric.not_reported',
	'lock.held', 'lock.expiring', 'lock.lost',
];

describe('status catalog', () => {
	it('has exactly the keys listed in the status catalog spec', () => {
		expect(Object.keys(STATUS).sort()).toEqual([...EXPECTED_KEYS].sort());
	});

	it.each(EXPECTED_KEYS)('%s has a known tone and an existing copy key', (key) => {
		const entry = STATUS[key];
		expect(STATUS_TONES).toContain(entry.tone);
		expect(Object.hasOwn(copy, entry.word)).toBe(true);
	});

	it('gives every copy word visible text', () => {
		for (const key of EXPECTED_KEYS) {
			expect(t(STATUS[key].word)).toMatch(/\S/);
		}
	});
});
