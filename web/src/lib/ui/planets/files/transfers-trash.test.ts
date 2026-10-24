import { t } from '$lib/copy/t';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { TrashItem } from '$lib/api/types';
import type { Transfer } from '$lib/stores/transfers.svelte';
import TransfersPlanet from './TransfersPlanet.svelte';
import TrashPlanet from './TrashPlanet.svelte';

const mocks = vi.hoisted(() => ({
	listTrash: vi.fn(),
	restore: vi.fn(),
	deleteTrashItem: vi.fn(),
	emptyTrash: vi.fn(),
	stat: vi.fn(),
	serverTransfers: vi.fn(),
	goto: vi.fn(),
	store: {
		items: [] as Transfer[],
		resumable: [] as { uploadId: string; name: string; size: number }[],
		resolveConflict: vi.fn(),
		resumeFrom: vi.fn(),
		discardResumable: vi.fn(),
		settings: { parallel: 3, partBytes: 8388608 },
		pause: vi.fn(),
		resume: vi.fn(),
		retry: vi.fn(),
		retryFailed: vi.fn(),
		cancel: vi.fn(),
		setParallel: vi.fn(),
		setPartBytes: vi.fn()
	},
	session: { isAdmin: false }
}));

vi.mock('$lib/api/trash', () => ({
	listTrash: mocks.listTrash,
	restore: mocks.restore,
	deleteTrashItem: mocks.deleteTrashItem,
	emptyTrash: mocks.emptyTrash
}));
vi.mock('$lib/api/files', () => ({ stat: mocks.stat }));
vi.mock('$lib/api/admin/telemetry', () => ({ transfers: mocks.serverTransfers }));
vi.mock('$lib/stores/transfers.svelte', () => ({ transfers: mocks.store }));
vi.mock('$lib/stores/session.svelte', () => ({ session: mocks.session }));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));

const trashItem = (overrides: Partial<TrashItem> = {}): TrashItem => ({
	id: 'trash-1',
	original_path: '/reports/report.pdf',
	name: 'report.pdf',
	type: 'file',
	size: 2048,
	deleted_by: 'alice',
	deleted_at: '2026-10-20T09:00:00Z',
	expires_at: '2026-11-19T09:00:00Z',
	...overrides
});

const transfer = (overrides: Partial<Transfer> = {}): Transfer => ({
	id: 't1',
	direction: 'upload',
	name: 'video.mp4',
	destination: '/media/video.mp4',
	state: 'uploading',
	done: 5,
	total: 10,
	partsDone: 1,
	partsTotal: 2,
	bytesPerSecond: 1000,
	eta: 30,
	resumeId: null,
	resumedAt: null,
	existing: null,
	conflictPolicy: null,
	error: null,
	...overrides
});

const trashPage = (items: TrashItem[]) => ({ ok: true, value: { items, next_cursor: null, total_bytes: 0 } });

const primaryButtons = (container: HTMLElement): number => container.querySelectorAll('.button[data-variant="primary"]').length;

beforeEach(() => {
	vi.clearAllMocks();
	mocks.store.items = [];
	mocks.session.isAdmin = false;
	mocks.listTrash.mockResolvedValue(trashPage([]));
	mocks.restore.mockResolvedValue({ ok: true, value: { path: '/reports/report.pdf' } });
	mocks.deleteTrashItem.mockResolvedValue({ ok: true, value: undefined });
	mocks.emptyTrash.mockResolvedValue({ ok: true, value: { job_id: 'job-1' } });
	mocks.serverTransfers.mockResolvedValue({ ok: true, value: { items: [] } });
});

afterEach(() => {
	cleanup();
});

describe('TrashPlanet', () => {
	it('renders the Trash heading and the 30-day notice', async () => {
		render(TrashPlanet);
		expect(screen.getByRole('heading', { level: 1, name: 'Trash' })).toBeTruthy();
		expect(screen.getByText('Files stay in trash for 30 days. Then they are deleted for good on the date shown.')).toBeTruthy();
		await waitFor(() => expect(mocks.listTrash).toHaveBeenCalledWith(null));
	});

	it('shows the empty copy when nothing is in trash', async () => {
		render(TrashPlanet);
		await waitFor(() => expect(screen.getByText('Trash is empty')).toBeTruthy());
		expect(screen.getByText('Deleted files show here for 30 days.')).toBeTruthy();
	});

	it('shows the error state with a retry action when the list cannot load', async () => {
		mocks.listTrash.mockResolvedValue({
			ok: false,
			error: {
				title: 'We cannot reach callfs.',
				body: 'Check your connection. We will try again.',
				retryable: true,
				layer: 'network'
			}
		});
		render(TrashPlanet);
		await waitFor(() => expect(screen.getByText('We cannot reach callfs.')).toBeTruthy());
		expect(screen.getByRole('button', { name: /retry/i })).toBeTruthy();
	});

	it('lists deleted files with their original path', async () => {
		mocks.listTrash.mockResolvedValue(trashPage([trashItem()]));
		render(TrashPlanet);
		await waitFor(() => expect(screen.getByTitle('report.pdf')).toBeTruthy());
		expect(screen.getByTitle('/reports/report.pdf')).toBeTruthy();
	});

	it('exposes exactly one primary action in the success state', async () => {
		mocks.listTrash.mockResolvedValue(trashPage([trashItem()]));
		const { container } = render(TrashPlanet);
		await waitFor(() => expect(screen.getByTitle('report.pdf')).toBeTruthy());
		await fireEvent.click(screen.getByRole('checkbox', { name: /report\.pdf/ }));
		expect(primaryButtons(container)).toBe(1);
	});

	it('restores the selected file and reloads the list', async () => {
		mocks.listTrash.mockResolvedValue(trashPage([trashItem()]));
		render(TrashPlanet);
		await waitFor(() => expect(screen.getByTitle('report.pdf')).toBeTruthy());
		await fireEvent.click(screen.getByRole('checkbox', { name: /report\.pdf/ }));
		await fireEvent.click(screen.getByRole('button', { name: t('transfers-trash.trash.action.restore_count', { count: 1 }) }));
		await waitFor(() => expect(mocks.restore).toHaveBeenCalledWith('trash-1'));
		expect(mocks.listTrash).toHaveBeenCalledTimes(2);
	});

	it('asks before emptying trash and empties it on confirmation', async () => {
		mocks.listTrash.mockResolvedValue(trashPage([trashItem()]));
		render(TrashPlanet);
		await waitFor(() => expect(screen.getByTitle('report.pdf')).toBeTruthy());
		await fireEvent.click(screen.getByRole('button', { name: 'Empty trash' }));
		await waitFor(() => expect(screen.getByText('Every item in trash is deleted for good. This cannot be undone.')).toBeTruthy());
		expect(mocks.emptyTrash).not.toHaveBeenCalled();
		const confirmButtons = screen.getAllByRole('button', { name: 'Empty trash' });
		await fireEvent.click(confirmButtons[confirmButtons.length - 1]);
		await waitFor(() => expect(mocks.emptyTrash).toHaveBeenCalledTimes(1));
	});
});

describe('TransfersPlanet', () => {
	it('renders the Transfers heading and the empty copy when no transfers exist', async () => {
		render(TransfersPlanet);
		expect(screen.getByRole('heading', { level: 1, name: 'Transfers' })).toBeTruthy();
		expect(screen.getByText('No transfers yet')).toBeTruthy();
		expect(screen.getByText('Uploads and downloads show here while they run.')).toBeTruthy();
	});

	it('lists running transfers in the Active tab', async () => {
		mocks.store.items = [transfer()];
		render(TransfersPlanet);
		expect(screen.getByTitle('video.mp4')).toBeTruthy();
		expect(screen.getByTitle('/media/video.mp4')).toBeTruthy();
	});

	it('offers Retry failed as the only primary action when transfers failed', async () => {
		mocks.store.items = [transfer({ id: 't2', state: 'failed', error: null })];
		const { container } = render(TransfersPlanet);
		expect(primaryButtons(container)).toBe(1);
		await fireEvent.click(screen.getByRole('button', { name: 'Retry failed' }));
		expect(mocks.store.retryFailed).toHaveBeenCalledTimes(1);
	});

	it('exposes no primary action when no transfer failed', async () => {
		mocks.store.items = [transfer()];
		const { container } = render(TransfersPlanet);
		expect(primaryButtons(container)).toBe(0);
	});

	it('hides the Server tab from members', async () => {
		render(TransfersPlanet);
		expect(screen.queryByRole('tab', { name: /Server/ })).toBeNull();
	});

	it('lists every user transfer on the Server tab for admins', async () => {
		mocks.session.isAdmin = true;
		mocks.serverTransfers.mockResolvedValue({
			ok: true,
			value: {
				items: [
					{
						id: 's1',
						user_id: 'alice',
						kind: 'upload',
						path: '/archive/backup.tar',
						bytes_done: 1,
						bytes_total: 2,
						started_at: '2026-10-23T10:00:00Z',
						client: 'curl'
					}
				]
			}
		});
		render(TransfersPlanet);
		await fireEvent.click(screen.getByRole('tab', { name: /Server/ }));
		await waitFor(() => expect(screen.getByTitle('alice')).toBeTruthy());
		expect(mocks.serverTransfers).toHaveBeenCalledTimes(1);
	});

	it('shows the error state with a retry action when the Server tab cannot load', async () => {
		mocks.session.isAdmin = true;
		mocks.serverTransfers.mockResolvedValue({ ok: false, error: { kind: 'network' } });
		render(TransfersPlanet);
		await fireEvent.click(screen.getByRole('tab', { name: /Server/ }));
		await waitFor(() => expect(screen.getByRole('button', { name: /retry/i })).toBeTruthy());
	});
});
