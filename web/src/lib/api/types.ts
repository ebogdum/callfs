export type Result<T, E> = { ok: true; value: T } | { ok: false; error: E };

export interface Me {
	user_id: string;
	is_admin: boolean;
	auth_method: 'api_key' | 'session';
	session_expires_at: string | null;
	csrf_token: string | null;
	permissions_model: 'owner';
	idle_timeout_seconds: number;
}

export interface Capabilities {
	version: string;
	commit: string;
	build_date: string;
	go_version: string;
	instance_id: string;
	node_id: string | null;
	features: {
		erasure: boolean;
		nats: boolean;
		cross_server: boolean;
		websocket: boolean;
		replication: boolean;
		quic: boolean;
		metrics_listener: boolean;
		trash: boolean;
		resumable_uploads: boolean;
		search: boolean;
		ui: boolean;
	};
	storages: string[];
	default_storage: string;
	file_index: string;
	lock_type: string;
	external_url: string;
	limits: {
		max_upload_bytes: number;
		max_split_upload_bytes: number;
		max_ws_upload_bytes: number;
		ws_frame_bytes: number;
		max_patch_body_bytes: number;
		link_expiry_min_s: number;
		link_expiry_max_s: number;
		list_page_max: number;
		upload_part_bytes: number;
	};
	erasure_defaults: {
		data_shards: number;
		parity_shards: number;
		max_total_shards: number;
		min_file_size: number;
	};
	trash_retention_s: number;
}

export interface Stat {
	path: string;
	name: string;
	type: 'file' | 'directory';
	size: number;
	mode: string;
	owner: string;
	etag: string;
	storage: 'localfs' | 's3' | 'erasure';
	instance_id: string;
	is_local: boolean;
	erasure_coded: boolean;
	symlink_target: string | null;
	mtime: string;
	atime: string;
	ctime: string;
	created_at: string;
	updated_at: string;
	locked: boolean;
	children_count: number | null;
}

export interface FileEntry {
	name: string;
	path: string;
	type: 'file' | 'directory';
	size: number;
	mode: string;
	owner: string;
	mtime: string;
	etag: string;
	storage: 'localfs' | 's3' | 'erasure';
	instance_id: string;
	erasure_coded: boolean;
	locked: boolean;
}

export interface DirPage {
	path: string;
	type: 'directory';
	count: number;
	total: number | null;
	items: FileEntry[];
	next_cursor: string | null;
	partial: boolean;
}

export interface SearchQuery {
	q?: string;
	path?: string;
	type?: 'file' | 'directory';
	min_size?: number;
	max_size?: number;
	modified_after?: string;
	modified_before?: string;
	owner?: string;
	storage?: string;
	instance?: string;
	erasure?: boolean;
	locked?: boolean;
	limit?: number;
	cursor?: string;
	facets?: boolean;
}

export interface SearchResult {
	items: FileEntry[];
	next_cursor: string | null;
	took_ms: number;
	scan_truncated: boolean;
	scanned: number;
	facets: {
		type: Record<string, number>;
		storage: Record<string, number>;
		owner: Record<string, number>;
		instance: Record<string, number>;
	};
}

export interface UsageBucket {
	files: number;
	bytes: number;
}

export interface Usage {
	path: string;
	files: number;
	directories: number;
	bytes: number;
	by_storage: Record<string, UsageBucket>;
	by_instance: Record<string, UsageBucket>;
	by_owner: Record<string, UsageBucket>;
	erasure_files: number;
	erasure_raw_bytes: number;
	children: { path: string; files: number; bytes: number }[];
	size_histogram: { le: number; count: number }[];
	age_histogram: { le_days: number; count: number }[];
	computed_at: string;
	scan_truncated: boolean;
}

export interface PermissionDecision {
	path: string;
	perm: 'read' | 'write' | 'delete';
	user: string;
	allowed: boolean;
	reason: 'owner' | 'admin' | 'open_directory' | 'not_owner' | 'not_found' | 'parent_open';
	owner: string;
	type: 'file' | 'directory';
}

export interface ObjectPage {
	count: number;
	bytes: number;
	truncated: boolean;
}

export interface DeletePlan {
	files: number;
	directories: number;
	bytes: number;
	erasure_files: number;
	remote_instances: string[];
}

export interface BatchOp {
	op: 'delete' | 'trash' | 'move' | 'rename' | 'mode' | 'restore';
	path: string;
	destination?: string;
	name?: string;
	mode?: string;
	recursive?: boolean;
	overwrite?: boolean;
	create_parents?: boolean;
	storage?: string;
	instance?: string;
}

export interface BatchResult {
	results?: { index: number; status: number; code: string | null; message: string | null; path: string }[];
	job_id?: string;
}

export interface Link {
	id: number;
	token_prefix: string;
	file_path: string;
	status: 'active' | 'used' | 'expired' | 'revoked';
	created_by: string;
	created_at: string;
	expires_at: string;
	used_at: string | null;
	used_by_ip: string | null;
	updated_at?: string;
	attempts?: { at: string; result: 'success' | 'expired' | 'invalid' | 'not_found'; ip: string }[];
}

export interface LinkPage {
	items: Link[];
	next_cursor: string | null;
	counts: { active: number; used: number; expired: number; revoked: number };
}

export interface LinkStats {
	window: '1h' | '24h' | '7d';
	generated: number;
	consumed: { success: number; not_found: number; expired: number; invalid: number };
	active: number;
	used_last_24h: number;
	last_cleanup: { at: string | null; expired_removed: number; used_removed: number; error: string | null };
}

export interface SplitFile {
	path: string;
	original_size: number;
	data_shards: number;
	parity_shards: number;
	shard_size: number;
	shards_total: number;
	healthy_shards: number | null;
	last_checked_at: string | null;
	health: 'ok' | 'reduced' | 'critical' | 'unreadable' | 'unknown';
}

export interface SplitFileDetail {
	path: string;
	original_size: number;
	profile: { data_shards: number; parity_shards: number; shard_size: number };
	shards: {
		index: number;
		kind: 'data' | 'parity';
		instance_id: string;
		storage: string;
		path: string;
		size: number;
		checksum: string;
		state: 'ok' | 'missing' | 'corrupt' | 'unreachable' | 'unknown';
	}[];
	tolerance: number;
	last_checked_at: string | null;
}

export interface VerifyReport {
	path: string;
	readable: boolean;
	healthy: number;
	required: number;
	shards: { index: number; state: string; error: string | null }[];
}

export interface TrashItem {
	id: string;
	original_path: string;
	name: string;
	type: 'file' | 'directory';
	size: number;
	deleted_by: string;
	deleted_at: string;
	expires_at: string;
}

export interface UploadSession {
	upload_id: string;
	path: string;
	offset: number;
	size: number;
	state: 'open' | 'completing' | 'done' | 'failed' | 'expired';
	expires_at: string;
}

export interface Job {
	id: string;
	kind:
		| 'delete_prefix'
		| 'delete_recursive'
		| 'batch'
		| 'erasure_repair'
		| 'erasure_verify_all'
		| 'replication_retry'
		| 'owner_reassign'
		| 'staging_cleanup'
		| 'usage_scan'
		| 'trash_empty'
		| 'link_cleanup'
		| 'retag';
	state: 'queued' | 'running' | 'succeeded' | 'partial' | 'failed' | 'cancelled' | 'cancelling';
	phase: 'copy' | 'verify' | 'delete' | 'scan' | null;
	progress: {
		done: number;
		total: number | null;
		bytes_done: number;
		bytes_total: number | null;
		per_second: number;
	};
	created_by: string;
	created_at: string;
	started_at: string | null;
	finished_at: string | null;
	error: string | null;
	failed_items: { path: string; code: string; message: string }[];
	failed_count: number;
	result: Record<string, unknown>;
}

export interface Event {
	id: string;
	at: string;
	type: string;
	level: 'info' | 'warn' | 'error';
	actor: string | null;
	instance_id: string;
	message: string;
	entities: { kind: 'file' | 'folder' | 'link' | 'member' | 'server' | 'job' | 'setting'; id: string }[];
	fields: Record<string, unknown>;
}

export interface AdminStatus {
	version: string;
	commit: string;
	build_date: string;
	go_version: string;
	instance_id: string;
	node_id: string | null;
	started_at: string;
	uptime_s: number;
	config_path: string;
	config_format: string;
	keys_from_env: string[];
	keys_at_default: string[];
	listeners: { kind: 'https' | 'http' | 'quic' | 'metrics'; addr: string; tls: boolean; auth: boolean }[];
	protocol_configured: string;
	protocol_effective: string;
	file_index: string;
	lock_type: string;
	storages: { localfs: boolean; s3: boolean; internal_proxy: boolean; erasure: boolean };
	runtime: { goroutines: number; heap_bytes: number; open_fds: number | null; cpu_seconds: number };
}

export interface Health {
	status: 'ok' | 'warn' | 'down';
	checked_at: string;
	components: {
		name: string;
		status: 'ok' | 'warn' | 'down' | 'disabled';
		latency_ms: number | null;
		detail: string;
		since: string | null;
	}[];
}

export interface Finding {
	id: string;
	severity: 'critical' | 'warning' | 'info';
	area: 'security' | 'storage' | 'cluster' | 'config' | 'runtime';
	title: string;
	body: string;
	affected: { kind: string; id: string; href: string }[];
	since: string;
	fix: { label: string; route: string; setting_key: string | null; command: string | null };
	muted: { until: string; by: string; reason: string } | null;
	ignored?: { at: string; by: string; reason: string } | null;
}

export interface ConfigView {
	config_path: string;
	restart_required: boolean;
	pending: string[];
	keys: {
		path: string;
		value: unknown | null;
		default: unknown;
		source: 'default' | 'file' | 'env' | 'runtime';
		env: string;
		secret: boolean;
		set: boolean;
		restart: 'restart' | 'runtime' | 'auth_reload';
		valid: boolean;
	}[];
}

export interface ConfigSchema {
	keys: {
		path: string;
		type:
			| 'string'
			| 'bool'
			| 'int'
			| 'float'
			| 'duration'
			| 'enum'
			| 'path'
			| 'url'
			| 'hostport'
			| 'map'
			| 'list'
			| 'secret';
		enum: string[];
		min: number | null;
		max: number | null;
		default: unknown;
		required_when: string | null;
		secret: boolean;
		env: string;
		restart: 'restart' | 'runtime' | 'auth_reload';
		group: string;
		docs_anchor: string;
	}[];
}

export interface ValidateResult {
	valid: boolean;
	errors: { path: string; message: string }[];
	warnings: { path: string; message: string }[];
	restart_required: string[];
	applies_now: string[];
}

export interface ClusterMember {
	node_id: string;
	instance_id: string | null;
	api_endpoint: string | null;
	state: 'leader' | 'follower' | 'offline';
	is_self: boolean;
	online: boolean;
	last_seen_ms: number | null;
	behind_by: number | null;
}

export interface Snapshot {
	id: string;
	seq: number;
	bytes: number;
	created_at: string;
}

export interface ClusterLog {
	first_seq: number;
	last_seq: number;
	messages: number;
	bytes: number;
	applied_seq: number;
	last_snapshot: Snapshot | null;
	snapshot_interval_ms: number;
}

export interface ClusterInfo {
	mode: 'nats' | 'single';
	self_id: string;
	cluster_name: string;
	leader_id: string | null;
	leader_endpoint: string | null;
	replicas: number;
	has_quorum: boolean;
	can_lose: number;
	log: ClusterLog;
	members: ClusterMember[];
}

export interface StateStats {
	files: number;
	directories: number;
	links: { active: number; used: number; expired: number; revoked: number };
	erasure_entries: number;
	index_entries: number;
}

export interface ServerInfo {
	instance_id: string;
	endpoint: string;
	source: 'config' | 'joined' | 'self';
	is_self: boolean;
	scheme: 'https' | 'http';
	reachable: boolean | null;
	rtt_ms: number | null;
	checked_at: string | null;
	files: number;
	bytes: number;
	pieces: number;
	disk_free_bytes: number | null;
	disk_total_bytes: number | null;
}

export interface Backend {
	name: 'localfs' | 's3' | 'internal_proxy' | 'erasure';
	enabled: boolean;
	status: 'ok' | 'warn' | 'down' | 'disabled';
	root_path: string | null;
	free_bytes: number | null;
	total_bytes: number | null;
	used_bytes: number | null;
	bucket: string | null;
	region: string | null;
	endpoint: string | null;
	encryption: string | null;
	acl: string | null;
	kms_key_set: boolean;
	peers: number;
	last_probe: { at: string; ok: boolean; latency_ms: number; error: string | null } | null;
}

export interface ErasureSummary {
	enabled: boolean;
	data_shards: number;
	parity_shards: number;
	min_file_size: number;
	shard_storage: string;
	shard_path: string;
	placement: 'round_robin';
	instances: string[];
	files: number;
	bytes: number;
	raw_bytes: number;
	health: { ok: number; reduced: number; critical: number; unreadable: number; unknown: number };
	pieces_by_instance: Record<string, number>;
}

export interface PlacementPlan {
	total_shards: number;
	assignments: { index: number; instance_id: string; kind: 'data' | 'parity' }[];
	per_instance: Record<string, number>;
	max_on_one_instance: number;
	tolerated_instance_failures: number;
	tolerated_piece_losses: number;
	overhead_ratio: number;
	fault_tolerant: boolean;
	warnings: string[];
}

export interface Replication {
	enabled: boolean;
	replica_storage: string;
	require_success: boolean;
	counts_24h: { ok: number; failed: number; skipped: number };
	failures: { path: string; error: string; at: string }[];
}

export interface StagingItem {
	storage: string;
	path: string;
	kind: 'staging' | 'tmp' | 'upload_session' | 'probe';
	bytes: number;
	modified: string;
	age_s: number;
}

export interface Hygiene {
	files_without_owner: number;
	files_without_etag: number;
	orphan_pieces: number | null;
	stale_erasure_records: number | null;
	staging: { count: number; bytes: number };
	migrations: { driver: string; version: number | null; dirty: boolean; latest: number | null };
}

export interface Migrations {
	driver: 'postgres' | 'none';
	current_version: number | null;
	dirty: boolean;
	available: { version: number; name: string }[];
}

export interface Locks {
	type: 'local' | 'redis' | 'nats';
	wait_timeout_ms: number;
	ttl_ms: number;
	held: { key: string; path: string; owner_hint: string; acquired_at: string; expires_at: string }[];
	stats: {
		acquired: number;
		timeouts: number;
		cancelled: number;
		errors: number;
		lost: number;
		renew_errors: number;
		wait_p50_ms: number | null;
		wait_p95_ms: number | null;
	};
}

export interface CacheStats {
	enabled: boolean;
	ttl_ms: number;
	entries: number;
	max_entries: number;
	hits: number;
	misses: number;
	evictions: number;
	expired_cleaned: number;
	last_cleanup_at: string | null;
}

export interface RateLimits {
	limits: { endpoint: 'download' | 'link_generate' | 'session'; rps: number; burst: number }[];
	tracked_clients: number;
	max_entries: number;
	ttl_s: number;
	rejections_total: { download: number; link_generate: number; session: number };
	evictions_total: number;
	top_clients: { client: string; endpoint: string; rejections: number; last_at: string }[];
}

export interface UserInfo {
	user_id: string;
	source: 'api_key_users' | 'api_keys' | 'reserved';
	index: number | null;
	key_hint: string;
	key_fingerprint: string;
	is_admin: boolean;
	owned_files: number;
	owned_bytes: number;
	last_auth_at: string | null;
	requests_24h: number;
	auth_failures_24h: number;
}

export interface OwnerStat {
	owner: string;
	files: number;
	bytes: number;
}

export interface AuthFailure {
	at: string;
	client: string;
	reason: 'missing_key' | 'wrong_key' | 'expired_session' | 'csrf';
	path: string;
}

export interface TlsInfo {
	protocol_configured: string;
	protocol_effective: string;
	fallback: boolean;
	cert: {
		path: string;
		subject: string;
		issuer: string;
		sans: string[];
		not_before: string;
		not_after: string;
		days_remaining: number;
	} | null;
	key: { path: string; mode: string; world_readable: boolean } | null;
	quic: { enabled: boolean; addr: string };
	hsts: boolean;
	headers: { csp: string; frame_options: string; coop: string; coep: string };
}

export interface MetricFamily {
	name: string;
	type: string;
	help: string;
	emitted: boolean;
	series: {
		labels: Record<string, string>;
		value?: number;
		count?: number;
		sum?: number;
		buckets?: { le: number; count: number }[];
	}[];
}

export interface SeriesResult {
	name: string;
	measure: 'rate' | 'total' | 'avg' | 'p50' | 'p95' | 'p99' | 'max' | 'value';
	unit: string;
	step_s: number;
	series: { labels: Record<string, string>; points: [number, number | null][] }[];
	sampled_since: string | null;
}

export interface LogEntry {
	id: string;
	at: string;
	level: 'debug' | 'info' | 'warn' | 'error';
	message: string;
	logger: string;
	fields: Record<string, unknown>;
	request_id: string | null;
}

export interface AuditEntry {
	at: string;
	request_id: string | null;
	user_id: string | null;
	method: string;
	route: string;
	path: string;
	op: 'read' | 'list' | 'stat' | 'upload' | 'replace' | 'rename' | 'move' | 'delete' | 'trash' | 'restore' | 'link_create' | 'link_open' | 'session' | 'admin';
	status: number;
	bytes_in: number;
	bytes_out: number;
	duration_ms: number;
	instance_id: string;
	client: string;
}

export interface Transfer {
	id: string;
	user_id: string;
	kind: string;
	path: string;
	bytes_done: number;
	bytes_total: number | null;
	started_at: string;
	client: string;
}

export interface Worker {
	name: 'link_cleanup' | 'trash_cleanup' | 'upload_cleanup' | 'instance_prober' | 'metrics_sampler' | 'usage_refresh';
	interval_s: number;
	running: boolean;
	last_run_at: string | null;
	last_duration_ms: number | null;
	last_result: Record<string, unknown>;
	last_error: string | null;
	next_run_at: string | null;
}

export interface DeadTask {
	id: number;
	pool: string;
	reason: string;
	failed_at: string;
	summary: string;
}
