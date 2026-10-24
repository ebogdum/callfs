import type { MockResponse, MockTable } from './server';

const BASE = '2026-10-23T12:00:00Z';
const BASE_S = Date.parse(BASE) / 1000;
const EXPIRES = '2026-10-23T20:00:00Z';
const SERIES_POINTS = 288;
const STEP_S = 300;

export const ok = (body: unknown): MockResponse => ({ status: 200, body });
const created = (body: unknown): MockResponse => ({ status: 201, body });
const accepted = (body: unknown): MockResponse => ({ status: 202, body });
const noContent: MockResponse = { status: 204, body: null };

const folderItem = (name: string, parent: string, owner: string, mtime: string) => ({
	path: `${parent}${name}`,
	name,
	type: 'directory',
	size: 0,
	owner,
	mtime,
	storage: 'localfs',
	instance_id: 'callfs-node-1',
	etag: '',
	erasure_coded: false,
	locked: false
});

const fileItem = (name: string, parent: string, size: number, owner: string, mtime: string, erasure: boolean) => ({
	path: `${parent}${name}`,
	name,
	type: 'file',
	size,
	owner,
	mtime,
	storage: erasure ? 'erasure' : 'localfs',
	instance_id: 'callfs-node-1',
	etag: `"etag-${name}"`,
	erasure_coded: erasure,
	locked: false
});

const rootItems = [
	folderItem('team', '/', 'alice', '2026-10-22T09:00:00Z'),
	folderItem('archive', '/', 'alice', '2026-09-01T09:00:00Z'),
	fileItem('report.pdf', '/', 2411724, 'alice', '2026-10-22T10:15:00Z', true),
	fileItem('notes.txt', '/', 1842, 'bob', '2026-10-21T16:40:00Z', false)
];

const HUGE_ITEM_COUNT = 200;

export const hugeFolderBody = {
	path: '/huge',
	type: 'directory',
	count: HUGE_ITEM_COUNT,
	total: 48210,
	items: Array.from({ length: HUGE_ITEM_COUNT }, (_, index) =>
		fileItem(`batch-${String(index + 1).padStart(5, '0')}.bin`, '/huge/', 1048576 + index, 'alice', BASE, false)
	),
	next_cursor: 'c200',
	partial: true
};

const LINK_PATHS = ['/team/report.pdf', '/team/notes.txt', '/archive/2025.zip'];
const LINK_STATUSES: string[] = [
	...Array(9).fill('active'),
	...Array(8).fill('used'),
	...Array(4).fill('expired'),
	...Array(2).fill('revoked')
];

const linkItems = LINK_STATUSES.map((status, index) => ({
	id: index + 1,
	token_prefix: `tk${String(index + 1).padStart(2, '0')}a9`,
	file_path: LINK_PATHS[index % LINK_PATHS.length],
	status,
	created_by: 0 === index % 2 ? 'alice' : 'bob',
	created_at: '2026-10-22T08:00:00Z',
	expires_at: 'expired' === status ? '2026-10-22T10:00:00Z' : '2026-10-24T08:00:00Z',
	used_at: 'used' === status ? '2026-10-23T09:30:00Z' : null,
	used_by_ip: 'used' === status ? `203.0.113.${10 + index}` : null
}));

const linkCounts = {
	active: LINK_STATUSES.filter((status) => 'active' === status).length,
	used: LINK_STATUSES.filter((status) => 'used' === status).length,
	expired: LINK_STATUSES.filter((status) => 'expired' === status).length,
	revoked: LINK_STATUSES.filter((status) => 'revoked' === status).length
};

const erasureItems = Array.from({ length: 12 }, (_, index) => {
	const reduced = 0 === index;
	return {
		path: `/archive/split-${String(index + 1).padStart(2, '0')}.bin`,
		original_size: 524288000,
		data_shards: 4,
		parity_shards: 2,
		shard_size: 131072000,
		shards_total: 6,
		healthy_shards: reduced ? 5 : 6,
		last_checked_at: BASE,
		health: reduced ? 'reduced' : 'ok'
	};
});

const erasureDetail = {
	path: '/archive/split-01.bin',
	original_size: 524288000,
	profile: { data_shards: 4, parity_shards: 2, shard_size: 131072000 },
	shards: Array.from({ length: 6 }, (_, index) => ({
		index,
		kind: index < 4 ? 'data' : 'parity',
		instance_id: `callfs-node-${(index % 3) + 1}`,
		storage: 'localfs',
		path: `/.callfs/pieces/split-01/${index}`,
		size: 131072000,
		checksum: `sha256:${String(index).repeat(8)}`,
		state: 1 === index ? 'missing' : 'ok'
	})),
	tolerance: 2,
	last_checked_at: BASE
};

const jobBase = (id: string, kind: string, state: string, done: number, total: number | null, createdBy: string) => ({
	id,
	kind,
	state,
	phase: 'running' === state ? 'copy' : null,
	progress: { done, total, bytes_done: done * 1024, bytes_total: null === total ? null : total * 1024, per_second: 'running' === state ? 12.5 : 0 },
	created_by: createdBy,
	created_at: BASE,
	started_at: 'queued' === state ? null : BASE,
	finished_at: ['succeeded', 'partial', 'failed', 'cancelled'].includes(state) ? BASE : null,
	error: 'failed' === state ? 'Storage did not answer.' : null,
	failed_items: ['partial', 'failed'].includes(state)
		? [{ path: '/team/locked.txt', code: 'RESOURCE_LOCKED', message: 'File is being changed.' }]
		: [],
	failed_count: ['partial', 'failed'].includes(state) ? 1 : 0,
	result: {}
});

const JOBS = [
	jobBase('job-1', 'delete_prefix', 'queued', 0, 1204, 'alice'),
	jobBase('job-2', 'batch', 'running', 1180, 1204, 'alice'),
	jobBase('job-3', 'retag', 'succeeded', 48210, 48210, 'alice'),
	jobBase('job-4', 'batch', 'partial', 1180, 1204, 'alice'),
	jobBase('job-5', 'erasure_repair', 'failed', 3, 6, 'alice'),
	jobBase('job-6', 'delete_recursive', 'cancelled', 400, 1204, 'bob'),
	jobBase('job-7', 'replication_retry', 'cancelling', 5, 7, 'alice')
];

const EVENTS = [
	{ id: 'evt-1', type: 'config.changed', level: 'info', message: 'Log level changed to info.' },
	{ id: 'evt-2', type: 'cluster.member_joined', level: 'info', message: 'callfs-node-1 joined the cluster.' },
	{ id: 'evt-3', type: 'server.started', level: 'info', message: 'callfs-node-1 started.' },
	{ id: 'evt-4', type: 'job.failed', level: 'error', message: 'Erasure repair failed for 3 pieces.' },
	{ id: 'evt-5', type: 'link.created', level: 'info', message: 'Download link created for /team/report.pdf.' },
	{ id: 'evt-6', type: 'file.created', level: 'info', message: 'alice uploaded /team/report.pdf.' }
].map((event, index) => ({
	...event,
	at: new Date((BASE_S - index * 600) * 1000).toISOString(),
	actor: 'alice',
	instance_id: 'callfs-node-1',
	entities: [],
	fields: {}
}));

const FINDINGS = [
	{
		id: 'secret.default',
		severity: 'critical',
		area: 'security',
		title: 'Default secret in use',
		body: 'The internal secret is the example value. Anyone who reads the example config can forge requests.',
		affected: [{ kind: 'setting', id: 'internal_proxy.secret', href: '/settings/security' }],
		since: BASE,
		fix: { label: 'Change secret', route: '/settings/security', setting_key: 'internal_proxy.secret', command: null },
		muted: null
	},
	{
		id: 'tls.cert_expiring',
		severity: 'warning',
		area: 'security',
		title: 'Certificate expires in 81 days',
		body: 'Renew the certificate before it expires. Clients refuse the server after that.',
		affected: [{ kind: 'setting', id: 'server.tls_cert', href: '/settings/security' }],
		since: BASE,
		fix: { label: 'Open Security', route: '/settings/security', setting_key: null, command: null },
		muted: null
	},
	{
		id: 'secret.short',
		severity: 'warning',
		area: 'security',
		title: 'Secret is short',
		body: 'The link secret has fewer than 32 characters. Use a longer random value.',
		affected: [{ kind: 'setting', id: 'links.secret', href: '/settings/security' }],
		since: BASE,
		fix: { label: 'Change secret', route: '/settings/security', setting_key: 'links.secret', command: null },
		muted: null
	},
	{
		id: 'keys.positional',
		severity: 'warning',
		area: 'security',
		title: 'Some keys share a numbered user',
		body: 'Three keys get their user from their place in the list. Give each key its own user.',
		affected: [{ kind: 'user', id: 'api-user-0', href: '/settings/access' }],
		since: BASE,
		fix: { label: 'Fix this', route: '/settings/access', setting_key: null, command: null },
		muted: null
	},
	{
		id: 'erasure.damaged',
		severity: 'warning',
		area: 'storage',
		title: '1 split file needs repair',
		body: 'One split file has a missing piece. It still reads, but one more loss breaks it.',
		affected: [{ kind: 'file', id: '/archive/split-01.bin', href: '/cluster/durability/file/archive/split-01.bin' }],
		since: BASE,
		fix: { label: 'Repair', route: '/cluster/durability', setting_key: null, command: 'erasure.repair' },
		muted: null
	},
	{
		id: 'backup.failing',
		severity: 'warning',
		area: 'storage',
		title: 'Backup copies are failing',
		body: 'Seven backup copies failed in the last hour. Retry them from the backup page.',
		affected: [{ kind: 'server', id: 'archive-1', href: '/cluster/backup' }],
		since: BASE,
		fix: { label: 'Open Backup copy', route: '/cluster/backup', setting_key: null, command: null },
		muted: null
	},
	{
		id: 'links.failed_opens',
		severity: 'warning',
		area: 'runtime',
		title: 'Many download links fail',
		body: 'Opens failed for more than 20 percent of links in the last hour. Check expired links.',
		affected: [{ kind: 'link', id: '12', href: '/sharing/12' }],
		since: BASE,
		fix: { label: 'Open Sharing', route: '/sharing', setting_key: null, command: null },
		muted: null
	},
	{
		id: 'auth.failures_spike',
		severity: 'warning',
		area: 'security',
		title: 'Many failed sign-ins',
		body: 'More than 100 sign-ins failed in ten minutes. Check the sign-in log for the source.',
		affected: [{ kind: 'user', id: 'carol', href: '/activity/sign-ins' }],
		since: BASE,
		fix: { label: 'Open Sign-ins', route: '/activity/sign-ins', setting_key: null, command: null },
		muted: null
	},
	{
		id: 'hygiene.no_owner',
		severity: 'info',
		area: 'storage',
		title: '3 files have no owner',
		body: 'Files without an owner cannot be changed by members. Assign an owner.',
		affected: [{ kind: 'folder', id: '/archive', href: '/cluster/hygiene' }],
		since: BASE,
		fix: { label: 'Open Hygiene', route: '/cluster/hygiene', setting_key: null, command: null },
		muted: null
	}
];

const HEALTH_COMPONENTS = [
	'file_index',
	'locks',
	'storage.localfs',
	'storage.s3',
	'storage.internal_proxy',
	'nats',
	'metadata_log',
	'erasure',
	'tls',
	'disk',
	'replication',
	'link_cleanup'
].map((name) => ({
	name,
	status: 'tls' === name ? 'warn' : 'ok',
	latency_ms: 3,
	detail: 'Reachable',
	since: BASE
}));

const INSTANCES = [
	{ instance_id: 'callfs-node-1', endpoint: 'https://node-1.example.test', source: 'self', is_self: true, scheme: 'https', reachable: true, rtt_ms: 0, checked_at: BASE, files: 16403, bytes: 400000000000, pieces: 4, disk_free_bytes: null, disk_total_bytes: null },
	{ instance_id: 'callfs-node-2', endpoint: 'https://node-2.example.test', source: 'config', is_self: false, scheme: 'https', reachable: true, rtt_ms: 14, checked_at: BASE, files: 16104, bytes: 400000000000, pieces: 4, disk_free_bytes: null, disk_total_bytes: null },
	{ instance_id: 'callfs-node-3', endpoint: 'https://node-3.example.test', source: 'config', is_self: false, scheme: 'https', reachable: true, rtt_ms: 18, checked_at: BASE, files: 15703, bytes: 400000000000, pieces: 4, disk_free_bytes: null, disk_total_bytes: null },
	{ instance_id: 'archive-1', endpoint: 'http://archive.example.test', source: 'joined', is_self: false, scheme: 'http', reachable: false, rtt_ms: null, checked_at: BASE, files: 0, bytes: 0, pieces: 0, disk_free_bytes: null, disk_total_bytes: null }
];

const BACKENDS = [
	{ name: 'localfs', enabled: true, status: 'ok', root_path: '/var/lib/callfs/files', free_bytes: 320000000000, total_bytes: 500000000000, used_bytes: 180000000000, bucket: null, region: null, endpoint: null, encryption: null, acl: null, kms_key_set: false, peers: 0, last_probe: { at: BASE, ok: true, latency_ms: 2, error: null } },
	{ name: 's3', enabled: true, status: 'ok', root_path: null, free_bytes: null, total_bytes: null, used_bytes: 220000000000, bucket: 'callfs-files', region: 'eu-central-1', endpoint: null, encryption: 'AES256', acl: 'private', kms_key_set: false, peers: 0, last_probe: { at: BASE, ok: true, latency_ms: 41, error: null } },
	{ name: 'internal_proxy', enabled: true, status: 'ok', root_path: null, free_bytes: null, total_bytes: null, used_bytes: null, bucket: null, region: null, endpoint: null, encryption: null, acl: null, kms_key_set: false, peers: 3, last_probe: { at: BASE, ok: true, latency_ms: 14, error: null } },
	{ name: 'erasure', enabled: true, status: 'warn', root_path: null, free_bytes: null, total_bytes: null, used_bytes: 96000000000, bucket: null, region: null, endpoint: null, encryption: null, acl: null, kms_key_set: false, peers: 4, last_probe: { at: BASE, ok: false, latency_ms: 0, error: 'One piece is missing.' } }
];

const CLUSTER_MEMBERS = [
	{ node_id: 'callfs-node-1', instance_id: 'callfs-node-1', api_endpoint: 'https://node-1.example.test', state: 'leader', is_self: true, online: true, last_seen_ms: 0, behind_by: 0 },
	{ node_id: 'callfs-node-2', instance_id: 'callfs-node-2', api_endpoint: 'https://node-2.example.test', state: 'follower', is_self: false, online: true, last_seen_ms: 22, behind_by: 0 },
	{ node_id: 'callfs-node-3', instance_id: 'callfs-node-3', api_endpoint: 'https://node-3.example.test', state: 'follower', is_self: false, online: true, last_seen_ms: 31, behind_by: 2 }
];

export const CLUSTER = {
	mode: 'nats',
	self_id: 'callfs-node-1',
	cluster_name: 'callfs',
	leader_id: 'callfs-node-1',
	leader_endpoint: 'https://node-1.example.test',
	replicas: 3,
	has_quorum: true,
	can_lose: 1,
	log: {
		first_seq: 1,
		last_seq: 49426,
		messages: 49426,
		bytes: 8123450,
		applied_seq: 49426,
		last_snapshot: { id: 'snap-2', seq: 48000, bytes: 2090000, created_at: BASE },
		snapshot_interval_ms: 300000
	},
	members: CLUSTER_MEMBERS
};

const USERS = [
	{ user_id: 'alice', source: 'api_key_users', index: null, key_hint: '…a1b2', key_fingerprint: 'sha256:1f0c9a7e3b2d4c51', is_admin: true, owned_files: 30120, owned_bytes: 800000000000, last_auth_at: BASE, requests_24h: 9120, auth_failures_24h: 0 },
	{ user_id: 'bob', source: 'api_key_users', index: null, key_hint: '…c3d4', key_fingerprint: 'sha256:7a2e11b9c0d84f62', is_admin: false, owned_files: 14200, owned_bytes: 300000000000, last_auth_at: BASE, requests_24h: 2401, auth_failures_24h: 0 },
	{ user_id: 'carol', source: 'api_keys', index: 0, key_hint: '…e5f6', key_fingerprint: 'sha256:4d9b03c8e1a27f55', is_admin: false, owned_files: 3890, owned_bytes: 100000000000, last_auth_at: '2026-10-22T18:00:00Z', requests_24h: 0, auth_failures_24h: 38 }
];

const METRIC_FAMILIES = [
	['callfs_http_requests_total', 'counter', 'Requests'],
	['callfs_http_request_duration_seconds', 'histogram', 'Response time'],
	['callfs_backend_ops_total', 'counter', 'Storage actions'],
	['callfs_backend_op_duration_seconds', 'histogram', 'Storage action time'],
	['callfs_metadata_db_queries_total', 'counter', 'Index queries'],
	['callfs_metadata_db_query_duration_seconds', 'histogram', 'Index query time'],
	['callfs_single_use_link_generations_total', 'counter', 'Links created'],
	['callfs_single_use_link_consumptions_total', 'counter', 'Link opens'],
	['callfs_lock_operations_total', 'counter', 'Lock actions'],
	['callfs_lock_operation_duration_seconds', 'histogram', 'Lock wait time'],
	['callfs_active_locks', 'gauge', 'Locks held'],
	['callfs_file_operations_total', 'counter', 'File actions'],
	['callfs_errors_total', 'counter', 'Errors'],
	['callfs_cache_lookups_total', 'counter', 'Index cache lookups'],
	['callfs_cache_evictions_total', 'counter', 'Cache evictions'],
	['callfs_cache_entries', 'gauge', 'Cache entries'],
	['callfs_rate_limit_rejections_total', 'counter', 'Request limit rejections'],
	['callfs_replication_total', 'counter', 'Backup copies'],
	['callfs_transfers_active', 'gauge', 'Transfers running'],
	['callfs_auth_failures_total', 'counter', 'Failed sign-ins'],
	['callfs_nats_connected', 'gauge', 'NATS running'],
	['callfs_nats_routes', 'gauge', 'Cluster routes'],
	['callfs_nats_stream_messages', 'gauge', 'Stored messages'],
	['callfs_jobs_total', 'counter', 'Jobs finished'],
	['callfs_erasure_pieces_repaired_total', 'counter', 'Pieces repaired'],
	['callfs_usage_bytes', 'gauge', 'Bytes stored'],
	['callfs_usage_files', 'gauge', 'Files stored']
] as const;

const seriesPoints = (seed: number): [number, number | null][] =>
	Array.from({ length: SERIES_POINTS }, (_, index) => {
		const at = BASE_S - (SERIES_POINTS - 1 - index) * STEP_S;
		const value = 40 + ((index * 7 + seed) % 23);
		return [at, 0 === (index + seed) % 97 ? null : value];
	});

const WORKERS = ['link_cleanup', 'trash_cleanup', 'upload_cleanup', 'instance_prober', 'metrics_sampler', 'usage_refresh'].map(
	(name) => ({
		name,
		interval_s: 300,
		running: false,
		last_run_at: BASE,
		last_duration_ms: 12,
		last_result: {},
		last_error: null,
		next_run_at: '2026-10-23T12:05:00Z'
	})
);

const TRASH_ITEMS = [
	{ id: 'trash-1', original_path: '/team/old-notes.txt', name: 'old-notes.txt', type: 'file', size: 2048, deleted_by: 'alice', deleted_at: '2026-10-20T09:00:00Z', expires_at: '2026-11-19T09:00:00Z' },
	{ id: 'trash-2', original_path: '/archive/2024', name: '2024', type: 'directory', size: 1048576, deleted_by: 'bob', deleted_at: '2026-10-18T09:00:00Z', expires_at: '2026-11-17T09:00:00Z' }
];

const SEARCH_ITEMS = [
	fileItem('report-v1.pdf', '/team/', 2411724, 'alice', BASE, true),
	fileItem('report-v2.pdf', '/team/', 2411800, 'alice', BASE, false),
	fileItem('report-final.pdf', '/archive/', 2411900, 'bob', BASE, false)
];

const LOG_ITEMS = [
	{ id: 'log-1', at: BASE, level: 'info', message: 'Request served.', logger: 'http', fields: {}, request_id: 'req-1' },
	{ id: 'log-2', at: BASE, level: 'warn', message: 'Slow metadata query.', logger: 'metadata', fields: {}, request_id: null },
	{ id: 'log-3', at: BASE, level: 'error', message: 'Backup copy failed.', logger: 'replication', fields: {}, request_id: 'req-3' },
	{ id: 'log-4', at: BASE, level: 'debug', message: 'Cache entry refreshed.', logger: 'cache', fields: {}, request_id: null }
];

const AUDIT_ITEMS = [
	{ at: BASE, request_id: 'req-1', user_id: 'alice', method: 'PUT', route: '/v1/files/*', path: '/team/report.pdf', op: 'upload', status: 201, bytes_in: 2411724, bytes_out: 0, duration_ms: 84, instance_id: 'callfs-node-1', client: '198.51.100.7' },
	{ at: BASE, request_id: 'req-2', user_id: 'bob', method: 'DELETE', route: '/v1/files/*', path: '/archive/2024', op: 'delete', status: 200, bytes_in: 0, bytes_out: 0, duration_ms: 31, instance_id: 'callfs-node-1', client: '198.51.100.8' },
	{ at: BASE, request_id: 'req-3', user_id: 'alice', method: 'POST', route: '/v1/links/generate', path: '/team/report.pdf', op: 'link_create', status: 201, bytes_in: 0, bytes_out: 0, duration_ms: 9, instance_id: 'callfs-node-1', client: '198.51.100.7' },
	{ at: BASE, request_id: 'req-4', user_id: 'carol', method: 'POST', route: '/v1/session', path: '/v1/session', op: 'session', status: 401, bytes_in: 0, bytes_out: 0, duration_ms: 3, instance_id: 'callfs-node-1', client: '203.0.113.99' }
];

const SIGN_IN_FAILURES = [
	{ at: BASE, client: '203.0.113.99', reason: 'wrong_key', path: '/v1/session' },
	{ at: BASE, client: '203.0.113.99', reason: 'missing_key', path: '/v1/session' },
	{ at: BASE, client: '198.51.100.20', reason: 'expired_session', path: '/v1/me' }
];

const SETTING_KEYS = [
	{ path: 'server.protocol', value: 'auto', default: 'auto', source: 'file', env: 'CALLFS_SERVER_PROTOCOL', secret: false, set: true, restart: 'restart', valid: true },
	{ path: 'server.read_timeout', value: '30s', default: '60s', source: 'file', env: 'CALLFS_SERVER_READ_TIMEOUT', secret: false, set: true, restart: 'restart', valid: true },
	{ path: 'log.level', value: 'info', default: 'info', source: 'runtime', env: 'CALLFS_LOG_LEVEL', secret: false, set: true, restart: 'runtime', valid: true },
	{ path: 'ui.idle_timeout', value: '1h', default: '1h', source: 'default', env: 'CALLFS_UI_IDLE_TIMEOUT', secret: false, set: false, restart: 'runtime', valid: true },
	{ path: 'ui.session_ttl', value: '8h', default: '8h', source: 'default', env: 'CALLFS_UI_SESSION_TTL', secret: false, set: false, restart: 'runtime', valid: true },
	{ path: 'trash.enabled', value: true, default: true, source: 'file', env: 'CALLFS_TRASH_ENABLED', secret: false, set: true, restart: 'runtime', valid: true },
	{ path: 'internal_proxy.secret', value: null, default: null, source: 'file', env: 'CALLFS_INTERNAL_PROXY_SECRET', secret: true, set: true, restart: 'restart', valid: true },
	{ path: 'links.secret', value: null, default: null, source: 'env', env: 'CALLFS_LINKS_SECRET', secret: true, set: true, restart: 'restart', valid: true }
];

const CONFIG_SCHEMA = [
	{ path: 'server.protocol', type: 'enum', enum: ['https', 'http', 'auto'], min: null, max: null, default: 'auto', required_when: null, secret: false, env: 'CALLFS_SERVER_PROTOCOL', restart: 'restart', group: 'network', docs_anchor: 'protocol' },
	{ path: 'server.read_timeout', type: 'duration', enum: [], min: '1s', max: '1h', default: '60s', required_when: null, secret: false, env: 'CALLFS_SERVER_READ_TIMEOUT', restart: 'restart', group: 'network', docs_anchor: 'timeouts' },
	{ path: 'log.level', type: 'enum', enum: ['debug', 'info', 'warn', 'error'], min: null, max: null, default: 'info', required_when: null, secret: false, env: 'CALLFS_LOG_LEVEL', restart: 'runtime', group: 'logging', docs_anchor: 'logging' },
	{ path: 'internal_proxy.secret', type: 'secret', enum: [], min: null, max: null, default: null, required_when: null, secret: true, env: 'CALLFS_INTERNAL_PROXY_SECRET', restart: 'restart', group: 'security', docs_anchor: 'secrets' }
];

export const fixtures: MockTable = {
	'POST /v1/session': created({ user_id: 'alice', is_admin: true, expires_at: EXPIRES, csrf_token: 'mock-csrf' }),
	'DELETE /v1/session': { status: 204, body: null },
	'GET /v1/me': ok({ user_id: 'alice', is_admin: true, auth_method: 'session', session_expires_at: EXPIRES, csrf_token: 'mock-csrf', permissions_model: 'owner' }),
	'GET /v1/me/events': ok({ items: EVENTS.filter((event) => 'alice' === event.actor) }),
	'GET /v1/capabilities': ok({
		version: '1.9.0',
		commit: 'abc1234',
		build_date: BASE,
		go_version: 'go1.25.0',
		instance_id: 'callfs-node-1',
		node_id: 'callfs-node-1',
		features: { erasure: true, nats: true, cross_server: true, websocket: true, replication: true, quic: false, metrics_listener: true, trash: true, resumable_uploads: true, search: true, ui: true },
		storages: ['localfs', 's3'],
		default_storage: 'localfs',
		file_index: 'nats',
		lock_type: 'redis',
		external_url: 'https://files.example.test',
		limits: { max_upload_bytes: 10737418240, max_split_upload_bytes: 1073741824, max_ws_upload_bytes: 100000000, ws_frame_bytes: 1048576, max_patch_body_bytes: 1048576, link_expiry_min_s: 1, link_expiry_max_s: 86400, list_page_max: 1000, upload_part_bytes: 8388608 },
		erasure_defaults: { data_shards: 4, parity_shards: 2, max_total_shards: 64, min_file_size: 0 },
		trash_retention_s: 2592000
	}),
	'GET /health/ready': ok({ status: 'ok' }),
	'GET /v1/stat/*': ok({
		path: '/team/report.pdf',
		name: 'report.pdf',
		type: 'file',
		size: 2411724,
		mode: '0644',
		owner: 'alice',
		etag: '"etag-report.pdf"',
		storage: 'erasure',
		instance_id: 'callfs-node-1',
		is_local: true,
		erasure_coded: true,
		symlink_target: null,
		mtime: BASE,
		atime: BASE,
		ctime: BASE,
		created_at: BASE,
		updated_at: BASE,
		locked: false,
		children_count: null
	}),
	'GET /v1/directories/*': ok({ path: '/', type: 'directory', count: rootItems.length, total: rootItems.length, items: rootItems, next_cursor: null, partial: false }),
	'GET /v1/search': ok({ items: SEARCH_ITEMS, next_cursor: null, took_ms: 4, scan_truncated: false, scanned: 48210, facets: { type: { file: 3, directory: 0 }, storage: { localfs: 2, erasure: 1 }, owner: { alice: 2, bob: 1 }, instance: { 'callfs-node-1': 3 } } }),
	'GET /v1/usage': ok({
		path: '/',
		files: 48210,
		directories: 1204,
		bytes: 1200000000000,
		by_storage: { localfs: { files: 47000, bytes: 900000000000 }, s3: { files: 1210, bytes: 300000000000 } },
		by_instance: { 'callfs-node-1': { files: 16403, bytes: 400000000000 } },
		by_owner: { alice: { files: 30120, bytes: 800000000000 }, bob: { files: 14200, bytes: 300000000000 } },
		erasure_files: 12,
		erasure_raw_bytes: 6291456000,
		children: [
			{ path: '/team', files: 30000, bytes: 800000000000 },
			{ path: '/archive', files: 18210, bytes: 400000000000 }
		],
		size_histogram: [{ le: 1024, count: 12000 }, { le: 1048576, count: 30000 }, { le: 1073741824, count: 6210 }],
		age_histogram: [{ le_days: 1, count: 3000 }, { le_days: 30, count: 20000 }, { le_days: 365, count: 25210 }],
		computed_at: BASE,
		scan_truncated: false
	}),
	'GET /v1/permissions/check': ok({ path: '/team/report.pdf', perm: 'read', user: 'alice', allowed: true, reason: 'owner', owner: 'alice', type: 'file' }),
	'GET /v1/objects': ok({ count: 48210, bytes: 1200000000000, truncated: false, items: rootItems.filter((item) => 'file' === item.type), next_cursor: null }),
	'DELETE /v1/objects': ok({ deleted: 0, would_delete: 0, bytes: 0, forbidden: 0 }),
	'PATCH /v1/files/*': ok({ path: '/team/report.pdf', mode: '0640', owner: 'alice' }),
	'PUT /v1/files/*': created({ path: '/team/report.pdf', etag: '"etag-report.pdf"', created: true }),
	'DELETE /v1/files/*': ok({ trash_id: 'trash-3', expires_at: '2026-11-22T09:00:00Z', original_path: '/team/report.pdf' }),
	'POST /v1/batch': ok({ results: [{ index: 0, status: 'ok', code: null, message: null, path: '/team/report.pdf' }] }),
	'POST /v1/uploads': created({ upload_id: 'up-1', offset: 0, part_bytes: 8388608, expires_at: '2026-10-24T12:00:00Z' }),
	'GET /v1/uploads/*': ok({ upload_id: 'up-1', path: '/team/big.iso', offset: 8388608, size: 67108864, state: 'open', expires_at: '2026-10-24T12:00:00Z' }),
	'PUT /v1/uploads/*': ok({ offset: 16777216 }),
	'POST /v1/uploads/*/complete': created({ path: '/team/big.iso', etag: '"etag-big"', created: true }),
	'DELETE /v1/uploads/*': noContent,
	'GET /v1/links': ok({ items: linkItems, next_cursor: null, counts: linkCounts }),
	'GET /v1/links/stats': ok({ window: '24h', generated: 340, consumed: { success: 328, not_found: 5, expired: 4, invalid: 3 }, active: linkCounts.active, used_last_24h: 328, last_cleanup: { at: BASE, expired_removed: 4, used_removed: 2, error: null } }),
	'GET /v1/links/*': ok({ ...linkItems[0], updated_at: BASE, attempts: [{ at: BASE, result: 'success', ip: '203.0.113.10' }, { at: BASE, result: 'expired', ip: '203.0.113.11' }] }),
	'DELETE /v1/links/*': ok({ id: 1, status: 'revoked' }),
	'POST /v1/links/generate': created({ id: 24, url: 'https://files.example.test/download/mock-token', expires_at: '2026-10-24T12:00:00Z' }),
	'GET /v1/erasure/files': ok({ items: erasureItems, next_cursor: null }),
	'GET /v1/erasure/files/*': ok(erasureDetail),
	'GET /v1/erasure/manifest/*': ok({ path: '/archive/split-01.bin', original_size: 524288000, data_shards: 4, parity_shards: 2, shard_size: 131072000 }),
	'POST /v1/erasure/verify/*': ok({ path: '/archive/split-01.bin', readable: true, healthy: 5, required: 4, shards: erasureDetail.shards.map((shard) => ({ index: shard.index, state: shard.state, error: null })) }),
	'POST /v1/erasure/repair/*': accepted({ job_id: 'job-5' }),
	'GET /v1/trash': ok({ items: TRASH_ITEMS, next_cursor: null, total_bytes: 1050624 }),
	'POST /v1/trash/*/restore': ok({ path: '/team/old-notes.txt' }),
	'DELETE /v1/trash/*': noContent,
	'DELETE /v1/trash': accepted({ job_id: 'job-7' }),
	'GET /v1/jobs': ok({ items: JOBS.filter((job) => 'alice' === job.created_by), next_cursor: null }),
	'GET /v1/jobs/*': ok(JOBS[1]),
	'POST /v1/jobs/*/cancel': ok(JOBS[1]),
	'POST /v1/jobs/*/retry-failed': accepted({ job_id: 'job-4' }),
	'GET /v1/admin/status': ok({
		version: '1.9.0',
		commit: 'abc1234',
		build_date: BASE,
		go_version: 'go1.25.0',
		instance_id: 'callfs-node-1',
		node_id: 'callfs-node-1',
		started_at: BASE,
		uptime_s: 86400,
		config_path: '/etc/callfs/config.yaml',
		config_format: 'yaml',
		keys_from_env: 2,
		keys_at_default: 18,
		listeners: [{ kind: 'https', addr: '0.0.0.0:8443', tls: true, auth: true }, { kind: 'metrics', addr: '127.0.0.1:9090', tls: false, auth: false }],
		protocol_configured: 'auto',
		protocol_effective: 'https',
		file_index: 'nats',
		lock_type: 'redis',
		storages: { localfs: true, s3: true, internal_proxy: true, erasure: true },
		runtime: { goroutines: 214, heap_bytes: 134217728, open_fds: 88, cpu_seconds: 412.5 }
	}),
	'GET /v1/admin/health': ok({ status: 'warn', checked_at: BASE, components: HEALTH_COMPONENTS }),
	'GET /v1/admin/findings': ok({ items: FINDINGS, counts: { critical: 1, warning: 7, info: 1 } }),
	'POST /v1/admin/findings/*/mute': ok({ ...FINDINGS[0], muted: { until: '2026-10-23T13:00:00Z', by: 'alice', reason: 'Planned change' } }),
	'DELETE /v1/admin/findings/*/mute': ok(FINDINGS[0]),
	'GET /v1/admin/config': ok({ config_path: '/etc/callfs/config.yaml', restart_required: true, pending: ['server.read_timeout'], keys: SETTING_KEYS }),
	'GET /v1/admin/config/schema': ok({ keys: CONFIG_SCHEMA }),
	'POST /v1/admin/config/validate': ok({ valid: true, errors: [], warnings: [], restart_required: ['server.read_timeout'], applies_now: ['log.level'] }),
	'PUT /v1/admin/config': ok({ written: true, backup_path: '/etc/callfs/config.yaml.bak.20261023', revision: 'sha256:9f2c1d', restart_required: ['server.read_timeout'], applied_now: ['log.level'] }),
	'PATCH /v1/admin/config/runtime': ok({ applied: ['log.level'], persisted: true, revision: 'sha256:9f2c1d' }),
	'GET /v1/admin/config/export': ok('server:\n  protocol: auto\n  read_timeout: 30s\n'),
	'POST /v1/admin/restart': accepted({ restarting: true }),
	'GET /v1/admin/cluster': ok(CLUSTER),
	'GET /v1/admin/cluster/join-config': ok({
		yaml: 'nats:\n  enabled: true\n  server_name: callfs-node-4\n  cluster_name: callfs\n  cluster_listen: 10.0.0.14:6222\n  routes:\n    - nats://10.0.0.1:6222\n    - nats://10.0.0.14:6222\nmetadata_store:\n  type: nats\ndlm:\n  type: nats\n',
		steps: [
			'Copy auth.internal_proxy_secret, auth.single_use_link_secret and the API keys from an existing node.',
			'Start the new node with this config.',
			'Add its route and peer endpoint to the other nodes on their next restart.'
		]
	}),
	'DELETE /v1/admin/cluster/nodes/*': ok({ status: 'removed', remaining: 2 }),
	'POST /v1/admin/cluster/leadership-transfer': ok({ status: 'transferred', leader_id: 'callfs-node-2' }),
	'POST /v1/admin/cluster/snapshot': created({ id: 'snap-4', seq: 49426, bytes: 2097152, took_ms: 38 }),
	'GET /v1/admin/cluster/snapshots': ok({ retain: 3, items: [{ id: 'snap-1', seq: 49426, bytes: 2097152, created_at: BASE }, { id: 'snap-2', seq: 48000, bytes: 2090000, created_at: '2026-10-23T06:00:00Z' }, { id: 'snap-3', seq: 46500, bytes: 2080000, created_at: '2026-10-22T18:00:00Z' }] }),
	'GET /v1/admin/cluster/state-stats': ok({ files: 48210, directories: 1204, links: linkCounts, erasure_entries: 12, index_entries: 49426 }),
	'GET /v1/admin/instances': ok({ self: { instance_id: 'callfs-node-1', external_url: 'https://files.example.test', started_at: BASE }, items: INSTANCES }),
	'PUT /v1/admin/instances/*/endpoint': ok(INSTANCES[1]),
	'DELETE /v1/admin/instances/*/endpoint': ok(INSTANCES[3]),
	'POST /v1/admin/instances/*/probe': ok({ reachable: true, rtt_ms: 14, status_code: 200, error: null }),
	'GET /v1/admin/backends': ok({ default: 'localfs', items: BACKENDS }),
	'POST /v1/admin/backends/*/probe': ok({ ok: true, write_ms: 3, read_ms: 2, delete_ms: 2, error: null }),
	'GET /v1/admin/erasure': ok({
		enabled: true,
		data_shards: 4,
		parity_shards: 2,
		min_file_size: 0,
		shard_storage: 'localfs',
		shard_path: '/var/lib/callfs/pieces',
		placement: 'round_robin',
		instances: ['callfs-node-1', 'callfs-node-2', 'callfs-node-3'],
		files: 12,
		bytes: 6291456000,
		raw_bytes: 9437184000,
		health: { ok: 11, reduced: 1, critical: 0, unreadable: 0, unknown: 0 },
		pieces_by_instance: { 'callfs-node-1': 4, 'callfs-node-2': 4, 'callfs-node-3': 4 }
	}),
	'POST /v1/admin/erasure/plan': ok({ total_shards: 6, assignments: [{ index: 0, instance_id: 'callfs-node-1', kind: 'data' }, { index: 1, instance_id: 'callfs-node-2', kind: 'data' }], per_instance: { 'callfs-node-1': 2, 'callfs-node-2': 2, 'callfs-node-3': 2 }, max_on_one_instance: 2, tolerated_instance_failures: 1, tolerated_piece_losses: 2, overhead_ratio: 1.5, fault_tolerant: true, warnings: [] }),
	'POST /v1/admin/erasure/verify-all': accepted({ job_id: 'job-3' }),
	'GET /v1/admin/replication': ok({ enabled: true, replica_storage: 's3', require_success: false, counts_24h: { ok: 12401, failed: 7, skipped: 0 }, failures: [{ path: '/team/notes.txt', error: 'Connection reset.', at: BASE }, { path: '/archive/2024', error: 'Access denied.', at: BASE }] }),
	'POST /v1/admin/replication/retry': accepted({ job_id: 'job-6' }),
	'GET /v1/admin/staging': ok({ items: [{ storage: 'localfs', path: '/var/lib/callfs/.callfs-uploads/up-old', kind: 'upload_session', bytes: 8388608, modified: BASE, age_s: 90000 }, { storage: 'localfs', path: '/var/lib/callfs/tmp/part-7', kind: 'tmp', bytes: 1048576, modified: BASE, age_s: 3600 }], total_bytes: 9437184 }),
	'DELETE /v1/admin/staging': accepted({ job_id: 'job-4' }),
	'GET /v1/admin/hygiene': ok({ files_without_owner: 3, files_without_etag: 2, orphan_pieces: 0, stale_erasure_records: 0, staging: { count: 2, bytes: 9437184 }, migrations: { driver: 'postgres', version: 6, dirty: false, latest: 6 } }),
	'GET /v1/admin/migrations': ok({ driver: 'postgres', current_version: 6, dirty: false, available: [{ version: 6, name: 'ui' }] }),
	'POST /v1/admin/retag': accepted({ job_id: 'job-3' }),
	'GET /v1/admin/locks': ok({
		type: 'redis',
		wait_timeout_ms: 5000,
		ttl_ms: 30000,
		held: [
			{ key: 'callfs:lock:/team/report.pdf', path: '/team/report.pdf', owner_hint: '…a1b2', acquired_at: BASE, expires_at: '2026-10-23T12:00:30Z' },
			{ key: 'callfs:lock:/archive/2024', path: '/archive/2024', owner_hint: '…c3d4', acquired_at: BASE, expires_at: '2026-10-23T12:00:20Z' }
		],
		stats: { acquired: 2400, timeouts: 3, cancelled: 1, errors: 0, lost: 0, renew_errors: 0, wait_p50_ms: 4, wait_p95_ms: 120 }
	}),
	'DELETE /v1/admin/locks/*': ok({ released: true }),
	'GET /v1/admin/cache': ok({ enabled: true, ttl_ms: 2000, entries: 812, max_entries: 1000, hits: 9512, misses: 488, evictions: 24, expired_cleaned: 40, last_cleanup_at: BASE }),
	'POST /v1/admin/cache/invalidate': ok({ invalidated: 812 }),
	'GET /v1/admin/ratelimits': ok({
		limits: [
			{ endpoint: 'download', rps: 10, burst: 5 },
			{ endpoint: 'link_generate', rps: 100, burst: 1 },
			{ endpoint: 'session', rps: 1, burst: 5 }
		],
		tracked_clients: 812,
		max_entries: 100000,
		ttl_s: 600,
		rejections_total: { download: 14, link_generate: 0, session: 38 },
		evictions_total: 0,
		top_clients: [{ client: '203.0.113.99', endpoint: 'session', rejections: 38, last_at: BASE }]
	}),
	'GET /v1/admin/users': ok({ items: USERS, positional_count: 3 }),
	'POST /v1/admin/users': created({ user_id: 'dan', api_key: 'mock-key-shown-once' }),
	'POST /v1/admin/users/*/rotate': ok({ user_id: 'bob', api_key: 'mock-rotated-key-shown-once' }),
	'PATCH /v1/admin/users/*': ok(USERS[1]),
	'DELETE /v1/admin/users/*': noContent,
	'POST /v1/admin/users/migrate-positional': accepted({ job_id: 'job-1' }),
	'GET /v1/admin/owners': ok({ items: [{ owner: '', files: 3, bytes: 4096 }, { owner: 'alice', files: 30120, bytes: 800000000000 }, { owner: 'bob', files: 14200, bytes: 300000000000 }], next_cursor: null }),
	'POST /v1/admin/owners/reassign': accepted({ job_id: 'job-2' }),
	'GET /v1/admin/auth/failures': ok({ total_24h: 38, items: SIGN_IN_FAILURES }),
	'GET /v1/admin/tls': ok({
		protocol_configured: 'auto',
		protocol_effective: 'https',
		fallback: false,
		cert: { path: '/etc/callfs/tls/cert.pem', subject: 'files.example.test', issuer: 'Example CA', sans: ['files.example.test'], not_before: '2026-10-01T00:00:00Z', not_after: '2027-01-12T00:00:00Z', days_remaining: 81 },
		key: { path: '/etc/callfs/tls/key.pem', mode: '0600', world_readable: false },
		quic: { enabled: false, addr: '' },
		hsts: true,
		headers: { csp: "default-src 'self'", frame_options: 'DENY', coop: 'same-origin', coep: 'require-corp' }
	}),
	'GET /v1/admin/metrics': ok({
		generated_at: BASE,
		families: METRIC_FAMILIES.map(([name, type, help]) => ({ name, type, help, emitted: 'callfs_usage_files' !== name, series: [{ labels: {}, value: 1 }] }))
	}),
	'GET /v1/admin/metrics/series': ok({
		name: 'callfs_http_requests_total',
		measure: 'rate',
		unit: 'requests_per_second',
		step_s: STEP_S,
		series: [
			{ labels: { method: 'GET', path: '/v1/directories/*' }, points: seriesPoints(3) },
			{ labels: { method: 'POST', path: '/v1/links/generate' }, points: seriesPoints(11) }
		],
		sampled_since: BASE
	}),
	'GET /v1/admin/logs': ok({ items: LOG_ITEMS, next_cursor: null }),
	'GET /v1/admin/logs/stream': ok({ items: LOG_ITEMS }),
	'GET /v1/admin/audit': ok({ items: AUDIT_ITEMS, next_cursor: null }),
	'GET /v1/admin/events': ok({ items: EVENTS }),
	'GET /v1/admin/events/history': ok({ items: EVENTS }),
	'GET /v1/admin/transfers': ok({ items: [{ id: 'tr-1', user_id: 'alice', kind: 'http_upload', path: '/team/big.iso', bytes_done: 33554432, bytes_total: 67108864, started_at: BASE, client: '198.51.100.7' }, { id: 'tr-2', user_id: 'bob', kind: 'ws_download', path: '/archive/2024', bytes_done: 1048576, bytes_total: null, started_at: BASE, client: '198.51.100.8' }] }),
	'GET /v1/admin/jobs': ok({ items: JOBS, next_cursor: null }),
	'GET /v1/admin/jobs/*': ok(JOBS[1]),
	'POST /v1/admin/jobs/*/cancel': ok(JOBS[1]),
	'POST /v1/admin/jobs/*/retry-failed': accepted({ job_id: 'job-4' }),
	'GET /v1/admin/workers': ok({ items: WORKERS }),
	'POST /v1/admin/workers/*/run': accepted({ job_id: 'job-8' })
};
