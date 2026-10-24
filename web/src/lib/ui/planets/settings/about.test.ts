import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { goto } from '$app/navigation';
import { API_ROUTES } from '$lib/api/routes';
import { status } from '$lib/api/admin/status';
import { capabilities as fetchCapabilities } from '$lib/api/session';
import type { AdminStatus, Capabilities } from '$lib/api/types';
import { invalidateAll } from '$lib/data/cache';
import { capabilities } from '$lib/stores/capabilities.svelte';
import type { UiError } from '$lib/ui/types';
import NotFoundPlanet from '../NotFoundPlanet.svelte';
import AboutPlanet from './AboutPlanet.svelte';

const sessionState = vi.hoisted(() => ({ isAdmin: true }));

vi.mock('$lib/stores/session.svelte', () => ({ session: sessionState }));

vi.mock('$lib/api/session', () => ({ capabilities: vi.fn() }));

vi.mock('$lib/api/admin/status', () => ({ status: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn(async () => undefined) }));

const capabilitiesFixture: Capabilities = {
	version: '1.4.2',
	commit: 'a1b2c3d',
	build_date: '2026-10-20T08:00:00Z',
	go_version: 'go1.26.1',
	instance_id: 'callfs-instance-1',
	node_id: null,
	features: {
		erasure: true,
		nats: false,
		cross_server: true,
		websocket: true,
		replication: false,
		quic: false,
		metrics_listener: true,
		trash: true,
		resumable_uploads: true,
		search: true,
		ui: true
	},
	storages: ['localfs'],
	default_storage: 'localfs',
	file_index: 'sqlite',
	lock_type: 'local',
	external_url: 'https://files.example.com',
	limits: {
		max_upload_bytes: 10737418240,
		max_split_upload_bytes: 1073741824,
		max_ws_upload_bytes: 100000000,
		ws_frame_bytes: 1048576,
		max_patch_body_bytes: 1048576,
		link_expiry_min_s: 1,
		link_expiry_max_s: 86400,
		list_page_max: 1000,
		upload_part_bytes: 8388608
	},
	erasure_defaults: { data_shards: 4, parity_shards: 2, max_total_shards: 64, min_file_size: 0 },
	trash_retention_s: 2592000
};

const statusFixture: AdminStatus = {
	version: '1.4.2',
	commit: 'a1b2c3d',
	build_date: '2026-10-20T08:00:00Z',
	go_version: 'go1.26.1',
	instance_id: 'callfs-instance-1',
	node_id: null,
	started_at: '2026-10-22T08:00:00Z',
	uptime_s: 90061,
	config_path: '/etc/callfs/config.yaml',
	config_format: 'yaml',
	keys_from_env: [],
	keys_at_default: [],
	listeners: [{ kind: 'https', addr: ':8443', tls: true, auth: true }],
	protocol_configured: 'auto',
	protocol_effective: 'https',
	file_index: 'sqlite',
	lock_type: 'local',
	storages: { localfs: true, s3: false, internal_proxy: false, erasure: true },
	runtime: { goroutines: 42, heap_bytes: 1000, open_fds: null, cpu_seconds: 1.5 }
};

const loadFailure: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true,
	layer: 'network'
};

const internalNote = 'Used between servers. People use the cluster pages instead.';

describe('AboutPlanet', () => {
	beforeEach(() => {
		invalidateAll();
		capabilities.reset();
		sessionState.isAdmin = true;
		vi.mocked(fetchCapabilities).mockResolvedValue({ ok: true, value: capabilitiesFixture });
		vi.mocked(status).mockClear();
		vi.mocked(status).mockResolvedValue({ ok: true, value: statusFixture });
	});

	it('shows the page heading and job line', () => {
		render(AboutPlanet);

		expect(screen.getByRole('heading', { level: 1, name: 'About' })).toBeInTheDocument();
		expect(screen.getByText('See the version and features.')).toBeInTheDocument();
	});

	it('shows version, commit, server ID and uptime for an admin', async () => {
		render(AboutPlanet);

		expect(await screen.findByText('1.4.2')).toBeInTheDocument();
		expect(screen.getByText('a1b2c3d')).toBeInTheDocument();
		expect(screen.getByText('callfs-instance-1')).toBeInTheDocument();
		expect(screen.getByText('Uptime')).toBeInTheDocument();
	});

	it('marks each feature as on or off with a word', async () => {
		render(AboutPlanet);

		const splitStorage = (await screen.findByText('Split storage')).closest('li');
		const nats = screen.getByText('Clustering').closest('li');

		expect(splitStorage).not.toBeNull();
		expect(nats).not.toBeNull();
		expect(within(splitStorage as HTMLElement).getByText('On')).toBeInTheDocument();
		expect(within(nats as HTMLElement).getByText('Off')).toBeInTheDocument();
	});

	it('lists every API route in the parity table', async () => {
		render(AboutPlanet);

		await screen.findByText('1.4.2');
		const rows = within(screen.getByRole('table')).getAllByRole('row');

		expect(rows).toHaveLength(API_ROUTES.length + 1);
	});

	it('explains internal routes instead of listing screens', async () => {
		render(AboutPlanet);

		await screen.findByText('1.4.2');
		const internalRoutes = API_ROUTES.filter((route) => true === route.internal);

		expect(screen.getAllByText(internalNote)).toHaveLength(internalRoutes.length);
	});

	it('lists the third-party licenses', () => {
		render(AboutPlanet);

		expect(screen.getByText('Inter, SIL Open Font License 1.1')).toBeInTheDocument();
		expect(screen.getByText('JetBrains Mono, SIL Open Font License 1.1')).toBeInTheDocument();
		expect(screen.getByText('Lucide icons, ISC License')).toBeInTheDocument();
		expect(screen.getByText('bits-ui, MIT License')).toBeInTheDocument();
	});

	it('hides uptime and skips the status request for a member', async () => {
		sessionState.isAdmin = false;
		render(AboutPlanet);

		expect(await screen.findByText('1.4.2')).toBeInTheDocument();
		expect(screen.queryByText('Uptime')).not.toBeInTheDocument();
		expect(status).not.toHaveBeenCalled();
	});

	it('shows a loading skeleton while capabilities load', () => {
		vi.mocked(fetchCapabilities).mockReturnValue(new Promise(() => undefined));
		render(AboutPlanet);

		expect(screen.getByRole('heading', { name: 'Version' })).toBeInTheDocument();
		expect(screen.queryByText('Commit')).not.toBeInTheDocument();
	});

	it('shows the error state when capabilities fail', async () => {
		vi.mocked(fetchCapabilities).mockResolvedValue({ ok: false, error: loadFailure });
		render(AboutPlanet);

		expect(await screen.findByText(loadFailure.title)).toBeInTheDocument();
		expect(screen.queryByText('Commit')).not.toBeInTheDocument();
	});

	it('keeps version details and warns when the status request fails', async () => {
		vi.mocked(status).mockResolvedValue({ ok: false, error: loadFailure });
		render(AboutPlanet);

		expect(await screen.findByText('Some details did not load.')).toBeInTheDocument();
		expect(screen.getByText('a1b2c3d')).toBeInTheDocument();
	});
});

describe('NotFoundPlanet', () => {
	beforeEach(() => {
		vi.mocked(goto).mockClear();
	});

	it('shows the not found heading and plain guidance', () => {
		render(NotFoundPlanet);

		expect(screen.getByRole('heading', { level: 1, name: 'We cannot find that page.' })).toBeInTheDocument();
		expect(screen.getByText('Check the address or go back.')).toBeInTheDocument();
	});

	it('offers Go to Overview and Search files buttons', () => {
		render(NotFoundPlanet);

		expect(screen.getByRole('button', { name: 'Go to Overview' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Search files' })).toBeInTheDocument();
	});

	it('opens Overview when Go to Overview is pressed', () => {
		render(NotFoundPlanet);

		fireEvent.click(screen.getByRole('button', { name: 'Go to Overview' }));

		expect(goto).toHaveBeenCalledWith('/ui/');
	});

	it('opens search when Search files is pressed', () => {
		render(NotFoundPlanet);

		fireEvent.click(screen.getByRole('button', { name: 'Search files' }));

		expect(goto).toHaveBeenCalledWith('/ui/files/search');
	});

	it('links to the three top pages', () => {
		render(NotFoundPlanet);

		const links = within(screen.getByRole('navigation', { name: 'Top pages' })).getAllByRole('link');

		expect(links.map((link) => link.getAttribute('href'))).toEqual(['/ui/', '/ui/files', '/ui/sharing']);
	});
});
