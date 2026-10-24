import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Capabilities, Me, ServerInfo } from '$lib/api/types';
import { t } from '$lib/copy/t';
import DestinationPicker from './DestinationPicker.svelte';
import type { MoveOptions } from './DestinationPicker.svelte';
import PermissionInspector from './PermissionInspector.svelte';
import type { PermissionCheck } from './PermissionInspector.svelte';
import ShareLinkForm from './ShareLinkForm.svelte';

afterEach(() => {
	cleanup();
});

const capabilities = { features: { cross_server: true } } as Capabilities;

const servers: ServerInfo[] = [
	{
		instance_id: 'callfs-2',
		endpoint: 'https://callfs-2.internal',
		source: 'joined',
		is_self: false,
		scheme: 'https',
		reachable: true,
		rtt_ms: 4,
		checked_at: null,
		files: 0,
		bytes: 0,
		pieces: 0
	}
];

const me: Me = {
	user_id: 'alice',
	is_admin: false,
	auth_method: 'session',
	session_expires_at: null,
	csrf_token: null,
	permissions_model: 'owner'
};

const moveOptions = (overrides: Partial<MoveOptions> = {}): MoveOptions => ({
	overwrite: false,
	createParents: false,
	...overrides
});

const pickerProps = (overrides: Record<string, unknown> = {}) => ({
	start: '/',
	value: '/archive',
	recents: [] as string[],
	pinned: [] as string[],
	load: vi.fn(() => Promise.resolve({ ok: true as const, value: [] })),
	storages: ['localfs', 's3'],
	servers,
	options: moveOptions(),
	capabilities,
	count: 3,
	bytes: null,
	folder: false,
	...overrides
});

describe('DestinationPicker', () => {
	it('names the region and labels the destination path field', () => {
		render(DestinationPicker, { props: pickerProps() });
		expect(screen.getByRole('region', { name: t('fileforms.move.region') })).toBeInTheDocument();
		expect(screen.getByLabelText(t('fileforms.move.destination'))).toHaveValue('/archive');
	});

	it('previews the move of files to the destination', () => {
		render(DestinationPicker, { props: pickerProps() });
		expect(
			screen.getByText(t('fileforms.move.preview', { count: 3, destination: '/archive' }))
		).toBeInTheDocument();
	});

	it('disables storage and server for folders and explains why', () => {
		render(DestinationPicker, { props: pickerProps({ folder: true }) });
		expect(screen.getByLabelText(t('fileforms.move.storage'))).toBeDisabled();
		expect(screen.getByLabelText(t('fileforms.move.server'))).toBeDisabled();
		expect(screen.getByText(t('fileforms.move.folder_locked'))).toBeInTheDocument();
	});

	it('states the copy step when the storage is changed', () => {
		render(DestinationPicker, { props: pickerProps({ options: moveOptions({ storage: 's3' }) }) });
		expect(
			screen.getByText(t('fileforms.move.preview_cross', { storage: t('fileforms.storage.s3') }), { exact: false })
		).toBeInTheDocument();
	});

	it('keeps overwrite and create missing folders off and writes toggles to the options', async () => {
		const options = moveOptions();
		render(DestinationPicker, { props: pickerProps({ options }) });
		const overwrite = screen.getByRole('switch', { name: t('fileforms.move.overwrite') });
		const createParents = screen.getByRole('switch', { name: t('fileforms.move.create_parents') });
		expect(overwrite).toHaveAttribute('aria-checked', 'false');
		expect(createParents).toHaveAttribute('aria-checked', 'false');
		await fireEvent.click(overwrite);
		expect(options.overwrite).toBe(true);
	});

	it('sets the destination when a recent path is chosen', async () => {
		render(DestinationPicker, { props: pickerProps({ recents: ['/team'] }) });
		await fireEvent.click(screen.getByRole('button', { name: '/team' }));
		expect(screen.getByLabelText(t('fileforms.move.destination'))).toHaveValue('/team');
	});
});

describe('ShareLinkForm', () => {
	const shareProps = (overrides: Record<string, unknown> = {}) => ({
		path: '/team/report.pdf',
		size: 1024,
		maxSeconds: 86400,
		value: 3600,
		result: null,
		busy: false,
		ongenerate: vi.fn(),
		...overrides
	});

	it('names the region and selects the recommended one hour preset', () => {
		render(ShareLinkForm, { props: shareProps() });
		expect(screen.getByRole('region', { name: t('fileforms.share.region') })).toBeInTheDocument();
		expect(screen.getByRole('radio', { name: /^1 hour/ })).toBeChecked();
	});

	it('lists the facts before the link is created', () => {
		render(ShareLinkForm, { props: shareProps() });
		expect(screen.getByText(t('fileforms.share.fact_once'))).toBeInTheDocument();
		expect(
			screen.getByText(t('fileforms.share.fact_anyone', { name: 'report.pdf' }))
		).toBeInTheDocument();
		expect(screen.getByText(t('fileforms.share.fact_opening'))).toBeInTheDocument();
	});

	it('blocks link creation for folders with a plain reason', () => {
		render(ShareLinkForm, { props: shareProps({ path: '/team/' }) });
		const generate = screen.getByRole('button', { name: t('fileforms.share.generate') });
		expect(generate).toHaveAttribute('aria-disabled', 'true');
		expect(screen.getAllByText(t('fileforms.share.folder_blocked')).length).toBeGreaterThan(0);
	});

	it('calls ongenerate when the link is created', async () => {
		const ongenerate = vi.fn();
		render(ShareLinkForm, { props: shareProps({ ongenerate }) });
		await fireEvent.click(screen.getByRole('button', { name: t('fileforms.share.generate') }));
		expect(ongenerate).toHaveBeenCalledTimes(1);
	});

	it('shows the created link with a curl copy action', () => {
		const url = 'https://callfs.example/download/abc';
		render(ShareLinkForm, {
			props: shareProps({ result: { url, expiresAt: '2099-01-01T00:00:00.000Z' } })
		});
		expect(screen.getByLabelText(t('fileforms.share.url'))).toHaveValue(url);
		expect(screen.getByRole('button', { name: t('fileforms.share.copy_curl') })).toBeInTheDocument();
	});
});

describe('PermissionInspector', () => {
	const checks: PermissionCheck[] = [
		{ perm: 'read', allowed: true, reason: 'owner', owner: 'alice' },
		{ perm: 'write', allowed: true, reason: 'owner', owner: 'alice' },
		{ perm: 'delete', allowed: false, reason: 'not_owner', owner: 'alice' }
	];

	it('names the region by the path and shows the signed in user', () => {
		render(PermissionInspector, { props: { path: '/team/a.txt', checks, me } });
		expect(
			screen.getByRole('region', { name: t('fileforms.permissions.region', { path: '/team/a.txt' }) })
		).toBeInTheDocument();
		expect(screen.getByText(t('fileforms.permissions.signed_in', { user: 'alice' }))).toBeInTheDocument();
	});

	it('renders one row per check in order', () => {
		render(PermissionInspector, { props: { path: '/team/a.txt', checks, me } });
		expect(screen.getAllByRole('listitem')).toHaveLength(3);
	});

	it('shows allowed and not allowed words beside each row', () => {
		render(PermissionInspector, { props: { path: '/team/a.txt', checks, me } });
		expect(screen.getAllByText(t('fileforms.permissions.allowed'))).toHaveLength(2);
		expect(screen.getAllByText(t('fileforms.permissions.denied'))).toHaveLength(1);
	});

	it('explains the delete rule in plain words with the owner', () => {
		render(PermissionInspector, { props: { path: '/team/a.txt', checks, me } });
		expect(
			screen.getByText(
				t('fileforms.permissions.rule.not_owner', {
					action: t('fileforms.permissions.action.delete'),
					owner: 'alice'
				})
			)
		).toBeInTheDocument();
	});
});
