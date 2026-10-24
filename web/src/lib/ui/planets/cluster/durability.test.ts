import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { erasurePlan, erasureSummary, verifyAll } from '$lib/api/admin/topology';
import { httpFailure } from '$lib/api/errors';
import { getSplitFile, listSplitFiles, manifest, repair, verify } from '$lib/api/erasure';
import type { ErasureSummary, SplitFile, SplitFileDetail, VerifyReport } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';
import { t } from '$lib/copy/t';
import { clearCache } from '$lib/data/cache';
import { jobs } from '$lib/stores/jobs.svelte';
import DurabilityPlanet from './DurabilityPlanet.svelte';
import SplitFilePlanet from './SplitFilePlanet.svelte';

vi.mock('$lib/api/admin/topology', () => ({
	erasureSummary: vi.fn(),
	erasurePlan: vi.fn(),
	verifyAll: vi.fn()
}));
vi.mock('$lib/api/erasure', () => ({
	listSplitFiles: vi.fn(),
	getSplitFile: vi.fn(),
	manifest: vi.fn(),
	verify: vi.fn(),
	repair: vi.fn(),
	pieceUrl: (path: string, index: number) => `/v1/shards${path}/${index}`
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({
	page: { url: new URL('https://console.test/cluster/durability'), params: { path: 'docs/report.pdf' } }
}));
vi.mock('$lib/stores/jobs.svelte', () => ({ jobs: { watch: vi.fn() } }));

type Shard = SplitFileDetail['shards'][number];

const UI_ERROR: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true
};

function summaryWith(health: Partial<ErasureSummary['health']>): ErasureSummary {
	return {
		enabled: true,
		data_shards: 4,
		parity_shards: 2,
		min_file_size: 0,
		shard_storage: 'localfs',
		shard_path: '',
		placement: 'round_robin',
		instances: ['node-a', 'node-b'],
		files: 12,
		bytes: 4096,
		raw_bytes: 6144,
		health: { ok: 11, reduced: 1, critical: 0, unreadable: 0, unknown: 0, ...health },
		pieces_by_instance: { 'node-a': 36, 'node-b': 36 }
	};
}

function splitFile(overrides: Partial<SplitFile> = {}): SplitFile {
	return {
		path: '/docs/report.pdf',
		original_size: 2048,
		data_shards: 4,
		parity_shards: 2,
		shard_size: 512,
		shards_total: 6,
		healthy_shards: 6,
		last_checked_at: '2026-10-22T10:00:00Z',
		health: 'ok',
		...overrides
	};
}

function shard(index: number, state: Shard['state'] = 'ok'): Shard {
	return {
		index,
		kind: 4 > index ? 'data' : 'parity',
		instance_id: 0 === index % 2 ? 'node-a' : 'node-b',
		storage: 'localfs',
		path: `/shards/${index}`,
		size: 512,
		checksum: `sum-${index}`,
		state
	};
}

function detail(missingIndex: number | null): SplitFileDetail {
	return {
		path: '/docs/report.pdf',
		original_size: 2048,
		profile: { data_shards: 4, parity_shards: 2, shard_size: 512 },
		shards: Array.from({ length: 6 }, (_, index) => shard(index, index === missingIndex ? 'missing' : 'ok')),
		tolerance: 2,
		last_checked_at: '2026-10-22T10:00:00Z'
	};
}

function mockSummary(summary: ErasureSummary): void {
	vi.mocked(erasureSummary).mockResolvedValue({ ok: true, value: summary });
}

function mockFiles(items: SplitFile[]): void {
	vi.mocked(listSplitFiles).mockResolvedValue({ ok: true, value: { items, next_cursor: null } });
}

function primaryButtons(): HTMLButtonElement[] {
	return Array.from(document.querySelectorAll<HTMLButtonElement>('button[data-variant="primary"]'));
}

beforeEach(() => {
	clearCache();
	vi.mocked(erasurePlan).mockResolvedValue({
		ok: true,
		value: {
			total_shards: 6,
			assignments: [],
			per_instance: { 'node-a': 3, 'node-b': 3 },
			max_on_one_instance: 3,
			tolerated_instance_failures: 0,
			tolerated_piece_losses: 2,
			overhead_ratio: 1.5,
			fault_tolerant: true,
			warnings: []
		}
	});
	vi.mocked(verifyAll).mockResolvedValue({ ok: true, value: { job_id: 'job-verify' } });
	vi.mocked(repair).mockResolvedValue({ ok: true, value: { job_id: 'job-repair' } });
	vi.mocked(verify).mockResolvedValue({ ok: true, value: report() });
	vi.mocked(getSplitFile).mockResolvedValue({ ok: true, value: detail(null) });
	vi.mocked(manifest).mockResolvedValue({
		ok: true,
		value: {
			path: '/docs/report.pdf',
			original_size: 2048,
			erasure_profile: { data_shards: 4, parity_shards: 2, shard_size: 512 },
			shards: []
		}
	});
});

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
});

function report(): VerifyReport {
	return {
		path: '/docs/report.pdf',
		readable: true,
		healthy: 6,
		required: 4,
		shards: Array.from({ length: 6 }, (_, index) => ({ index, state: 'ok', error: null }))
	};
}

describe('DurabilityPlanet', () => {
	it('renders the title as the only h1 and one primary action in the success state', async () => {
		mockSummary(summaryWith({}));
		mockFiles([splitFile()]);
		render(DurabilityPlanet);
		expect(await screen.findByRole('heading', { level: 1, name: t('durability.title') })).toBeTruthy();
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		expect(await screen.findByRole('region', { name: t('durability.kpi.full') })).toBeTruthy();
		expect(primaryButtons()).toHaveLength(1);
		expect(primaryButtons()[0].textContent).toContain(t('durability.primary.repair'));
	});

	it('shows the loading state without a table while the summary is pending', () => {
		vi.mocked(erasureSummary).mockReturnValue(new Promise(() => undefined));
		mockFiles([]);
		render(DurabilityPlanet);
		expect(screen.getByRole('heading', { level: 1, name: t('durability.title') })).toBeTruthy();
		expect(screen.queryByRole('region', { name: t('durability.kpi.full') })).toBeNull();
	});

	it('shows the not-enabled state when split storage is off', async () => {
		mockSummary({ ...summaryWith({}), enabled: false });
		render(DurabilityPlanet);
		expect(await screen.findByText(t('durability.off.title'))).toBeTruthy();
		expect(screen.getByText(t('durability.off.body'))).toBeTruthy();
	});

	it('shows the error state with plain copy when the summary fails', async () => {
		vi.mocked(erasureSummary).mockResolvedValue({ ok: false, error: httpFailure(500, '', null, 'req-1') });
		mockFiles([]);
		render(DurabilityPlanet);
		expect(await screen.findByText(/load split files/)).toBeTruthy();
	});

	it('shows the empty state when no split files exist', async () => {
		mockSummary(summaryWith({ ok: 0, reduced: 0 }));
		mockFiles([]);
		render(DurabilityPlanet);
		expect(await screen.findByText(t('durability.table.empty.title'))).toBeTruthy();
	});

	it('shows the partial alert when a file has no piece check yet', async () => {
		mockSummary(summaryWith({}));
		mockFiles([splitFile({ healthy_shards: null, health: 'unknown' })]);
		render(DurabilityPlanet);
		expect(await screen.findByText(t('durability.partial.title'))).toBeTruthy();
		expect(screen.getAllByText(t('durability.pieces.unchecked')).length).toBeGreaterThan(0);
	});

	it('runs a cluster-wide verification job from the verify primary action', async () => {
		mockSummary(summaryWith({ ok: 12, reduced: 0 }));
		mockFiles([splitFile()]);
		render(DurabilityPlanet);
		await screen.findByRole('region', { name: t('durability.kpi.full') });
		await waitFor(() => expect(primaryButtons()).toHaveLength(1));
		expect(primaryButtons()[0].textContent).toContain(t('durability.primary.verify'));
		await fireEvent.click(primaryButtons()[0]);
		await waitFor(() => expect(verifyAll).toHaveBeenCalledWith('/'));
		await waitFor(() => expect(jobs.watch).toHaveBeenCalledWith('job-verify'));
	});

	it('starts a repair for each damaged row from the repair primary action', async () => {
		mockSummary(summaryWith({}));
		mockFiles([splitFile({ health: 'reduced', healthy_shards: 5 })]);
		render(DurabilityPlanet);
		await screen.findByRole('region', { name: t('durability.kpi.full') });
		await waitFor(() => expect(primaryButtons()).toHaveLength(1));
		await fireEvent.click(primaryButtons()[0]);
		await waitFor(() => expect(repair).toHaveBeenCalledWith('/docs/report.pdf'));
		await waitFor(() => expect(jobs.watch).toHaveBeenCalledWith('job-repair'));
	});

	it('opens the file detail route when a row is opened', async () => {
		mockSummary(summaryWith({}));
		mockFiles([splitFile()]);
		render(DurabilityPlanet);
		await screen.findByRole('region', { name: t('durability.kpi.full') });
		const link = await screen.findByRole('link', { name: /report\.pdf/ });
		expect(link.getAttribute('href')).toBe('/ui/cluster/durability/file/docs/report.pdf');
	});
});

describe('SplitFilePlanet', () => {
	it('renders the file name as the only h1 and one primary action', async () => {
		vi.mocked(getSplitFile).mockResolvedValue({ ok: true, value: detail(5) });
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		expect(await screen.findByRole('heading', { level: 1, name: 'report.pdf' })).toBeTruthy();
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		expect(primaryButtons()).toHaveLength(1);
		expect(primaryButtons()[0].textContent).toContain(t('durability.file.primary.repair'));
	});

	it('shows the loading state without the piece table while the file is pending', () => {
		vi.mocked(getSplitFile).mockReturnValue(new Promise(() => undefined));
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		expect(screen.getByRole('heading', { level: 1, name: 'report.pdf' })).toBeTruthy();
		expect(screen.queryByRole('table')).toBeNull();
	});

	it('shows the error state with plain copy when the file cannot be loaded', async () => {
		vi.mocked(getSplitFile).mockResolvedValue({ ok: false, error: UI_ERROR });
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		expect(await screen.findByText(UI_ERROR.title)).toBeTruthy();
	});

	it('shows the verify result after a dry-run verification', async () => {
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		await screen.findByRole('heading', { level: 1, name: 'report.pdf' });
		await waitFor(() => expect(primaryButtons()[0].textContent).toContain(t('durability.file.primary.verify')));
		await fireEvent.click(primaryButtons()[0]);
		await waitFor(() => expect(verify).toHaveBeenCalledWith('/docs/report.pdf'));
		expect(await screen.findByText(t('durability.verify.ok', { total: '6' }))).toBeTruthy();
	});

	it('starts a repair of the pieces when a piece is missing', async () => {
		vi.mocked(getSplitFile).mockResolvedValue({ ok: true, value: detail(5) });
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		await screen.findByRole('heading', { level: 1, name: 'report.pdf' });
		await waitFor(() => expect(primaryButtons()).toHaveLength(1));
		await fireEvent.click(primaryButtons()[0]);
		await waitFor(() => expect(repair).toHaveBeenCalledWith('/docs/report.pdf'));
		await waitFor(() => expect(jobs.watch).toHaveBeenCalledWith('job-repair'));
	});

	it('lists every piece and the manifest section once loaded', async () => {
		render(SplitFilePlanet, { props: { path: 'docs/report.pdf' } });
		expect(await screen.findByRole('heading', { level: 2, name: t('durability.section.pieces') })).toBeTruthy();
		expect(await screen.findByRole('heading', { level: 2, name: t('durability.section.manifest') })).toBeTruthy();
		expect(screen.getByText(t('durability.piece.download', { index: '0' }))).toBeTruthy();
	});
});
