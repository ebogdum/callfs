import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuditEntry, Event as ActivityEvent } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { clearCache } from '$lib/data/cache';
import EventsPlanet from './EventsPlanet.svelte';
import AuditPlanet from './AuditPlanet.svelte';

const mocks = vi.hoisted(() => ({
	session: { isAdmin: true },
	url: new URL('http://localhost/activity'),
	eventHistory: vi.fn(),
	myEvents: vi.fn(),
	audit: vi.fn(),
	logs: vi.fn(),
	replaceState: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		get url() {
			return mocks.url;
		}
	}
}));

vi.mock('$app/navigation', () => ({ replaceState: mocks.replaceState }));

vi.mock('$lib/stores/session.svelte', () => ({ session: mocks.session }));

vi.mock('$lib/api/admin/telemetry', () => ({
	eventHistory: mocks.eventHistory,
	audit: mocks.audit,
	logs: mocks.logs
}));

vi.mock('$lib/api/session', () => ({ myEvents: mocks.myEvents }));

function activityEvent(type: string, message: string): ActivityEvent {
	return {
		id: `${type}-${message}`,
		at: new Date().toISOString(),
		type,
		level: 'info',
		actor: 'alice',
		instance_id: 'callfs-1',
		message,
		entities: [],
		fields: {}
	} as ActivityEvent;
}

function auditEntry(overrides: Partial<AuditEntry> = {}): AuditEntry {
	return {
		at: new Date().toISOString(),
		request_id: 'req-1',
		user_id: 'alice',
		method: 'DELETE',
		route: '/v1/files/*',
		path: '/team/a.txt',
		op: 'delete',
		status: 200,
		bytes_in: 0,
		bytes_out: 0,
		duration_ms: 12,
		instance_id: 'callfs-1',
		client: '10.0.0.5',
		...overrides
	} as AuditEntry;
}

describe('EventsPlanet', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		clearCache();
		mocks.session.isAdmin = true;
		mocks.eventHistory.mockResolvedValue({ ok: true, value: { items: [activityEvent('file.moved', 'File moved')] } });
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the title and the admin event feed', async () => {
		render(EventsPlanet);
		expect(await screen.findByText('File moved')).toBeInTheDocument();
		expect(screen.getByRole('heading', { level: 1, name: t('activity.events.title') })).toBeInTheDocument();
		expect(mocks.eventHistory).toHaveBeenCalledWith(expect.objectContaining({ since: expect.any(String) }));
	});

	it('filters events by type group and exposes the pressed state', async () => {
		mocks.eventHistory.mockResolvedValue({
			ok: true,
			value: { items: [activityEvent('file.moved', 'File moved'), activityEvent('job.finished', 'Job done')] }
		});
		render(EventsPlanet);
		await screen.findByText('File moved');
		const files = screen.getByRole('button', { name: t('u106logs.group.files') });
		await fireEvent.click(files);
		expect(files).toHaveAttribute('aria-pressed', 'true');
		expect(screen.queryByText('Job done')).not.toBeInTheDocument();
	});

	it('toggles the live switch with the t key', async () => {
		render(EventsPlanet);
		await screen.findByText('File moved');
		const live = screen.getByRole('button', { name: /live/i });
		const before = live.getAttribute('aria-pressed');
		await fireEvent.keyDown(window, { key: 't' });
		expect(live).toHaveAttribute('aria-pressed', 'true' === before ? 'false' : 'true');
	});

	it('shows the empty state when no event is in range', async () => {
		mocks.eventHistory.mockResolvedValue({ ok: true, value: { items: [] } });
		render(EventsPlanet);
		expect(await screen.findByText(t('activity.events.empty.title'))).toBeInTheDocument();
		expect(screen.getByText(t('activity.events.empty.body'))).toBeInTheDocument();
	});

	it('reads the member feed and skips the admin feed when the user is not an admin', async () => {
		mocks.session.isAdmin = false;
		mocks.myEvents.mockResolvedValue({ ok: true, value: { items: [activityEvent('file.created', 'Own upload')] } });
		render(EventsPlanet);
		expect(await screen.findByText('Own upload')).toBeInTheDocument();
		expect(mocks.eventHistory).not.toHaveBeenCalled();
	});
});

describe('AuditPlanet', () => {
	beforeEach(() => {
		vi.clearAllMocks();
		clearCache();
		mocks.session.isAdmin = true;
		mocks.audit.mockResolvedValue({ ok: true, value: { items: [auditEntry()], next_cursor: null } });
		mocks.logs.mockResolvedValue({ ok: true, value: { items: [] } });
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the audit rows with plain action words and an export action', async () => {
		render(AuditPlanet);
		expect(await screen.findByLabelText('/team/a.txt')).toBeInTheDocument();
		expect(screen.getByLabelText(t('activity.audit.op.delete'))).toBeInTheDocument();
		const clicked: string[] = [];
		const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
			clicked.push(this.getAttribute('href') ?? '');
		});
		await fireEvent.keyDown(screen.getByRole('button', { name: t('datatable.toolbar.export') }), { key: 'Enter' });
		await fireEvent.keyDown(screen.getByRole('menuitem', { name: t('datatable.toolbar.exportCsv') }), { key: 'Enter' });
		expect(clicked).toHaveLength(1);
		expect(clicked[0]).toContain('format=csv');
		click.mockRestore();
	});

	it('sends the parsed query tokens as audit filters', async () => {
		render(AuditPlanet);
		await screen.findByLabelText('/team/a.txt');
		const box = screen.getByRole('combobox', { name: t('activity.audit.query') });
		await fireEvent.input(box, { target: { value: 'user:alice op:delete status:>=400' } });
		await fireEvent.keyDown(box, { key: 'Enter' });
		await waitFor(() =>
			expect(mocks.audit).toHaveBeenLastCalledWith(
				expect.objectContaining({ user: 'alice', op: 'delete', status: '>=400' })
			)
		);
	});

	it('opens the row drawer and loads the linked logs for the request', async () => {
		render(AuditPlanet);
		const row = (await screen.findByLabelText('/team/a.txt')).closest('tr') as HTMLElement;
		await fireEvent.keyDown(row, { key: 'Enter' });
		expect(await screen.findByText(t('activity.audit.drawer.title'))).toBeInTheDocument();
		await waitFor(() =>
			expect(mocks.logs).toHaveBeenCalledWith(expect.objectContaining({ request_id: 'req-1' }))
		);
		expect(await screen.findByText(t('activity.audit.logs.empty'))).toBeInTheDocument();
	});

	it('shows the admin-only state and makes no request to members', async () => {
		mocks.session.isAdmin = false;
		render(AuditPlanet);
		expect(await screen.findByText(t('activity.audit.noPermission.title'))).toBeInTheDocument();
		expect(mocks.audit).not.toHaveBeenCalled();
	});

	it('shows the empty state when no action matches', async () => {
		mocks.audit.mockResolvedValue({ ok: true, value: { items: [], next_cursor: null } });
		render(AuditPlanet);
		expect(await screen.findByText(t('activity.audit.empty.title'))).toBeInTheDocument();
	});
});
