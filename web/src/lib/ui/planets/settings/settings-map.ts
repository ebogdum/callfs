import type { SettingType } from '$lib/ui/organisms/SettingRow.svelte';

export const SETTINGS_GROUPS = ['network', 'limits', 'security', 'storage', 'index', 'coordination', 'logging'] as const;

export type SettingsGroup = (typeof SETTINGS_GROUPS)[number];
export type SettingsPage = SettingsGroup | 'access';
export type SettingsApply = 'restart' | 'runtime' | 'auth_reload' | 'env';

export type SettingsRiskRule =
	| { kind: 'equals'; value: unknown }
	| {
			kind:
				| 'public-host'
				| 'not-loopback'
				| 'loopback'
				| 'plain-http'
				| 'plain-http-map'
				| 'no-scheme-or-local'
				| 'weak-dsn'
				| 'unset'
				| 'changed';
	  };

export interface SettingsDependency {
	key: string;
	equals: unknown;
}

export interface SettingsEntry {
	key: string;
	page: SettingsPage;
	section: string;
	labelKey: string;
	effectKey: string;
	type: SettingType;
	apply: SettingsApply;
	defaultValue: unknown;
	options?: string[];
	recommended?: string;
	min?: number;
	max?: number;
	hasNote?: boolean;
	risk?: SettingsRiskRule;
	riskKey?: string;
	zeroLabel?: boolean;
	invert?: boolean;
	confirmWord?: string;
	dependsOn?: SettingsDependency;
}

export interface SettingsPresetDef {
	id: string;
	recommended?: boolean;
	values: Record<string, unknown>;
}

export interface SettingsSectionDef {
	id: string;
	page: SettingsPage;
	presets?: SettingsPresetDef[];
}

type RowExtra = Partial<Omit<SettingsEntry, 'key' | 'page' | 'section' | 'labelKey' | 'effectKey' | 'type' | 'apply' | 'defaultValue'>>;

const row = (
	key: string,
	page: SettingsPage,
	section: string,
	type: SettingType,
	apply: SettingsApply,
	defaultValue: unknown,
	extra: RowExtra = {}
): SettingsEntry => ({
	key,
	page,
	section,
	labelKey: `settings.label.${key}`,
	effectKey: `settings.effect.${key}`,
	type,
	apply,
	defaultValue,
	...extra
});

const LOOPBACK_HOSTS = ['127.0.0.1', 'localhost', '::1', '[::1]'];

export const SETTINGS_SECTIONS: SettingsSectionDef[] = [
	{ id: 'listener', page: 'network' },
	{ id: 'metrics-port', page: 'network' },
	{ id: 'timeouts', page: 'limits', presets: [
		{ id: 'standard', recommended: true, values: { 'server.read_timeout': '30s', 'server.write_timeout': '30s', 'server.rate_limit.download_rps': 10, 'server.rate_limit.download_burst': 5, 'server.rate_limit.link_generate_rps': 100, 'server.rate_limit.link_generate_burst': 1, 'server.rate_limit.session_rps': 1, 'server.rate_limit.session_burst': 5 } },
		{ id: 'large-files', values: { 'server.read_timeout': '0s', 'server.write_timeout': '0s' } },
		{ id: 'strict', values: { 'server.read_timeout': '10s', 'server.write_timeout': '10s', 'server.rate_limit.download_rps': 5, 'server.rate_limit.download_burst': 2, 'server.rate_limit.link_generate_rps': 20 } }
	] },
	{ id: 'index-cache', page: 'limits' },
	{ id: 'edit-locks-timeout', page: 'limits' },
	{ id: 'request-limits', page: 'limits' },
	{ id: 'console-sessions', page: 'limits' },
	{ id: 'certificate', page: 'security' },
	{ id: 'server-to-server', page: 'security' },
	{ id: 'secrets', page: 'security' },
	{ id: 'log-privacy', page: 'security' },
	{ id: 'default-storage', page: 'storage', presets: [
		{ id: 'local-only', recommended: true, values: { 'backend.default_backend': 'localfs', 'ha.replication_enabled': false } },
		{ id: 's3', values: { 'backend.default_backend': 's3' } },
		{ id: 's3-backup', values: { 'backend.default_backend': 's3', 'ha.replication_enabled': true, 'ha.replica_backend': 'localfs' } }
	] },
	{ id: 'local-disk', page: 'storage' },
	{ id: 's3-bucket', page: 'storage' },
	{ id: 'split-storage', page: 'storage' },
	{ id: 'backup-copy', page: 'storage' },
	{ id: 'trash', page: 'storage' },
	{ id: 'engine', page: 'index' },
	{ id: 'postgres', page: 'index' },
	{ id: 'sqlite', page: 'index' },
	{ id: 'redis', page: 'index' },
	{ id: 'raft-member', page: 'index' },
	{ id: 'raft-members', page: 'index' },
	{ id: 'raft-timing', page: 'index' },
	{ id: 'raft-snapshots', page: 'index' },
	{ id: 'this-server', page: 'coordination' },
	{ id: 'other-servers', page: 'coordination' },
	{ id: 'edit-locks', page: 'coordination' },
	{ id: 'logs', page: 'logging', presets: [
		{ id: 'production', recommended: true, values: { 'log.level': 'info', 'log.format': 'json' } },
		{ id: 'troubleshooting', values: { 'log.level': 'debug' } }
	] },
	{ id: 'console-history', page: 'logging' },
	{ id: 'users', page: 'access' }
];

export const SETTINGS_MAP: SettingsEntry[] = [
	row('server.listen_addr', 'network', 'listener', 'text', 'restart', ':8443', { risk: { kind: 'public-host' }, riskKey: 'settings.risk.server.listen_addr' }),
	row('server.protocol', 'network', 'listener', 'enum', 'restart', 'https', { options: ['https', 'auto', 'http'], recommended: 'https', risk: { kind: 'equals', value: 'http' }, riskKey: 'settings.risk.server.protocol' }),
	row('server.external_url', 'network', 'listener', 'text', 'restart', 'localhost:8443', { risk: { kind: 'no-scheme-or-local' }, riskKey: 'settings.risk.server.external_url' }),
	row('server.enable_quic', 'network', 'listener', 'bool', 'restart', false, { hasNote: true }),
	row('server.quic_listen_addr', 'network', 'listener', 'text', 'restart', ':8443', { dependsOn: { key: 'server.enable_quic', equals: true } }),
	row('metrics.listen_addr', 'network', 'metrics-port', 'text', 'restart', ':9090', { risk: { kind: 'not-loopback' }, riskKey: 'settings.risk.metrics.listen_addr' }),
	row('metrics.profiling', 'network', 'metrics-port', 'bool', 'restart', false, { hasNote: true, risk: { kind: 'equals', value: true }, riskKey: 'settings.risk.metrics.profiling' }),

	row('server.read_timeout', 'limits', 'timeouts', 'duration', 'restart', '30s', { hasNote: true, zeroLabel: true, risk: { kind: 'equals', value: '0s' }, riskKey: 'settings.risk.server.timeout' }),
	row('server.write_timeout', 'limits', 'timeouts', 'duration', 'restart', '30s', { hasNote: true, zeroLabel: true, risk: { kind: 'equals', value: '0s' }, riskKey: 'settings.risk.server.timeout' }),
	row('server.file_op_timeout', 'limits', 'timeouts', 'duration', 'restart', '10s'),
	row('server.metadata_op_timeout', 'limits', 'timeouts', 'duration', 'restart', '5s'),
	row('server.metadata_cache_ttl', 'limits', 'index-cache', 'duration', 'runtime', '2s', { zeroLabel: true }),
	row('dlm.wait_timeout', 'limits', 'edit-locks-timeout', 'duration', 'runtime', '30s'),
	row('server.rate_limit.download_rps', 'limits', 'request-limits', 'number', 'runtime', 10, { min: 1 }),
	row('server.rate_limit.download_burst', 'limits', 'request-limits', 'number', 'runtime', 5, { min: 1 }),
	row('server.rate_limit.link_generate_rps', 'limits', 'request-limits', 'number', 'runtime', 100, { min: 1 }),
	row('server.rate_limit.link_generate_burst', 'limits', 'request-limits', 'number', 'runtime', 1, { min: 1 }),
	row('server.rate_limit.session_rps', 'limits', 'request-limits', 'number', 'runtime', 1, { min: 1 }),
	row('server.rate_limit.session_burst', 'limits', 'request-limits', 'number', 'runtime', 5, { min: 1 }),
	row('ui.session_ttl', 'limits', 'console-sessions', 'duration', 'runtime', '8h', { hasNote: true }),
	row('ui.idle_timeout', 'limits', 'console-sessions', 'duration', 'runtime', '1h'),

	row('server.cert_file', 'security', 'certificate', 'path', 'restart', 'server.crt'),
	row('server.key_file', 'security', 'certificate', 'path', 'restart', 'server.key'),
	row('backend.internal_proxy_skip_tls_verify', 'security', 'server-to-server', 'bool', 'restart', false, { invert: true, risk: { kind: 'equals', value: true }, riskKey: 'settings.risk.backend.internal_proxy_skip_tls_verify' }),
	row('auth.internal_proxy_secret', 'security', 'secrets', 'secret', 'restart', null, { hasNote: true }),
	row('auth.single_use_link_secret', 'security', 'secrets', 'secret', 'restart', null, { hasNote: true }),
	row('log_mode', 'security', 'log-privacy', 'enum', 'env', 'production', { options: ['production', 'development', 'debug'], recommended: 'production' }),

	row('performance.mode', 'storage', 'default-storage', 'enum', 'restart', 'fast', { options: ['fast', 'strict'], recommended: 'fast', hasNote: true }),
	row('backend.default_backend', 'storage', 'default-storage', 'enum', 'restart', 'localfs', { options: ['localfs', 's3'], hasNote: true }),
	row('backend.localfs_root_path', 'storage', 'local-disk', 'path', 'restart', '/var/lib/callfs', { hasNote: true }),
	row('backend.localfs_sync', 'storage', 'local-disk', 'enum', 'restart', 'full', { options: ['full', 'barrier', 'none'], recommended: 'full', hasNote: true }),
	row('backend.localfs_ack', 'storage', 'local-disk', 'enum', 'restart', 'disk', { options: ['memory', 'disk'], hasNote: true }),
	row('backend.flush_workers', 'storage', 'local-disk', 'number', 'restart', 0, { zeroLabel: true }),
	row('backend.flush_max_pending_bytes', 'storage', 'local-disk', 'size', 'restart', 268435456),
	row('backend.flush_max_delay', 'storage', 'local-disk', 'duration', 'restart', '0s', { zeroLabel: true }),
	row('backend.s3_bucket_name', 'storage', 's3-bucket', 'text', 'restart', ''),
	row('backend.s3_region', 'storage', 's3-bucket', 'text', 'restart', 'us-east-1'),
	row('backend.s3_endpoint', 'storage', 's3-bucket', 'text', 'restart', '', { risk: { kind: 'plain-http' }, riskKey: 'settings.risk.backend.s3_endpoint' }),
	row('backend.s3_access_key', 'storage', 's3-bucket', 'secret', 'restart', null),
	row('backend.s3_secret_key', 'storage', 's3-bucket', 'secret', 'restart', null),
	row('backend.s3_server_side_encryption', 'storage', 's3-bucket', 'enum', 'restart', 'AES256', { options: ['AES256', 'aws-kms', 'off'], recommended: 'AES256', hasNote: true }),
	row('backend.s3_kms_key_id', 'storage', 's3-bucket', 'text', 'restart', '', { dependsOn: { key: 'backend.s3_server_side_encryption', equals: 'aws-kms' } }),
	row('backend.s3_acl', 'storage', 's3-bucket', 'enum', 'restart', 'private', { options: ['private', 'public-read', 'other'], recommended: 'private', risk: { kind: 'equals', value: 'public-read' }, riskKey: 'settings.risk.backend.s3_acl', confirmWord: 'PUBLIC' }),
	row('erasure.enabled', 'storage', 'split-storage', 'bool', 'restart', false, { hasNote: true }),
	row('erasure.data_shards', 'storage', 'split-storage', 'number', 'restart', 4, { min: 2, max: 32, hasNote: true }),
	row('erasure.parity_shards', 'storage', 'split-storage', 'number', 'restart', 2, { min: 1, max: 32, hasNote: true }),
	row('erasure.min_file_size', 'storage', 'split-storage', 'size', 'restart', 0, { zeroLabel: true }),
	row('erasure.shard_backend', 'storage', 'split-storage', 'enum', 'restart', 'localfs', { options: ['localfs', 's3'] }),
	row('erasure.shard_path', 'storage', 'split-storage', 'path', 'restart', '', { hasNote: true }),
	row('ha.replication_enabled', 'storage', 'backup-copy', 'bool', 'restart', false),
	row('ha.replica_backend', 'storage', 'backup-copy', 'enum', 'restart', '', { options: ['localfs', 's3'], hasNote: true }),
	row('ha.require_replica_success', 'storage', 'backup-copy', 'bool', 'restart', false, { risk: { kind: 'equals', value: true }, riskKey: 'settings.risk.ha.require_replica_success' }),
	row('trash.enabled', 'storage', 'trash', 'bool', 'runtime', true),
	row('trash.retention', 'storage', 'trash', 'duration', 'runtime', '720h', { hasNote: true }),

	row('metadata_store.type', 'index', 'engine', 'enum', 'restart', 'postgres', { options: ['sqlite', 'postgres', 'redis', 'raft'], recommended: 'postgres', risk: { kind: 'changed' }, riskKey: 'settings.risk.metadata_store.type', confirmWord: 'SWITCH' }),
	row('metadata_store.dsn', 'index', 'postgres', 'text', 'restart', 'postgres://callfs:callfs@localhost/callfs?sslmode=require', { risk: { kind: 'weak-dsn' }, riskKey: 'settings.risk.metadata_store.dsn' }),
	row('metadata_store.sqlite_path', 'index', 'sqlite', 'path', 'restart', './callfs.sqlite3', { hasNote: true }),
	row('metadata_store.redis_addr', 'index', 'redis', 'text', 'restart', 'localhost:6379'),
	row('metadata_store.redis_password', 'index', 'redis', 'secret', 'restart', null, { risk: { kind: 'unset' }, riskKey: 'settings.risk.metadata_store.redis_password' }),
	row('metadata_store.redis_db', 'index', 'redis', 'number', 'restart', 0, { min: 0, max: 15 }),
	row('metadata_store.redis_key_prefix', 'index', 'redis', 'text', 'restart', 'callfs:', { hasNote: true }),
	row('raft.enabled', 'index', 'raft-member', 'bool', 'restart', false, { hasNote: true }),
	row('raft.node_id', 'index', 'raft-member', 'text', 'restart', 'callfs-node-1', { hasNote: true }),
	row('raft.bind_addr', 'index', 'raft-member', 'text', 'restart', '127.0.0.1:7000', { hasNote: true, risk: { kind: 'loopback' }, riskKey: 'settings.risk.raft.bind_addr' }),
	row('raft.data_dir', 'index', 'raft-member', 'path', 'restart', './raft'),
	row('raft.bootstrap', 'index', 'raft-member', 'bool', 'restart', false, { risk: { kind: 'equals', value: true }, riskKey: 'settings.risk.raft.bootstrap' }),
	row('raft.peers', 'index', 'raft-members', 'map', 'restart', {}, { hasNote: true }),
	row('raft.api_peer_endpoints', 'index', 'raft-members', 'map', 'restart', {}, { risk: { kind: 'plain-http-map' }, riskKey: 'settings.risk.peer_endpoints' }),
	row('raft.apply_timeout', 'index', 'raft-timing', 'duration', 'restart', '10s'),
	row('raft.forward_timeout', 'index', 'raft-timing', 'duration', 'restart', '10s'),
	row('raft.snapshot_interval', 'index', 'raft-snapshots', 'duration', 'restart', '60s'),
	row('raft.snapshot_threshold', 'index', 'raft-snapshots', 'number', 'restart', 256, { min: 1 }),
	row('raft.retain_snapshot_count', 'index', 'raft-snapshots', 'number', 'restart', 2, { min: 1 }),

	row('instance_discovery.instance_id', 'coordination', 'this-server', 'text', 'restart', 'callfs-instance-1', { risk: { kind: 'equals', value: 'callfs-instance-1' }, riskKey: 'settings.risk.instance_discovery.instance_id' }),
	row('instance_discovery.peer_endpoints', 'coordination', 'other-servers', 'map', 'restart', {}, { risk: { kind: 'plain-http-map' }, riskKey: 'settings.risk.peer_endpoints' }),
	row('dlm.type', 'coordination', 'edit-locks', 'enum', 'restart', 'redis', { options: ['redis', 'local'], recommended: 'redis', risk: { kind: 'equals', value: 'local' }, riskKey: 'settings.risk.dlm.type' }),
	row('dlm.redis_addr', 'coordination', 'edit-locks', 'text', 'restart', 'localhost:6379'),
	row('dlm.redis_password', 'coordination', 'edit-locks', 'secret', 'restart', null),

	row('log.level', 'logging', 'logs', 'enum', 'runtime', 'info', { options: ['error', 'warn', 'info', 'debug'], recommended: 'info', risk: { kind: 'equals', value: 'debug' }, riskKey: 'settings.risk.log.level' }),
	row('log.format', 'logging', 'logs', 'enum', 'restart', 'json', { options: ['json', 'text'], recommended: 'json' }),
	row('ui.log_buffer_entries', 'logging', 'console-history', 'number', 'restart', 5000, { min: 1000, max: 100000 }),
	row('ui.audit_buffer_entries', 'logging', 'console-history', 'number', 'restart', 20000, { min: 100, max: 1000000 }),
	row('ui.event_buffer_entries', 'logging', 'console-history', 'number', 'restart', 5000, { min: 100, max: 1000000 }),
	row('ui.metrics_sample_interval', 'logging', 'console-history', 'enum', 'restart', '10s', { options: ['5s', '10s', '30s'], recommended: '10s' }),
	row('ui.metrics_retention', 'logging', 'console-history', 'enum', 'restart', '24h', { options: ['6h', '24h', '168h'], recommended: '24h' }),
	row('ui.enabled', 'logging', 'console-history', 'bool', 'restart', true, { risk: { kind: 'equals', value: false }, riskKey: 'settings.risk.ui.enabled' }),
	row('ui.allow_remote_restart', 'logging', 'console-history', 'bool', 'restart', false, { hasNote: true }),

	row('auth.api_key_users', 'access', 'users', 'map', 'auth_reload', {}),
	row('auth.api_keys', 'access', 'users', 'list', 'auth_reload', []),
	row('auth.admin_users', 'access', 'users', 'list', 'auth_reload', [])
];

export function entriesFor(group: SettingsGroup): SettingsEntry[] {
	return SETTINGS_MAP.filter((entry) => group === entry.page);
}

export function sectionsFor(group: SettingsGroup): SettingsSectionDef[] {
	return SETTINGS_SECTIONS.filter((section) => group === section.page);
}

export function entryFor(key: string): SettingsEntry | undefined {
	return SETTINGS_MAP.find((entry) => entry.key === key);
}

function hostOf(address: string): string {
	return address.slice(0, address.lastIndexOf(':'));
}

export function isLoopbackHost(address: string): boolean {
	return LOOPBACK_HOSTS.includes(hostOf(address));
}

function anyPlainHttp(value: unknown): boolean {
	if (null === value || 'object' !== typeof value) {
		return false;
	}
	return Object.values(value).some((item) => 'string' === typeof item && item.startsWith('http://'));
}

export function riskApplies(entry: SettingsEntry, value: unknown, isSet: boolean, changed: boolean): boolean {
	if (undefined === entry.risk) {
		return false;
	}
	const rule = entry.risk;
	if ('equals' === rule.kind) {
		return sameRiskValue(rule.value, value);
	}
	const text = 'string' === typeof value ? value : '';
	switch (rule.kind) {
		case 'public-host':
			return '' !== text && ('' === hostOf(text) || ['0.0.0.0', '[::]', '::'].includes(hostOf(text)));
		case 'not-loopback':
			return '' !== text && !isLoopbackHost(text);
		case 'loopback':
			return '' !== text && isLoopbackHost(text);
		case 'plain-http':
			return text.startsWith('http://');
		case 'plain-http-map':
			return anyPlainHttp(value);
		case 'no-scheme-or-local':
			return !/^https?:\/\//.test(text) || text.includes('localhost');
		case 'weak-dsn':
			return text.includes('callfs:callfs@') || text.includes('sslmode=disable');
		case 'unset':
			return !isSet;
		case 'changed':
			return changed;
	}
}

function sameRiskValue(expected: unknown, value: unknown): boolean {
	return JSON.stringify(expected) === JSON.stringify(value);
}
