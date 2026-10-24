import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { validate as validateConfig } from '$lib/api/admin/config';
import type { FindingPage } from '$lib/api/admin/findings';
import type { ConfigView, Finding, LinkStats, TlsInfo } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { createQuery, type Query } from '$lib/data/query.svelte';
import type { UiError } from '$lib/ui/types';
import ConfigFilePlanet from './ConfigFilePlanet.svelte';
import SecurityPlanet from './SecurityPlanet.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));
vi.mock('$lib/api/admin/users', () => ({ tls: vi.fn() }));
vi.mock('$lib/api/admin/findings', () => ({ findings: vi.fn() }));
vi.mock('$lib/api/admin/config', () => ({
	getConfig: vi.fn(),
	save: vi.fn(),
	exportConfig: vi.fn(),
	validate: vi.fn()
}));
vi.mock('$lib/api/admin/status', () => ({ status: vi.fn(), restart: vi.fn() }));
vi.mock('$lib/api/links', () => ({ linkStats: vi.fn() }));
vi.mock('$lib/data/query.svelte', () => ({ createQuery: vi.fn() }));

interface Fixture {
	data?: unknown;
	error?: UiError | null;
}

const networkError: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true
};

const tlsInfo: TlsInfo = {
	protocol_configured: 'https',
	protocol_effective: 'https',
	fallback: false,
	cert: {
		path: '/etc/callfs/server.crt',
		subject: 'files.example.com',
		issuer: 'Example CA',
		sans: ['files.example.com'],
		not_before: '2026-01-01T00:00:00Z',
		not_after: '2027-01-12T00:00:00Z',
		days_remaining: 81
	},
	key: { path: '/etc/callfs/server.key', mode: '0600', world_readable: false },
	quic: { enabled: false, addr: '' },
	hsts: true,
	headers: {
		csp: "default-src 'self'",
		frame_options: 'DENY',
		coop: 'same-origin',
		coep: 'require-corp'
	}
};

const plainHttpFinding: Finding = {
	id: 'tls.plain_http',
	severity: 'critical',
	area: 'security',
	title: 'Connections are not encrypted',
	body: 'Access keys travel in the clear.',
	affected: [],
	since: '2026-10-01T00:00:00Z',
	fix: { label: 'Open network', route: '/settings/network', setting_key: 'server.protocol', command: null },
	muted: null
};

const findingPage: FindingPage = {
	items: [plainHttpFinding],
	counts: { critical: 1, warning: 0, info: 0 }
};

const emptyFindings: FindingPage = { items: [], counts: { critical: 0, warning: 0, info: 0 } };

const linkTotals: LinkStats = {
	window: '24h',
	generated: 3,
	consumed: { success: 0, not_found: 0, expired: 0, invalid: 0 },
	active: 23,
	used_last_24h: 0,
	last_cleanup: { at: null, expired_removed: 0, used_removed: 0, error: null }
};

function configWith(allowRestart: boolean, pending: string[] = []): ConfigView {
	return {
		config_path: '/etc/callfs/config.yaml',
		restart_required: 0 < pending.length,
		pending,
		keys: [
			{
				path: 'auth.internal_proxy_secret',
				value: null,
				default: null,
				source: 'file',
				env: 'CALLFS_AUTH__INTERNAL_PROXY_SECRET',
				secret: true,
				set: true,
				restart: 'restart',
				valid: true
			},
			{
				path: 'auth.single_use_link_secret',
				value: null,
				default: null,
				source: 'file',
				env: 'CALLFS_AUTH__SINGLE_USE_LINK_SECRET',
				secret: true,
				set: true,
				restart: 'restart',
				valid: true
			},
			{
				path: 'backend.internal_proxy_skip_tls_verify',
				value: false,
				default: false,
				source: 'default',
				env: '',
				secret: false,
				set: false,
				restart: 'restart',
				valid: true
			},
			{
				path: 'ui.allow_remote_restart',
				value: allowRestart,
				default: false,
				source: 'file',
				env: '',
				secret: false,
				set: false,
				restart: 'restart',
				valid: true
			}
		]
	};
}

const adminStatus = {
	config_path: '/etc/callfs/config.yaml',
	config_format: 'yaml',
	started_at: '2026-10-20T08:00:00Z',
	keys_from_env: ['CALLFS_SERVER__READ_TIMEOUT'],
	keys_at_default: ['server.read_timeout', 'server.write_timeout']
};

function ok<T>(value: T): { ok: true; value: T } {
	return { ok: true, value };
}

function failed(error: UiError): { ok: false; error: UiError } {
	return { ok: false, error };
}

function buildQuery(fixture: Fixture): Query<unknown> {
	return {
		data: fixture.data,
		status: 'success',
		error: fixture.error ?? null,
		updatedAt: null,
		fetching: false,
		refetch: vi.fn(() => Promise.resolve()),
		setData: vi.fn()
	} as unknown as Query<unknown>;
}

function installQueries(fixtures: Record<string, Fixture>): void {
	vi.mocked(createQuery).mockImplementation((options) => {
		const resolved = 'function' === typeof options ? options() : options;
		const id = resolved.key.join('|');
		const prefix = Object.keys(fixtures).find((key) => id.startsWith(key));
		return buildQuery(undefined === prefix ? {} : fixtures[prefix]);
	});
}

beforeEach(() => {
	vi.clearAllMocks();
});

afterEach(() => {
	cleanup();
});

describe('SecurityPlanet', () => {
	it('renders the page title and one row per exposure check on success', () => {
		installQueries({
			'security-page': {
				data: {
					tls: ok(tlsInfo),
					findings: ok(emptyFindings),
					config: ok(configWith(false)),
					links: ok(linkTotals)
				}
			}
		});
		render(SecurityPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('security.title') })).toBeTruthy();
		const checklist = screen.getByRole('list', { name: t('security.checklist.list') });
		expect(within(checklist).getAllByRole('listitem')).toHaveLength(13);
		expect(screen.queryByRole('button', { name: t('security.fix_top') })).toBeNull();
	});

	it('offers a fix link and the top issue button when a critical finding exists', () => {
		installQueries({
			'security-page': {
				data: {
					tls: ok(tlsInfo),
					findings: ok(findingPage),
					config: ok(configWith(false)),
					links: ok(linkTotals)
				}
			}
		});
		render(SecurityPlanet);

		expect(screen.getByText(t('security.word.danger'))).toBeTruthy();
		expect(
			screen.getByRole('link', { name: `${t('security.check.fix')}: ${t('security.check.https.label')}` })
		).toBeTruthy();
		expect(screen.getByRole('button', { name: t('security.fix_top') })).toBeTruthy();
	});

	it('shows the loading skeleton before the queries resolve', () => {
		installQueries({ 'security-page': {} });
		render(SecurityPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('security.title') })).toBeTruthy();
		expect(screen.queryByRole('list', { name: t('security.checklist.list') })).toBeNull();
	});

	it('renders the page error state when every section fails', () => {
		installQueries({
			'security-page': {
				data: {
					tls: failed(networkError),
					findings: failed(networkError),
					config: failed(networkError),
					links: failed(networkError)
				}
			}
		});
		render(SecurityPlanet);

		expect(screen.getByText(networkError.title)).toBeTruthy();
		expect(screen.queryByRole('list', { name: t('security.checklist.list') })).toBeNull();
	});

	it('renders the partial alert and withholds the checklist when findings fail', () => {
		installQueries({
			'security-page': {
				data: {
					tls: ok(tlsInfo),
					findings: failed(networkError),
					config: ok(configWith(false)),
					links: ok(linkTotals)
				}
			}
		});
		render(SecurityPlanet);

		expect(screen.getByText(t('security.partial.title'))).toBeTruthy();
		expect(screen.getByText(networkError.title)).toBeTruthy();
		expect(screen.queryByRole('list', { name: t('security.checklist.list') })).toBeNull();
	});

	it('disables both secret replacement reviews until a draft reaches the minimum length', async () => {
		installQueries({
			'security-page': {
				data: {
					tls: ok(tlsInfo),
					findings: ok(emptyFindings),
					config: ok(configWith(false)),
					links: ok(linkTotals)
				}
			}
		});
		render(SecurityPlanet);

		const labels = [t('security.secret.internal.label'), t('security.secret.link.label')];
		for (const name of labels) {
			await fireEvent.click(screen.getByRole('button', { name: t('editors.secret.replace_named', { name }) }));
			await fireEvent.input(screen.getByLabelText(name), { target: { value: 'short' } });
		}

		const reviews = screen.getAllByRole('button', { name: t('security.secret.review') });
		expect(reviews).toHaveLength(2);
		for (const review of reviews) {
			expect(review.getAttribute('aria-disabled')).toBe('true');
		}

		await fireEvent.input(screen.getByLabelText(labels[0]), { target: { value: 'a'.repeat(16) } });

		const enabled = screen.getAllByRole('button', { name: t('security.secret.review') });
		expect(enabled[0].getAttribute('aria-disabled')).toBeNull();
		expect(enabled[1].getAttribute('aria-disabled')).toBe('true');
	});

	it('shows the certificate verification switch on when servers check certificates', () => {
		installQueries({
			'security-page': {
				data: {
					tls: ok(tlsInfo),
					findings: ok(emptyFindings),
					config: ok(configWith(false)),
					links: ok(linkTotals)
				}
			}
		});
		render(SecurityPlanet);

		const verify = screen.getByRole('switch', { name: t('security.config.verify.label') });
		expect(verify.getAttribute('aria-checked')).toBe('true');
	});
});

describe('ConfigFilePlanet', () => {
	function installFileQueries(config: ConfigView): void {
		installQueries({
			'security-file|status': { data: ok(adminStatus) },
			'security-file|config': { data: ok(config) },
			'security-file|export': { data: ok('callfs:\n  server:\n    port: 8443\n') }
		});
	}

	it('shows the config file path and the YAML export tab selected', () => {
		installFileQueries(configWith(false));
		render(ConfigFilePlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('security.file.title') })).toBeTruthy();
		expect(screen.getByText('/etc/callfs/config.yaml')).toBeTruthy();
		expect(screen.getByRole('tab', { name: t('security.file.tab.yaml') }).getAttribute('aria-selected')).toBe('true');
	});

	it('explains how to restart when remote restart is not allowed', () => {
		installFileQueries(configWith(false, ['server.port']));
		render(ConfigFilePlanet);

		expect(screen.getByText(t('settings-screens.file.restart.manual'))).toBeTruthy();
		expect(screen.queryByRole('button', { name: t('security.file.restart.button') })).toBeNull();
	});

	it('offers the restart button when remote restart is allowed', () => {
		installFileQueries(configWith(true));
		render(ConfigFilePlanet);

		expect(screen.getByRole('button', { name: t('security.file.restart.button') })).toBeTruthy();
	});

	it('states that no saved changes are waiting when nothing is pending', () => {
		installFileQueries(configWith(false));
		render(ConfigFilePlanet);

		expect(screen.getByText(t('security.file.pending.none'))).toBeTruthy();
	});

	it('reports a valid config after validation', async () => {
		vi.mocked(validateConfig).mockResolvedValue(
			ok({ valid: true, errors: [], warnings: [], restart_required: [], applies_now: [] })
		);
		installFileQueries(configWith(false));
		render(ConfigFilePlanet);

		await fireEvent.click(screen.getByRole('button', { name: t('security.file.validate') }));

		expect(validateConfig).toHaveBeenCalledWith({});
		expect(await screen.findByText(t('security.file.validate.ok'))).toBeTruthy();
	});

	it('reports the first invalid value after validation fails', async () => {
		vi.mocked(validateConfig).mockResolvedValue(
			ok({
				valid: false,
				errors: [{ path: 'server.port', message: 'Port must be 1-65535.' }],
				warnings: [],
				restart_required: [],
				applies_now: []
			})
		);
		installFileQueries(configWith(false));
		render(ConfigFilePlanet);

		await fireEvent.click(screen.getByRole('button', { name: t('security.file.validate') }));

		expect(await screen.findByText(t('security.file.validate.fail'))).toBeTruthy();
		expect(screen.getByText(/First: Port must be 1-65535\./)).toBeTruthy();
	});

	it('renders the page error state when the status and config queries both fail', () => {
		installQueries({
			'security-file|status': { data: failed(networkError) },
			'security-file|config': { data: failed(networkError) },
			'security-file|export': { data: ok('') }
		});
		render(ConfigFilePlanet);

		expect(screen.getByText(networkError.title)).toBeTruthy();
		expect(screen.queryByRole('tab', { name: t('security.file.tab.yaml') })).toBeNull();
	});
});
