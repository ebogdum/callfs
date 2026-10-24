import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getConfig } from '$lib/api/admin/config';
import { authFailures, users } from '$lib/api/admin/users';
import { logs } from '$lib/api/admin/telemetry';
import type { AuthFailureList, UserList } from '$lib/api/admin/users';
import type { ConfigView, LogEntry, UserInfo } from '$lib/api/types';
import { invalidateAll } from '$lib/data/cache';
import LogsPlanet from './LogsPlanet.svelte';
import SignInsPlanet from './SignInsPlanet.svelte';

vi.mock('$lib/api/admin/telemetry', () => ({ logs: vi.fn() }));

vi.mock('$lib/api/admin/users', () => ({ authFailures: vi.fn(), users: vi.fn() }));

vi.mock('$lib/api/admin/config', () => ({ getConfig: vi.fn() }));

const infoLine: LogEntry = {
	id: 'log-1',
	at: '2026-10-23T10:00:00Z',
	level: 'info',
	message: 'Listening on 127.0.0.1:8443',
	logger: 'server',
	fields: {},
	request_id: null
};

const liveLine: LogEntry = {
	id: 'log-2',
	at: '2026-10-23T10:01:00Z',
	level: 'error',
	message: 'Live line from the stream',
	logger: 'server',
	fields: {},
	request_id: 'req-9'
};

const config: ConfigView = {
	config_path: '/etc/callfs/config.yaml',
	restart_required: false,
	pending: [],
	keys: [
		{
			path: 'log.level',
			value: 'info',
			default: 'info',
			source: 'default',
			env: 'CALLFS_LOG_LEVEL',
			secret: false,
			set: false,
			restart: 'runtime',
			valid: true
		}
	]
};

const failureListing: AuthFailureList = {
	total_24h: 38,
	items: [
		{ at: '2026-10-23T09:12:00Z', client: '203.0.113.9', reason: 'wrong_key', path: '/team/report.pdf' }
	]
};

const emptyFailures: AuthFailureList = { total_24h: 0, items: [] };

const alice: UserInfo = {
	user_id: 'alice',
	source: 'api_key_users',
	index: null,
	key_hint: 'a1b2',
	key_fingerprint: 'sha256:0123456789abcdef',
	is_admin: true,
	owned_files: 4,
	owned_bytes: 2048,
	last_auth_at: '2026-10-23T08:30:00Z',
	requests_24h: 120,
	auth_failures_24h: 2
};

const userListing: UserList = { items: [alice], positional_count: 0 };

class FakeEventSource {
	static instances: FakeEventSource[] = [];

	url: string;
	closed = false;
	onopen: (() => void) | null = null;
	onerror: (() => void) | null = null;
	private readonly listeners = new Map<string, EventListener>();

	constructor(url: string) {
		this.url = url;
		FakeEventSource.instances.push(this);
	}

	addEventListener(type: string, listener: EventListener): void {
		this.listeners.set(type, listener);
	}

	removeEventListener(type: string): void {
		this.listeners.delete(type);
	}

	close(): void {
		this.closed = true;
	}

	emit(type: string, data: string): void {
		this.listeners.get(type)?.(new MessageEvent(type, { data }));
	}
}

const networkFailure = { kind: 'network' as const };

beforeEach(() => {
	invalidateAll();
	vi.clearAllMocks();
	FakeEventSource.instances = [];
	vi.stubGlobal('EventSource', FakeEventSource);
	vi.mocked(logs).mockResolvedValue({ ok: true, value: { items: [infoLine], next_cursor: null } });
	vi.mocked(getConfig).mockResolvedValue({ ok: true, value: config });
	vi.mocked(authFailures).mockResolvedValue({ ok: true, value: failureListing });
	vi.mocked(users).mockResolvedValue({ ok: true, value: userListing });
});

afterEach(() => {
	cleanup();
	vi.unstubAllGlobals();
});

describe('LogsPlanet', () => {
	it('renders the Logs heading and the current log level with a link to change it', async () => {
		render(LogsPlanet);
		await waitFor(() => expect(screen.getByRole('heading', { name: 'Logs' })).toBeInTheDocument());
		expect(screen.getByText('Log level is info.')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: /Change it/ })).toHaveAttribute('href', '/ui/settings/logging');
	});

	it('shows the log lines returned by the server', async () => {
		render(LogsPlanet);
		await waitFor(() => expect(screen.getByText('Listening on 127.0.0.1:8443')).toBeInTheDocument());
	});

	it('shows the plain error copy when the log request fails', async () => {
		vi.mocked(logs).mockResolvedValue({ ok: false, error: networkFailure });
		render(LogsPlanet);
		await waitFor(() => expect(screen.getByText('We cannot reach callfs.')).toBeInTheDocument());
	});

	it('appends a line received from the live stream', async () => {
		render(LogsPlanet);
		await waitFor(() => expect(FakeEventSource.instances.length).toBeGreaterThan(0));
		const source = FakeEventSource.instances.at(-1);
		expect(source?.url).toBe('/v1/admin/logs/stream');
		source?.emit('log', JSON.stringify(liveLine));
		await waitFor(() => expect(screen.getByText('Live line from the stream')).toBeInTheDocument());
	});

	it('warns that live updates stopped when a stream line cannot be read', async () => {
		render(LogsPlanet);
		await waitFor(() => expect(FakeEventSource.instances.length).toBeGreaterThan(0));
		FakeEventSource.instances.at(-1)?.emit('log', 'not json');
		await waitFor(() => expect(screen.getByText('Live updates stopped.')).toBeInTheDocument());
	});

	it('closes the stream when the page unmounts', async () => {
		const { unmount } = render(LogsPlanet);
		await waitFor(() => expect(FakeEventSource.instances.length).toBeGreaterThan(0));
		const source = FakeEventSource.instances.at(-1);
		unmount();
		expect(source?.closed).toBe(true);
	});
});

describe('SignInsPlanet', () => {
	it('renders the Sign-ins heading and a failed sign-in row with reason and client address', async () => {
		render(SignInsPlanet);
		await waitFor(() => expect(screen.getByRole('heading', { name: 'Sign-ins' })).toBeInTheDocument());
		await waitFor(() => expect(screen.getByText('203.0.113.9')).toBeInTheDocument());
		expect(screen.getByLabelText('Wrong key')).toBeInTheDocument();
		expect(screen.getByRole('region', { name: 'Failed sign-ins, 24 hours' })).toBeInTheDocument();
	});

	it('shows the empty copy when there are no failed sign-ins', async () => {
		vi.mocked(authFailures).mockResolvedValue({ ok: true, value: emptyFailures });
		render(SignInsPlanet);
		await waitFor(() =>
			expect(screen.getByText('No failed sign-ins in the last 24 hours.')).toBeInTheDocument()
		);
	});

	it('lists each user with the last sign-in in the key use table', async () => {
		render(SignInsPlanet);
		await waitFor(() => expect(screen.getByLabelText('alice')).toBeInTheDocument());
		expect(screen.getByText('a1b2')).toBeInTheDocument();
	});

	it('shows the plain error copy when the failed sign-ins request fails', async () => {
		vi.mocked(authFailures).mockResolvedValue({ ok: false, error: networkFailure });
		render(SignInsPlanet);
		await waitFor(() => expect(screen.getByText('We cannot reach callfs.')).toBeInTheDocument());
	});

	it('retries the failed sign-ins request from the error copy', async () => {
		vi.mocked(authFailures).mockResolvedValueOnce({ ok: false, error: networkFailure });
		render(SignInsPlanet);
		await waitFor(() => expect(screen.getByText('We cannot reach callfs.')).toBeInTheDocument());
		vi.mocked(authFailures).mockResolvedValue({ ok: true, value: failureListing });
		await fireEvent.click(screen.getAllByRole('button', { name: /Try again|Retry/ })[0] as HTMLElement);
		await waitFor(() => expect(screen.getByText('203.0.113.9')).toBeInTheDocument());
	});
});
