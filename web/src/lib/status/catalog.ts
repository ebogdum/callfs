import type { CopyKey } from '$lib/copy/t';
import type { Tone } from '$lib/ui/types';

export type StatusTone = Tone | 'progress' | 'muted';

export interface StatusEntry {
	tone: StatusTone;
	word: CopyKey;
}

export type StatusKey =
	| 'health.ok' | 'health.warn' | 'health.down' | 'health.disabled' | 'health.unknown' | 'health.checking'
	| 'member.leader' | 'member.follower' | 'member.candidate' | 'member.shutdown' | 'member.unreachable' | 'member.lagging'
	| 'server.up' | 'server.down' | 'server.self' | 'server.unknown'
	| 'quorum.ok' | 'quorum.at_risk' | 'quorum.lost'
	| 'link.active' | 'link.used' | 'link.expired' | 'link.revoked'
	| 'job.queued' | 'job.running' | 'job.succeeded' | 'job.partial' | 'job.failed' | 'job.cancelled' | 'job.cancelling'
	| 'transfer.queued' | 'transfer.uploading' | 'transfer.downloading' | 'transfer.paused' | 'transfer.stalled' | 'transfer.verifying' | 'transfer.done' | 'transfer.failed' | 'transfer.cancelled' | 'transfer.conflict'
	| 'piece.ok' | 'piece.missing' | 'piece.corrupt' | 'piece.unreachable' | 'piece.repaired'
	| 'durability.full' | 'durability.reduced' | 'durability.critical' | 'durability.unreadable'
	| 'file.locked' | 'file.split' | 'file.remote' | 'file.backup_ok' | 'file.backup_failed' | 'file.no_owner' | 'file.no_tag' | 'file.hidden'
	| 'source.default' | 'source.file' | 'source.env' | 'source.runtime'
	| 'setting.saved' | 'setting.pending_restart' | 'setting.applies_now' | 'setting.invalid' | 'setting.locked_env'
	| 'conn.live' | 'conn.stale' | 'conn.reconnecting' | 'conn.offline' | 'conn.paused'
	| 'finding.critical' | 'finding.warning' | 'finding.info' | 'finding.muted' | 'finding.ignored'
	| 'backup.on_strict' | 'backup.on_best_effort' | 'backup.off'
	| 'tls.on' | 'tls.off' | 'tls.fallback' | 'tls.expiring' | 'tls.expired'
	| 'session.active' | 'session.expiring' | 'session.expired'
	| 'storage.on' | 'storage.off' | 'storage.error'
	| 'metric.reported' | 'metric.not_reported'
	| 'lock.held' | 'lock.expiring' | 'lock.lost';

export const STATUS: Record<StatusKey, StatusEntry> = {
	'health.ok': { tone: 'ok', word: 'status.health.ok' },
	'health.warn': { tone: 'warn', word: 'status.health.warn' },
	'health.down': { tone: 'danger', word: 'status.health.down' },
	'health.disabled': { tone: 'neutral', word: 'status.health.disabled' },
	'health.unknown': { tone: 'neutral', word: 'status.health.unknown' },
	'health.checking': { tone: 'progress', word: 'status.health.checking' },
	'member.leader': { tone: 'accent', word: 'status.member.leader' },
	'member.follower': { tone: 'ok', word: 'status.member.follower' },
	'member.candidate': { tone: 'warn', word: 'status.member.candidate' },
	'member.shutdown': { tone: 'neutral', word: 'status.member.shutdown' },
	'member.unreachable': { tone: 'danger', word: 'status.member.unreachable' },
	'member.lagging': { tone: 'warn', word: 'status.member.lagging' },
	'server.up': { tone: 'ok', word: 'status.server.up' },
	'server.down': { tone: 'danger', word: 'status.server.down' },
	'server.self': { tone: 'accent', word: 'status.server.self' },
	'server.unknown': { tone: 'neutral', word: 'status.server.unknown' },
	'quorum.ok': { tone: 'ok', word: 'status.quorum.ok' },
	'quorum.at_risk': { tone: 'warn', word: 'status.quorum.at_risk' },
	'quorum.lost': { tone: 'danger', word: 'status.quorum.lost' },
	'link.active': { tone: 'ok', word: 'status.link.active' },
	'link.used': { tone: 'neutral', word: 'status.link.used' },
	'link.expired': { tone: 'neutral', word: 'status.link.expired' },
	'link.revoked': { tone: 'warn', word: 'status.link.revoked' },
	'job.queued': { tone: 'neutral', word: 'status.job.queued' },
	'job.running': { tone: 'progress', word: 'status.job.running' },
	'job.succeeded': { tone: 'ok', word: 'status.job.succeeded' },
	'job.partial': { tone: 'warn', word: 'status.job.partial' },
	'job.failed': { tone: 'danger', word: 'status.job.failed' },
	'job.cancelled': { tone: 'neutral', word: 'status.job.cancelled' },
	'job.cancelling': { tone: 'progress', word: 'status.job.cancelling' },
	'transfer.queued': { tone: 'neutral', word: 'status.transfer.queued' },
	'transfer.uploading': { tone: 'progress', word: 'status.transfer.uploading' },
	'transfer.downloading': { tone: 'progress', word: 'status.transfer.downloading' },
	'transfer.paused': { tone: 'neutral', word: 'status.transfer.paused' },
	'transfer.stalled': { tone: 'warn', word: 'status.transfer.stalled' },
	'transfer.verifying': { tone: 'progress', word: 'status.transfer.verifying' },
	'transfer.done': { tone: 'ok', word: 'status.transfer.done' },
	'transfer.failed': { tone: 'danger', word: 'status.transfer.failed' },
	'transfer.cancelled': { tone: 'neutral', word: 'status.transfer.cancelled' },
	'transfer.conflict': { tone: 'warn', word: 'status.transfer.conflict' },
	'piece.ok': { tone: 'ok', word: 'status.piece.ok' },
	'piece.missing': { tone: 'danger', word: 'status.piece.missing' },
	'piece.corrupt': { tone: 'danger', word: 'status.piece.corrupt' },
	'piece.unreachable': { tone: 'warn', word: 'status.piece.unreachable' },
	'piece.repaired': { tone: 'ok', word: 'status.piece.repaired' },
	'durability.full': { tone: 'ok', word: 'status.durability.full' },
	'durability.reduced': { tone: 'warn', word: 'status.durability.reduced' },
	'durability.critical': { tone: 'danger', word: 'status.durability.critical' },
	'durability.unreadable': { tone: 'danger', word: 'status.durability.unreadable' },
	'file.locked': { tone: 'warn', word: 'status.file.locked' },
	'file.split': { tone: 'info', word: 'status.file.split' },
	'file.remote': { tone: 'info', word: 'status.file.remote' },
	'file.backup_ok': { tone: 'ok', word: 'status.file.backup_ok' },
	'file.backup_failed': { tone: 'warn', word: 'status.file.backup_failed' },
	'file.no_owner': { tone: 'warn', word: 'status.file.no_owner' },
	'file.no_tag': { tone: 'warn', word: 'status.file.no_tag' },
	'file.hidden': { tone: 'neutral', word: 'status.file.hidden' },
	'source.default': { tone: 'neutral', word: 'status.source.default' },
	'source.file': { tone: 'info', word: 'status.source.file' },
	'source.env': { tone: 'warn', word: 'status.source.env' },
	'source.runtime': { tone: 'accent', word: 'status.source.runtime' },
	'setting.saved': { tone: 'ok', word: 'status.setting.saved' },
	'setting.pending_restart': { tone: 'warn', word: 'status.setting.pending_restart' },
	'setting.applies_now': { tone: 'ok', word: 'status.setting.applies_now' },
	'setting.invalid': { tone: 'danger', word: 'status.setting.invalid' },
	'setting.locked_env': { tone: 'neutral', word: 'status.setting.locked_env' },
	'conn.live': { tone: 'ok', word: 'status.conn.live' },
	'conn.stale': { tone: 'warn', word: 'status.conn.stale' },
	'conn.reconnecting': { tone: 'progress', word: 'status.conn.reconnecting' },
	'conn.offline': { tone: 'danger', word: 'status.conn.offline' },
	'conn.paused': { tone: 'neutral', word: 'status.conn.paused' },
	'finding.critical': { tone: 'danger', word: 'status.finding.critical' },
	'finding.warning': { tone: 'warn', word: 'status.finding.warning' },
	'finding.info': { tone: 'info', word: 'status.finding.info' },
	'finding.muted': { tone: 'neutral', word: 'status.finding.muted' },
	'finding.ignored': { tone: 'neutral', word: 'status.finding.ignored' },
	'backup.on_strict': { tone: 'ok', word: 'status.backup.on_strict' },
	'backup.on_best_effort': { tone: 'info', word: 'status.backup.on_best_effort' },
	'backup.off': { tone: 'neutral', word: 'status.backup.off' },
	'tls.on': { tone: 'ok', word: 'status.tls.on' },
	'tls.off': { tone: 'danger', word: 'status.tls.off' },
	'tls.fallback': { tone: 'warn', word: 'status.tls.fallback' },
	'tls.expiring': { tone: 'warn', word: 'status.tls.expiring' },
	'tls.expired': { tone: 'danger', word: 'status.tls.expired' },
	'session.active': { tone: 'ok', word: 'status.session.active' },
	'session.expiring': { tone: 'warn', word: 'status.session.expiring' },
	'session.expired': { tone: 'danger', word: 'status.session.expired' },
	'storage.on': { tone: 'ok', word: 'status.storage.on' },
	'storage.off': { tone: 'neutral', word: 'status.storage.off' },
	'storage.error': { tone: 'danger', word: 'status.storage.error' },
	'metric.reported': { tone: 'ok', word: 'status.metric.reported' },
	'metric.not_reported': { tone: 'neutral', word: 'status.metric.not_reported' },
	'lock.held': { tone: 'warn', word: 'status.lock.held' },
	'lock.expiring': { tone: 'neutral', word: 'status.lock.expiring' },
	'lock.lost': { tone: 'danger', word: 'status.lock.lost' },
};
