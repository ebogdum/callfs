import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { probeBackend, probeInstance, removeEndpoint, setEndpoint } from '$lib/api/admin/topology';
import { t } from '$lib/copy/t';
import { createMutation } from '$lib/data/mutation.svelte';
import { createQuery } from '$lib/data/query.svelte';
import ServersPlanet from './ServersPlanet.svelte';
import StoragePlanet from './StoragePlanet.svelte';

vi.mock('$lib/data/query.svelte', () => ({ createQuery: vi.fn() }));
vi.mock('$lib/data/mutation.svelte', () => ({ createMutation: vi.fn() }));
vi.mock('$lib/api/admin/topology', () => ({
	instances: vi.fn(),
	backends: vi.fn(),
	probeInstance: vi.fn(),
	probeBackend: vi.fn(),
	setEndpoint: vi.fn(),
	removeEndpoint: vi.fn()
}));
vi.mock('$lib/api/admin/findings', () => ({ findings: vi.fn() }));
vi.mock('$lib/api/nodes', () => ({
	listNodes: vi.fn(() => Promise.resolve({ ok: true, value: { self: '', nodes: [] } })),
	roleOf: () => 'none'
}));
vi.mock('$lib/api/admin/telemetry', () => ({ series: vi.fn() }));

const UPDATED_AT = 1_800_000_000_000;
const DAY_SECONDS = 86400;
const TOTAL_BYTES = 100_000_000_000;
const USED_BYTES = 2_000_000_000;

const queries: Record<string, Record<string, unknown>> = {};

const query = (overrides: Record<string, unknown>) => ({
	data: undefined,
	error: null,
	status: 'success',
	updatedAt: UPDATED_AT,
	fetching: false,
	refetch: vi.fn(),
	setData: vi.fn(),
	...overrides
});

const serverSelf = {
	instance_id: 'callfs-1',
	endpoint: 'https://files-1.example.com:8443',
	source: 'self',
	is_self: true,
	scheme: 'https',
	reachable: true,
	rtt_ms: 3,
	checked_at: '2026-10-23T10:00:00Z',
	files: 1204,
	bytes: 5_000_000_000,
	pieces: 0
};

const serverPlain = {
	instance_id: 'callfs-2',
	endpoint: 'http://files-2.example.com:8080',
	source: 'config',
	is_self: false,
	scheme: 'http',
	reachable: false,
	rtt_ms: null,
	checked_at: '2026-10-23T10:00:00Z',
	files: 40,
	bytes: 900_000_000,
	pieces: 12
};

const localDisk = {
	name: 'localfs',
	enabled: true,
	status: 'ok',
	root_path: '/var/lib/callfs',
	free_bytes: TOTAL_BYTES - USED_BYTES,
	total_bytes: TOTAL_BYTES,
	used_bytes: USED_BYTES,
	bucket: null,
	region: null,
	endpoint: null,
	encryption: 'none',
	acl: null,
	kms_key_set: false,
	peers: 0,
	last_probe: { at: '2026-10-23T10:00:00Z', ok: true, latency_ms: 4, error: null }
};

const s3Bucket = {
	name: 's3',
	enabled: false,
	status: 'disabled',
	root_path: null,
	free_bytes: null,
	total_bytes: null,
	used_bytes: null,
	bucket: 'callfs-files',
	region: 'eu-central-1',
	endpoint: null,
	encryption: 'AES256',
	acl: 'public-read',
	kms_key_set: true,
	peers: 0,
	last_probe: null
};

const otherServers = {
	name: 'internal_proxy',
	enabled: true,
	status: 'ok',
	root_path: null,
	free_bytes: null,
	total_bytes: null,
	used_bytes: null,
	bucket: null,
	region: null,
	endpoint: null,
	encryption: null,
	acl: null,
	kms_key_set: false,
	peers: 2,
	last_probe: null
};

beforeEach(() => {
	vi.clearAllMocks();
	for (const key of Object.keys(queries)) {
		delete queries[key];
	}
	queries.findings = query({ data: { items: [], counts: { critical: 0, warning: 0, info: 0 } } });
	vi.mocked(createQuery).mockImplementation(
		((options: { key: readonly unknown[] }) => queries[String(options.key[0])]) as never
	);
	vi.mocked(createMutation).mockImplementation(
		((options: { run: (vars: unknown) => Promise<unknown> }) => ({
			run: (vars: unknown) => options.run(vars),
			pending: false,
			error: null
		})) as never
	);
	vi.mocked(setEndpoint).mockResolvedValue({} as never);
	vi.mocked(probeInstance).mockResolvedValue({} as never);
	vi.mocked(probeBackend).mockResolvedValue({} as never);
	vi.mocked(removeEndpoint).mockResolvedValue({} as never);
});

afterEach(() => {
	cleanup();
});

describe('ServersPlanet', () => {
	it('renders the page heading and the primary add action', () => {
		queries.instances = query({ data: { ok: true, value: { self: serverSelf, items: [serverSelf] } }, status: 'success' });
		render(ServersPlanet);
		expect(screen.getByRole('heading', { level: 1, name: t('servers-storage.servers.title') })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('servers-storage.servers.add') })).toBeTruthy();
	});

	it('lists every server with its ID and address', () => {
		queries.instances = query({
			data: { ok: true, value: { self: serverSelf, items: [serverSelf, serverPlain] } },
			status: 'success'
		});
		render(ServersPlanet);
		expect(screen.getByText('callfs-1')).toBeTruthy();
		expect(screen.getByText('callfs-2')).toBeTruthy();
		expect(screen.getByText('http://files-2.example.com:8080')).toBeTruthy();
	});

	it('shows the partial alert when a server cannot be reached', () => {
		queries.instances = query({
			data: { ok: true, value: { self: serverSelf, items: [serverSelf, serverPlain] } },
			status: 'partial'
		});
		render(ServersPlanet);
		expect(screen.getByText(t('servers-storage.servers.partial'))).toBeTruthy();
	});

	it('shows the empty copy with an add action when no server is listed', () => {
		queries.instances = query({ data: { ok: true, value: { self: serverSelf, items: [] } }, status: 'empty' });
		render(ServersPlanet);
		expect(screen.getByText(t('servers-storage.servers.empty.title'))).toBeTruthy();
		expect(screen.getByText(t('servers-storage.servers.empty.body'))).toBeTruthy();
	});

	it('opens the add dialog and keeps submit disabled until the ID and address are valid', async () => {
		queries.instances = query({ data: { ok: true, value: { self: serverSelf, items: [serverSelf] } }, status: 'success' });
		render(ServersPlanet);
		await fireEvent.click(screen.getByRole('button', { name: t('servers-storage.servers.add') }));
		const dialog = await screen.findByRole('dialog', { name: t('servers-storage.form.add.title') });
		const submit = within(dialog).getByRole('button', { name: t('servers-storage.form.submit.add') });
		expect(submit.getAttribute('aria-disabled')).toBe('true');
	});

	it('saves the new server address with persist enabled once the form is valid', async () => {
		queries.instances = query({ data: { ok: true, value: { self: serverSelf, items: [serverSelf] } }, status: 'success' });
		render(ServersPlanet);
		await fireEvent.click(screen.getByRole('button', { name: t('servers-storage.servers.add') }));
		const dialog = await screen.findByRole('dialog', { name: t('servers-storage.form.add.title') });
		await fireEvent.input(within(dialog).getByLabelText(t('servers-storage.form.server_id.label')), {
			target: { value: 'callfs-3' }
		});
		await fireEvent.input(within(dialog).getByLabelText(t('servers-storage.form.address.label')), {
			target: { value: 'https://files-3.example.com:8443' }
		});
		const submit = within(dialog).getByRole('button', { name: t('servers-storage.form.submit.add') });
		await waitFor(() => expect(submit.getAttribute('aria-disabled')).not.toBe('true'));
		await fireEvent.click(submit);
		await waitFor(() =>
			expect(setEndpoint).toHaveBeenCalledWith('callfs-3', 'https://files-3.example.com:8443', true)
		);
	});
});

describe('StoragePlanet', () => {
	it('renders the page heading and the check storage action', () => {
		queries.backends = query({
			data: { default: 'localfs', items: [localDisk, s3Bucket, otherServers] },
			status: 'success'
		});
		queries.series = query({ data: { series: [] } });
		render(StoragePlanet);
		expect(screen.getByRole('heading', { level: 1, name: t('servers-storage.storage.title') })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('servers-storage.storage.check') })).toBeTruthy();
	});

	it('renders one card per storage with the default badge on the default storage', () => {
		queries.backends = query({
			data: { default: 'localfs', items: [localDisk, s3Bucket, otherServers] },
			status: 'success'
		});
		queries.series = query({ data: { series: [] } });
		render(StoragePlanet);
		expect(
			screen.getByRole('heading', { level: 2, name: t('servers-storage.storage.card.localfs') })
		).toBeTruthy();
		expect(screen.getByRole('heading', { level: 2, name: t('servers-storage.storage.card.s3') })).toBeTruthy();
		expect(
			screen.getByRole('heading', { level: 2, name: t('servers-storage.storage.card.internal_proxy') })
		).toBeTruthy();
		expect(screen.getByText(t('servers-storage.storage.default_badge'))).toBeTruthy();
	});

	it('projects when local disk fills from the usage series', () => {
		const start = 1_800_000_000;
		queries.backends = query({
			data: { default: 'localfs', items: [localDisk] },
			status: 'success'
		});
		queries.series = query({
			data: {
				series: [
					{
						points: [
							[start, 0],
							[start + 2 * DAY_SECONDS, USED_BYTES]
						]
					}
				]
			}
		});
		render(StoragePlanet);
		expect(screen.getByText(t('servers-storage.storage.full_in', { days: '98' }))).toBeTruthy();
	});

	it('warns when a bucket is readable by anyone', () => {
		queries.backends = query({
			data: { default: 'localfs', items: [localDisk, { ...s3Bucket, enabled: true, status: 'ok' }] },
			status: 'success'
		});
		queries.series = query({ data: { series: [] } });
		render(StoragePlanet);
		expect(screen.getByText(t('servers-storage.storage.acl.public'))).toBeTruthy();
	});

	it('probes only enabled storages when check storage is pressed', async () => {
		queries.backends = query({
			data: { default: 'localfs', items: [localDisk, s3Bucket, otherServers] },
			status: 'success'
		});
		queries.series = query({ data: { series: [] } });
		render(StoragePlanet);
		await fireEvent.click(screen.getByRole('button', { name: t('servers-storage.storage.check') }));
		await waitFor(() => expect(probeBackend).toHaveBeenCalledTimes(2));
		expect(probeBackend).toHaveBeenCalledWith('localfs');
		expect(probeBackend).toHaveBeenCalledWith('internal_proxy');
		expect(probeBackend).not.toHaveBeenCalledWith('s3');
	});

	it('shows the empty copy with a settings link when no storage is set up', () => {
		queries.backends = query({ data: { default: 'localfs', items: [] }, status: 'empty' });
		queries.series = query({ data: { series: [] } });
		render(StoragePlanet);
		expect(screen.getByText(t('servers-storage.storage.empty.title'))).toBeTruthy();
		expect(screen.getByRole('link', { name: t('servers-storage.storage.settings_link') })).toBeTruthy();
	});
});
