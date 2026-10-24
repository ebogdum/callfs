import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getLink, linkStats, listLinks, revokeLink } from '$lib/api/links';
import type { Link, LinkPage, LinkStats } from '$lib/api/types';
import { invalidateAll } from '$lib/data/cache';
import type { UiError } from '$lib/ui/types';
import LinkDetailPlanet from './LinkDetailPlanet.svelte';
import LinksPlanet from './LinksPlanet.svelte';

vi.mock('$lib/api/links', () => ({
	listLinks: vi.fn(),
	linkStats: vi.fn(),
	getLink: vi.fn(),
	revokeLink: vi.fn(),
	generateLink: vi.fn()
}));

vi.mock('$lib/api/admin/jobs', () => ({ runWorker: vi.fn() }));

vi.mock('$lib/api/files', () => ({ stat: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));

vi.mock('$app/state', () => ({
	page: { url: new URL('http://127.0.0.1:4173/sharing'), params: { id: '7' } }
}));

const link: Link = {
	id: 7,
	token_prefix: 'a1b2c3d4',
	file_path: '/team/report.pdf',
	status: 'active',
	created_by: 'alice',
	created_at: '2026-10-23T10:00:00Z',
	expires_at: '2099-10-24T10:00:00Z',
	used_at: null,
	used_by_ip: null,
	attempts: [{ at: '2026-10-23T10:05:00Z', result: 'success', ip: '203.0.113.9' }]
};

const linkListing: LinkPage = {
	items: [link],
	next_cursor: null,
	counts: { active: 1, used: 0, expired: 0, revoked: 0 }
};

const emptyListing: LinkPage = {
	items: [],
	next_cursor: null,
	counts: { active: 0, used: 0, expired: 0, revoked: 0 }
};

const linkNumbers: LinkStats = {
	window: '24h',
	generated: 3,
	consumed: { success: 9, not_found: 1, expired: 1, invalid: 1 },
	active: 1,
	used_last_24h: 9,
	last_cleanup: { at: null, expired_removed: 0, used_removed: 0, error: null }
};

const loadFailure: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true,
	layer: 'network'
};

function mockListing(value: LinkPage): void {
	vi.mocked(listLinks).mockResolvedValue({ ok: true, value });
}

function mockNumbers(): void {
	vi.mocked(linkStats).mockResolvedValue({ ok: true, value: linkNumbers });
}

beforeEach(() => {
	invalidateAll();
	vi.clearAllMocks();
	mockNumbers();
});

afterEach(() => {
	cleanup();
});

describe('LinksPlanet', () => {
	it('renders the Sharing heading and the New download link primary action', async () => {
		mockListing(linkListing);
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: 'Sharing' })).toBeTruthy());
		expect(screen.getByRole('button', { name: 'New download link' })).toBeTruthy();
	});

	it('lists each download link with its file path', async () => {
		mockListing(linkListing);
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('report.pdf')).toBeTruthy());
		expect(screen.getByText('team')).toBeTruthy();
		expect(listLinks).toHaveBeenCalledWith({ status: 'active' }, null);
	});

	it('shows the plain error copy when the list cannot load', async () => {
		vi.mocked(listLinks).mockResolvedValue({ ok: false, error: loadFailure });
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('We cannot reach callfs.')).toBeTruthy());
	});

	it('shows the first-use empty state when no link has ever been created', async () => {
		mockListing(emptyListing);
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('No download links yet')).toBeTruthy());
		expect(screen.getByText('Create a link to send one file to someone. It works once.')).toBeTruthy();
	});

	it('shows the view empty state when links exist but this tab has none', async () => {
		mockListing({ ...emptyListing, counts: { active: 0, used: 2, expired: 0, revoked: 0 } });
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('No links here')).toBeTruthy());
	});

	it('shows a warning with retry when the link numbers cannot load', async () => {
		mockListing(linkListing);
		vi.mocked(linkStats).mockResolvedValue({ ok: false, error: loadFailure });
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('We cannot load link numbers.')).toBeTruthy());
		expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy();
	});

	it('hides the admin cleanup action for a member', async () => {
		mockListing(linkListing);
		render(LinksPlanet);
		await waitFor(() => expect(screen.getByText('report.pdf')).toBeTruthy());
		expect(screen.queryByRole('button', { name: 'Clean up old links now' })).toBeNull();
	});
});

describe('LinkDetailPlanet', () => {
	it('shows the invalid link error and never calls the API for a non-numeric id', async () => {
		render(LinkDetailPlanet, { props: { id: 'abc' } });
		await waitFor(() => expect(screen.getByText('This link does not exist.')).toBeTruthy());
		expect(getLink).not.toHaveBeenCalled();
	});

	it('shows the link facts and a timeline of its attempts', async () => {
		vi.mocked(getLink).mockResolvedValue({ ok: true, value: link });
		render(LinkDetailPlanet, { props: { id: '7' } });
		await waitFor(() => expect(screen.getByText('/team/report.pdf')).toBeTruthy());
		expect(screen.getByText('Opened and downloaded')).toBeTruthy();
		expect(screen.getByText('Link created')).toBeTruthy();
		expect(getLink).toHaveBeenCalledWith(7);
	});

	it('shows the partial notice when open attempts are not returned', async () => {
		vi.mocked(getLink).mockResolvedValue({ ok: true, value: { ...link, attempts: undefined } });
		render(LinkDetailPlanet, { props: { id: '7' } });
		await waitFor(() => expect(screen.getByText('Open attempts are not shown yet.')).toBeTruthy());
	});

	it('shows the error state when the link cannot be loaded', async () => {
		vi.mocked(getLink).mockResolvedValue({
			ok: false,
			error: { title: 'This link does not work anymore.', body: 'It was used or it expired. Ask for a new link.', retryable: false }
		});
		render(LinkDetailPlanet, { props: { id: '7' } });
		await waitFor(() => expect(screen.getByText('This link does not work anymore.')).toBeTruthy());
	});

	it('revokes an active link only after the confirmation is accepted', async () => {
		vi.mocked(getLink).mockResolvedValue({ ok: true, value: link });
		vi.mocked(revokeLink).mockResolvedValue({ ok: true, value: { id: 7, status: 'revoked' } });
		render(LinkDetailPlanet, { props: { id: '7' } });
		await waitFor(() => expect(screen.getByRole('button', { name: 'Revoke link' })).toBeTruthy());
		await fireEvent.click(screen.getByRole('button', { name: 'Revoke link' }));
		await waitFor(() => expect(screen.getByText('Revoke this link?')).toBeTruthy());
		expect(revokeLink).not.toHaveBeenCalled();
		await fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Revoke link' }));
		await waitFor(() => expect(revokeLink).toHaveBeenCalledWith(7));
	});
});
