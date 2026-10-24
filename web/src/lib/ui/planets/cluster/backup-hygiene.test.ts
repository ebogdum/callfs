import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import {
	cleanStaging,
	hygiene,
	migrations,
	replication,
	retag,
	retryReplication,
	staging
} from '$lib/api/admin/topology';
import { reassignOwner } from '$lib/api/admin/users';
import { invalidateAll } from '$lib/data/cache';
import type { StagingList } from '$lib/api/admin/topology';
import type { Hygiene, Migrations, Replication } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { formatBytes } from '$lib/format/bytes';
import BackupPlanet from './BackupPlanet.svelte';
import HygienePlanet from './HygienePlanet.svelte';

vi.mock('$lib/api/admin/topology', () => ({
	replication: vi.fn(),
	retryReplication: vi.fn(),
	staging: vi.fn(),
	cleanStaging: vi.fn(),
	hygiene: vi.fn(),
	migrations: vi.fn(),
	retag: vi.fn()
}));

vi.mock('$lib/api/admin/users', () => ({
	reassignOwner: vi.fn()
}));

const HTTP_FAILURE = {
	kind: 'http' as const,
	status: 500,
	code: 'INTERNAL_ERROR',
	message: null,
	details: null,
	requestId: null,
	retryAfterS: null
};

const replicationReport: Replication = {
	enabled: true,
	replica_storage: 's3',
	require_success: true,
	counts_24h: { ok: 12401, failed: 7, skipped: 0 },
	failures: [{ path: '/team/report.pdf', error: 'connection reset', at: '2026-10-22T10:00:00Z' }]
};

const hygieneReport: Hygiene = {
	files_without_owner: 3,
	files_without_etag: 5,
	orphan_pieces: null,
	stale_erasure_records: null,
	staging: { count: 2, bytes: 2048 },
	migrations: { driver: 'none', version: null, dirty: false, latest: null }
};

const stagingList: StagingList = {
	items: [
		{
			storage: 'localfs',
			path: '/.callfs/staging/a.part',
			kind: 'staging',
			bytes: 1024,
			modified: '2026-10-19T10:00:00Z',
			age_s: 259200
		},
		{
			storage: 'localfs',
			path: '/.callfs/staging/b.part',
			kind: 'tmp',
			bytes: 1024,
			modified: '2026-10-20T10:00:00Z',
			age_s: 172800
		}
	],
	total_bytes: 2048
};

const migrationReport: Migrations = {
	driver: 'none',
	current_version: null,
	dirty: false,
	available: []
};

function stagedSize(): string {
	return formatBytes(2048).text.replace(/\u00a0/g, ' ');
}

function mockHygieneSources(): void {
	vi.mocked(hygiene).mockResolvedValue({ ok: true, value: hygieneReport });
	vi.mocked(staging).mockResolvedValue({ ok: true, value: stagingList });
	vi.mocked(migrations).mockResolvedValue({ ok: true, value: migrationReport });
}

beforeEach(() => {
	invalidateAll();
});

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
});

describe('BackupPlanet', () => {
	it('shows the mode, the 24-hour copy counts and the failed copies', async () => {
		vi.mocked(replication).mockResolvedValue({ ok: true, value: replicationReport });
		render(BackupPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('backup.title') })).toBeTruthy();
		expect(await screen.findByText('On, required')).toBeTruthy();
		const made = screen.getByRole('region', { name: t('cluster.backup.kpi.made') });
		expect(within(made).getByText('12,401')).toBeTruthy();
		expect(within(made).getByText('of 12,408')).toBeTruthy();
		const failed = screen.getByRole('region', { name: t('cluster.backup.kpi.failed') });
		expect(within(failed).getByText('7')).toBeTruthy();
		expect(screen.getByLabelText('connection reset')).toBeTruthy();
	});

	it('retries every failed copy when the primary action is used', async () => {
		vi.mocked(replication).mockResolvedValue({ ok: true, value: replicationReport });
		vi.mocked(retryReplication).mockResolvedValue({ ok: true, value: { job_id: 'job-1' } });
		render(BackupPlanet);

		await screen.findByLabelText('connection reset');
		await fireEvent.click(screen.getByRole('button', { name: t('backup.retry.label') }));

		await waitFor(() => {
			expect(retryReplication).toHaveBeenCalledWith(null);
		});
		expect(await screen.findByText(t('backup.retry.started'))).toBeTruthy();
	});

	it('retries one copy from its row', async () => {
		vi.mocked(replication).mockResolvedValue({ ok: true, value: replicationReport });
		vi.mocked(retryReplication).mockResolvedValue({ ok: true, value: { job_id: 'job-2' } });
		render(BackupPlanet);

		await fireEvent.click(await screen.findByRole('button', { name: t('backup.failures.retry') }));

		await waitFor(() => {
			expect(retryReplication).toHaveBeenCalledWith(['/team/report.pdf']);
		});
	});

	it('shows the error state with the action when the report cannot load', async () => {
		vi.mocked(replication).mockResolvedValue({ ok: false, error: HTTP_FAILURE });
		render(BackupPlanet);

		expect(await screen.findByText(/load backup copies/)).toBeTruthy();
		expect(screen.queryByText('On, required')).toBeNull();
	});

	it('explains that backup copy is off when replication is disabled', async () => {
		vi.mocked(replication).mockResolvedValue({
			ok: true,
			value: { ...replicationReport, enabled: false, failures: [] }
		});
		render(BackupPlanet);

		expect(await screen.findByText(t('backup.off.title'))).toBeTruthy();
		expect(screen.queryByRole('table')).toBeNull();
	});

	it('shows the empty copy when no copy failed', async () => {
		vi.mocked(replication).mockResolvedValue({
			ok: true,
			value: { ...replicationReport, failures: [] }
		});
		render(BackupPlanet);

		expect(await screen.findByText(t('backup.failures.empty.title'))).toBeTruthy();
	});
});

describe('HygienePlanet', () => {
	it('shows leftovers, gaps, uncounted pieces and the schema state', async () => {
		mockHygieneSources();
		render(HygienePlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('hygiene.title') })).toBeTruthy();
		expect(await screen.findByText(stagedSize())).toBeTruthy();
		expect(screen.getByText(t('hygiene.orphan.not_counted'))).toBeTruthy();
		expect(screen.getByText(t('hygiene.schema.none'))).toBeTruthy();
	});

	it('cleans up leftover files after the confirmation names the action', async () => {
		mockHygieneSources();
		vi.mocked(cleanStaging).mockResolvedValue({ ok: true, value: { job_id: 'job-3' } });
		render(HygienePlanet);

		await screen.findByText(stagedSize());
		await fireEvent.click(await screen.findByRole('button', { name: t('hygiene.clean.label') }));
		await fireEvent.click(await screen.findByRole('button', { name: t('hygiene.clean.confirm') }));

		await waitFor(() => {
			expect(cleanStaging).toHaveBeenCalledWith(86400);
		});
	});

	it('assigns an owner to files that have none', async () => {
		mockHygieneSources();
		vi.mocked(reassignOwner).mockResolvedValue({ ok: true, value: { job_id: 'job-4' } });
		render(HygienePlanet);

		await fireEvent.click(await screen.findByRole('button', { name: t('hygiene.owner.assign') }));
		await fireEvent.input(screen.getByLabelText(t('hygiene.owner.field.label')), {
			target: { value: 'alice' }
		});
		await fireEvent.click(screen.getByRole('button', { name: t('hygiene.owner.confirm') }));

		await waitFor(() => {
			expect(reassignOwner).toHaveBeenCalledWith('', 'alice', '/');
		});
	});

	it('adds version tags from the root of the file index', async () => {
		mockHygieneSources();
		vi.mocked(retag).mockResolvedValue({ ok: true, value: { job_id: 'job-5' } });
		render(HygienePlanet);

		await fireEvent.click(await screen.findByRole('button', { name: t('hygiene.tag.run') }));

		await waitFor(() => {
			expect(retag).toHaveBeenCalledWith('/');
		});
	});
});
