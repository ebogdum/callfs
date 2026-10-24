import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getConfig, patchRuntime, save, schema, validate } from '$lib/api/admin/config';
import type { ConfigView, ConfigSchema } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { invalidateAll } from '$lib/data/cache';
import { toasts } from '$lib/stores/toasts.svelte';
import { SETTINGS_MAP, entriesFor, entryFor, riskApplies, type SettingsEntry } from './settings-map';
import { SettingsDraft, describeValue, metaFor, sameValue, type SettingsSnapshot } from './settings-draft.svelte';
import SettingsGroupPlanet from './SettingsGroupPlanet.svelte';

vi.mock('$lib/api/admin/config', () => ({
	getConfig: vi.fn(),
	schema: vi.fn(),
	validate: vi.fn(),
	save: vi.fn(),
	patchRuntime: vi.fn()
}));

vi.mock('$lib/api/admin/status', () => ({
	restart: vi.fn()
}));

vi.mock('$lib/stores/toasts.svelte', () => ({
	toasts: { push: vi.fn() }
}));

const EXPECTED_KEYS = [
	'server.listen_addr', 'server.protocol', 'server.external_url', 'server.enable_quic', 'server.quic_listen_addr', 'metrics.listen_addr', 'metrics.profiling', 'performance.mode', 'backend.localfs_sync', 'backend.localfs_ack', 'backend.localfs_true_overwrite', 'backend.localfs_tmp_save_max_size', 'backend.flush_workers', 'backend.flush_max_pending_bytes', 'backend.flush_max_delay',
	'server.read_timeout', 'server.write_timeout', 'server.file_op_timeout', 'server.metadata_op_timeout', 'server.metadata_cache_ttl',
	'dlm.wait_timeout', 'server.rate_limit.download_rps', 'server.rate_limit.download_burst', 'server.rate_limit.link_generate_rps',
	'server.rate_limit.link_generate_burst', 'server.rate_limit.session_rps', 'server.rate_limit.session_burst', 'ui.session_ttl', 'ui.idle_timeout',
	'server.cert_file', 'server.key_file', 'backend.internal_proxy_ca_file', 'backend.internal_proxy_skip_tls_verify', 'auth.internal_proxy_secret', 'auth.single_use_link_secret', 'log_mode',
	'backend.default_backend', 'backend.localfs_root_path', 'backend.s3_bucket_name', 'backend.s3_region', 'backend.s3_endpoint',
	'backend.s3_access_key', 'backend.s3_secret_key', 'backend.s3_server_side_encryption', 'backend.s3_kms_key_id', 'backend.s3_acl',
	'erasure.enabled', 'erasure.data_shards', 'erasure.parity_shards', 'erasure.min_file_size', 'erasure.shard_backend', 'erasure.shard_path',
	'ha.replication_enabled', 'ha.replica_backend', 'ha.require_replica_success', 'trash.enabled', 'trash.retention',
	'metadata_store.type', 'metadata_store.dsn', 'metadata_store.sqlite_path', 'metadata_store.redis_addr', 'metadata_store.redis_password',
	'metadata_store.redis_db', 'metadata_store.redis_key_prefix', 'instance_discovery.instance_id', 'instance_discovery.peer_endpoints',
	'dlm.type', 'dlm.redis_addr', 'dlm.redis_password', 'log.level', 'log.format', 'ui.log_buffer_entries', 'ui.audit_buffer_entries',
	'ui.event_buffer_entries', 'ui.metrics_sample_interval', 'ui.metrics_retention', 'ui.enabled', 'ui.allow_remote_restart',
	'nats.metadata.replicas', 'nats.metadata.apply_timeout', 'nats.metadata.read_barrier', 'nats.metadata.miss_barrier', 'nats.metadata.max_msg_size', 'nats.metadata.snapshot_interval', 'nats.metadata.snapshot_every', 'nats.metadata.retain_commands', 'nats.metadata.retain_snapshots', 'nats.enabled', 'nats.mode', 'nats.server_name', 'nats.cluster_name', 'nats.server_tags', 'nats.client_listen', 'nats.cluster_listen', 'nats.routes', 'nats.routes_from_peers', 'nats.monitor_listen', 'nats.tls.cert_file', 'nats.tls.key_file', 'nats.tls.ca_file', 'nats.tls.client_cert_file', 'nats.tls.client_key_file', 'nats.auth.mode', 'nats.auth.nkey_seed_file', 'nats.auth.creds_file', 'nats.auth.previous_secret', 'nats.jetstream.store_dir', 'nats.jetstream.max_memory', 'nats.jetstream.max_store', 'nats.jetstream.sync', 'nats.jetstream.sync_interval', 'nats.jetstream.unique_tag', 'nats.external.urls', 'nats.external.js_domain', 'nats.external.tls_ca_file', 'nats.locks.replicas', 'nats.locks.ttl', 'nats.locks.queue_stale', 'nats.streams.replicas', 'nats.streams.events_max_age', 'nats.streams.events_max_bytes', 'nats.streams.audit_max_age', 'nats.streams.logs_level', 'nats.streams.logs_max_age', 'nats.streams.tasks_max_deliver', 'nats.streams.tasks_backoff', 'nats.streams.dlq_max_age', 'nats.rpc_timeout', 'nats.bytes', 'nats.counters',
	'auth.api_key_users', 'auth.api_keys', 'auth.admin_users'
];

const RESTART_KEYS = ['server.listen_addr', 'nats.enabled', 'metadata_store.type'];
const RUNTIME_KEYS = ['log.level', 'trash.enabled', 'server.metadata_cache_ttl'];

function configView(pending: string[] = []): ConfigView {
	return {
		config_path: '/etc/callfs/config.yaml',
		restart_required: 0 < pending.length,
		pending,
		keys: [
			{ path: 'server.read_timeout', value: '30s', default: '30s', source: 'file', env: 'CALLFS_SERVER__READ_TIMEOUT', secret: false, set: false, restart: 'restart', valid: true },
			{ path: 'server.listen_addr', value: ':8443', default: ':8443', source: 'env', env: 'CALLFS_SERVER__LISTEN_ADDR', secret: false, set: false, restart: 'restart', valid: true },
			{ path: 'log.level', value: 'info', default: 'info', source: 'default', env: 'CALLFS_LOG__LEVEL', secret: false, set: false, restart: 'runtime', valid: true },
			{ path: 'auth.internal_proxy_secret', value: null, default: null, source: 'file', env: 'CALLFS_AUTH__INTERNAL_PROXY_SECRET', secret: true, set: true, restart: 'restart', valid: true }
		]
	};
}

function snapshotOf(pending: string[] = []): SettingsSnapshot {
	const shape: ConfigSchema = { keys: [] };
	return { view: configView(pending), schema: shape };
}

function entryOf(key: string): SettingsEntry {
	return entryFor(key) as SettingsEntry;
}

describe('settings map', () => {
	it('maps all config keys and env rows exactly once', () => {
		const keys = SETTINGS_MAP.map((entry) => entry.key);
		expect(keys).toHaveLength(133);
		expect(new Set(keys).size).toBe(133);
		expect([...keys].sort()).toEqual([...EXPECTED_KEYS].sort());
	});

	it('gives every row label and effect copy that resolves', () => {
		for (const entry of SETTINGS_MAP) {
			expect(t(entry.labelKey)).not.toBe(entry.labelKey);
			expect(t(entry.effectKey)).not.toBe(entry.effectKey);
		}
	});

	it('resolves note and risk copy for every flagged row', () => {
		for (const entry of SETTINGS_MAP) {
			if (true === entry.hasNote) {
				expect(t(`settings.note.${entry.key}`)).not.toBe(`settings.note.${entry.key}`);
			}
			if (undefined !== entry.riskKey) {
				expect(t(entry.riskKey)).not.toBe(entry.riskKey);
			}
		}
	});

	it('gives every enum option a label that resolves', () => {
		for (const entry of SETTINGS_MAP) {
			for (const option of entry.options ?? []) {
				const key = `settings.option.${entry.key}.${option}`;
				expect(t(key)).not.toBe(key);
			}
		}
	});

	it('splits rows across the seven groups and the access page', () => {
		const groups = ['network', 'limits', 'security', 'storage', 'index', 'coordination', 'logging'] as const;
		const grouped = groups.flatMap((group) => entriesFor(group).map((entry) => entry.key));
		expect(grouped).toHaveLength(130);
		expect(entriesFor('network').map((entry) => entry.key)).toContain('metrics.listen_addr');
	});

	it('has copy for every zero label row', () => {
		for (const entry of SETTINGS_MAP.filter((item) => item.zeroLabel)) {
			expect(t(`settings.zero.${entry.key}`)).not.toBe(`settings.zero.${entry.key}`);
		}
	});
});

describe('row metadata', () => {
	it('shows restart badges for restart keys and applies-now badges for runtime keys', () => {
		for (const key of RESTART_KEYS) {
			expect(metaFor(entryOf(key), undefined, null, null, null).restart).toBe('restart');
		}
		for (const key of RUNTIME_KEYS) {
			expect(metaFor(entryOf(key), undefined, null, null, null).restart).toBe('runtime');
		}
	});

	it('marks environment-sourced rows as env and keeps the env name on the key', () => {
		const view = configView();
		const state = view.keys.find((item) => 'server.listen_addr' === item.path);
		const meta = metaFor(entryOf('server.listen_addr'), state, ':8443', null, null);
		expect(meta.source).toBe('env');
		expect(meta.key).toBe('server.listen_addr');
	});

	it('describes values in words', () => {
		expect(describeValue(entryOf('log.level'), 'debug', false)).toBe(t('settings.option.log.level.debug'));
		expect(describeValue(entryOf('server.metadata_cache_ttl'), '0s', false)).toBe('Off');
		expect(describeValue(entryOf('auth.internal_proxy_secret'), null, true)).toBe('Set');
		expect(describeValue(entryOf('auth.internal_proxy_secret'), null, false)).toBe('Not set');
		expect(describeValue(entryOf('ha.replication_enabled'), true, false)).toBe('On');
		expect(describeValue(entryOf('instance_discovery.peer_endpoints'), { a: 'x', b: 'y' }, false)).toBe(t('settings.value.count', { count: 2 }));
	});

	it('applies risk rules to the current value', () => {
		expect(riskApplies(entryOf('server.listen_addr'), ':8443', true, false)).toBe(true);
		expect(riskApplies(entryOf('server.listen_addr'), '127.0.0.1:8443', true, false)).toBe(false);
		expect(riskApplies(entryOf('metrics.listen_addr'), ':9090', true, false)).toBe(true);
		expect(riskApplies(entryOf('metrics.listen_addr'), '127.0.0.1:9090', true, false)).toBe(false);
		expect(riskApplies(entryOf('backend.internal_proxy_skip_tls_verify'), true, true, false)).toBe(true);
		expect(riskApplies(entryOf('metadata_store.type'), 'sqlite', true, true)).toBe(true);
		expect(riskApplies(entryOf('metadata_store.type'), 'sqlite', true, false)).toBe(false);
		expect(sameValue({ a: 1 }, { a: 1 })).toBe(true);
	});
});

describe('settings draft', () => {
	beforeEach(() => {
		vi.mocked(validate).mockReset();
		vi.mocked(save).mockReset();
		vi.mocked(patchRuntime).mockReset();
		vi.mocked(toasts.push).mockReset();
	});

	it('keeps environment-locked rows read-only', async () => {
		const draft = new SettingsDraft();
		draft.load(snapshotOf());
		expect(draft.isLocked('server.listen_addr')).toBe(true);
		await draft.setValue(entryOf('server.listen_addr'), ':9000');
		expect(draft.dirtyKeys).toEqual([]);
		expect(patchRuntime).not.toHaveBeenCalled();
		expect(save).not.toHaveBeenCalled();
	});

	it('stages a transactional change, validates it and saves it with the diff inputs', async () => {
		const draft = new SettingsDraft();
		draft.load(snapshotOf());
		const entry = entryOf('server.read_timeout');
		await draft.setValue(entry, '2m');
		expect(draft.dirtyKeys).toEqual(['server.read_timeout']);
		expect(draft.baseline['server.read_timeout']).toBe('30s');
		expect(draft.edits['server.read_timeout']).toBe('2m');

		vi.mocked(validate).mockResolvedValue({
			ok: true,
			value: { valid: true, errors: [], warnings: [], restart_required: ['server.read_timeout'], applies_now: [] }
		});
		const checked = await draft.validate();
		expect(checked.ok).toBe(true);
		expect(validate).toHaveBeenCalledWith({ 'server.read_timeout': '2m' });

		vi.mocked(save).mockResolvedValue({
			ok: true,
			value: { written: true, backup_path: 'config.yaml.bak.1', revision: 'sha256:b', restart_required: ['server.read_timeout'], applied_now: [] }
		});
		const saved = await draft.save();
		expect(saved.ok).toBe(true);
		expect(save).toHaveBeenCalledWith({ 'server.read_timeout': '2m' }, '');
		expect(draft.baseline['server.read_timeout']).toBe('2m');
		expect(draft.pending).toEqual(['server.read_timeout']);
		expect(draft.dirtyKeys).toEqual([]);
		expect(draft.revision).toBe('sha256:b');
	});

	it('keeps the draft and reports the error when save fails', async () => {
		const draft = new SettingsDraft();
		draft.load(snapshotOf());
		await draft.setValue(entryOf('server.read_timeout'), '2m');
		vi.mocked(save).mockResolvedValue({
			ok: false,
			error: { title: 'Some values need fixing.', body: 'first error', retryable: false }
		});
		const outcome = await draft.save();
		expect(outcome.ok).toBe(false);
		expect(draft.dirtyKeys).toEqual(['server.read_timeout']);
		expect(draft.saveError?.title).toBe('Some values need fixing.');
	});

	it('autosaves runtime keys, announces them and undoes the change', async () => {
		const draft = new SettingsDraft();
		draft.load(snapshotOf());
		vi.mocked(patchRuntime).mockResolvedValue({ ok: true, value: { applied: ['log.level'], persisted: true, revision: 'sha256:c' } });
		await draft.setValue(entryOf('log.level'), 'debug');
		expect(patchRuntime).toHaveBeenCalledWith({ 'log.level': 'debug' });
		expect(draft.baseline['log.level']).toBe('debug');
		expect(draft.applied).toEqual(['log.level']);
		expect(draft.dirtyKeys).toEqual([]);

		const pushed = vi.mocked(toasts.push).mock.calls[0]?.[0];
		expect(pushed?.tone).toBe('ok');
		vi.mocked(patchRuntime).mockResolvedValue({ ok: true, value: { applied: ['log.level'], persisted: true, revision: 'sha256:d' } });
		pushed?.action?.run();
		await vi.waitFor(() => expect(patchRuntime).toHaveBeenLastCalledWith({ 'log.level': 'info' }));
	});

	it('discards staged edits for the given keys only', async () => {
		const draft = new SettingsDraft();
		draft.load(snapshotOf());
		await draft.setValue(entryOf('server.read_timeout'), '2m');
		draft.stage(entryOf('nats.enabled'), true);
		draft.discard(['server.read_timeout']);
		expect(draft.dirtyKeys).toEqual(['nats.enabled']);
	});
});

describe('SettingsGroupPlanet', () => {
	beforeEach(() => {
		invalidateAll();
		vi.mocked(getConfig).mockReset();
		vi.mocked(schema).mockReset();
		vi.mocked(getConfig).mockResolvedValue({ ok: true, value: configView(['server.read_timeout']) });
		vi.mocked(schema).mockResolvedValue({ ok: true, value: { keys: [] } });
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the group heading and the pending restart banner', async () => {
		render(SettingsGroupPlanet, { props: { group: 'limits' } });
		expect(await screen.findByRole('heading', { level: 1, name: t('settings.page.limits') })).toBeTruthy();
		expect(await screen.findByText(t('settings.pending.title', { count: 1 }))).toBeTruthy();
	});

	it('shows the empty state for an unknown group', async () => {
		render(SettingsGroupPlanet, { props: { group: 'nope' } });
		expect(await screen.findByText(t('settings.unknown.title'))).toBeTruthy();
	});
});

describe('optional size settings', () => {
	it('shows no limit when the temp save size is unset and 1 byte as the minimum', () => {
		const entry = entryOf('backend.localfs_tmp_save_max_size');
		expect(describeValue(entry, null, false)).toBe(t('settings.unset.backend.localfs_tmp_save_max_size'));
		expect(metaFor(entry, undefined, null, null, null)).toMatchObject({ optional: true, min: 1 });
	});
});
