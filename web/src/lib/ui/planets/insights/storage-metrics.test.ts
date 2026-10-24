import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { base } from '$app/paths';
import { t } from '$lib/copy/t';
import { createQuery } from '$lib/data/query.svelte';
import MetricsCatalogPlanet from './MetricsCatalogPlanet.svelte';
import StorageInsightsPlanet from './StorageInsightsPlanet.svelte';

vi.mock('$lib/data/query.svelte', () => ({ createQuery: vi.fn() }));

const UPDATED_AT = 1_800_000_000_000;

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

const useQueries = (byKey: Record<string, Record<string, unknown>>): void => {
	const create = (source: unknown) => {
		const options = 'function' === typeof source ? source() : source;
		return query(byKey[options.key[0]] ?? {});
	};
	vi.mocked(createQuery).mockImplementation(create as unknown as typeof createQuery);
};

const usageReady = {
	path: '/',
	files: 1204,
	directories: 40,
	bytes: 5_000_000_000,
	by_storage: {
		localfs: { files: 1000, bytes: 4_000_000_000 },
		s3: { files: 204, bytes: 1_000_000_000 }
	},
	by_instance: { 'callfs-1': { files: 1204, bytes: 5_000_000_000 } },
	by_owner: {},
	erasure_files: 3,
	erasure_raw_bytes: 0,
	children: [
		{ path: '/team/', files: 900, bytes: 4_000_000_000 },
		{ path: '/shared/', files: 304, bytes: 1_000_000_000 }
	],
	size_histogram: [{ le: 1024, count: 10 }],
	age_histogram: [{ le_days: 1, count: 4 }],
	computed_at: '2026-10-23T10:00:00Z',
	scan_truncated: false
};

const usageError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true
};

const owners = [
	{ owner: 'alice', files: 3, bytes: 300 },
	{ owner: '', files: 1, bytes: 10 }
];

const families = [
	{
		name: 'callfs_http_requests_total',
		type: 'counter',
		help: 'Requests.',
		emitted: true,
		series: [
			{ labels: { method: 'GET' }, value: 12 },
			{ labels: { method: 'PUT' }, value: 3 }
		]
	},
	{ name: 'callfs_active_locks', type: 'gauge', help: 'Locks.', emitted: false, series: [] }
];

describe('StorageInsightsPlanet', () => {
	beforeEach(() => {
		vi.mocked(createQuery).mockReset();
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the page heading, the Recalculate action and the folder chart rows', () => {
		useQueries({
			usage: query({ data: { kind: 'ready', usage: usageReady }, status: 'success' }),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: owners, status: 'success' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('storage-insights.title') })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('storage-insights.recalculate') })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: t('storage-insights.folders.title') })).toBeInTheDocument();
		expect(screen.getAllByText('/team/').length).toBeGreaterThan(0);
	});

	it('shows storage, server and owner breakdowns with the plain no-owner label', () => {
		useQueries({
			usage: query({ data: { kind: 'ready', usage: usageReady }, status: 'success' }),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: owners, status: 'success' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getAllByText(t('storage-insights.storage.localfs')).length).toBeGreaterThan(0);
		expect(screen.getAllByText(t('storage-insights.storage.s3')).length).toBeGreaterThan(0);
		expect(screen.getAllByText('callfs-1').length).toBeGreaterThan(0);
		expect(screen.getAllByText('alice').length).toBeGreaterThan(0);
		expect(screen.getAllByText(t('storage-insights.owner.empty')).length).toBeGreaterThan(0);
	});

	it('calls the usage refetch when Recalculate is pressed', async () => {
		const refetch = vi.fn();
		useQueries({
			usage: query({ data: { kind: 'ready', usage: usageReady }, status: 'success', refetch }),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: [], status: 'empty' })
		});
		render(StorageInsightsPlanet);

		await fireEvent.click(screen.getByRole('button', { name: t('storage-insights.recalculate') }));

		expect(refetch).toHaveBeenCalledTimes(1);
	});

	it('shows the first-use empty state when no files exist', () => {
		useQueries({
			usage: query({
				data: { kind: 'ready', usage: { ...usageReady, files: 0, directories: 0 } },
				status: 'empty'
			}),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: [], status: 'empty' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByText(t('storage-insights.empty.title'))).toBeInTheDocument();
		expect(screen.getByText(t('storage-insights.empty.body'))).toBeInTheDocument();
		expect(screen.queryByRole('heading', { name: t('storage-insights.folders.title') })).toBeNull();
	});

	it('shows the region error with plain copy when no usage data exists', () => {
		useQueries({
			usage: query({ data: undefined, status: 'error', error: usageError }),
			series: query({ data: undefined, status: 'loading' }),
			owners: query({ data: undefined, status: 'loading' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByText(usageError.title)).toBeInTheDocument();
		expect(screen.getByText(usageError.body)).toBeInTheDocument();
	});

	it('shows a link to the running job when the count is still being calculated', () => {
		useQueries({
			usage: query({ data: { kind: 'job', jobId: 'job-7' }, status: 'success' }),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: [], status: 'empty' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByText(t('storage-insights.job_running.title'))).toBeInTheDocument();
		expect(screen.getByRole('link', { name: t('storage-insights.job_running.open') })).toHaveAttribute(
			'href',
			`${base}/activity/jobs/job-7`
		);
	});

	it('warns that numbers may be low when the scan stopped early', () => {
		useQueries({
			usage: query({
				data: { kind: 'ready', usage: { ...usageReady, scan_truncated: true } },
				status: 'partial'
			}),
			series: query({ data: { series: [] }, status: 'empty' }),
			owners: query({ data: [], status: 'empty' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByText(t('storage-insights.partial'))).toBeInTheDocument();
	});

	it('renders growth series from the usage_bytes metric', () => {
		useQueries({
			usage: query({ data: { kind: 'ready', usage: usageReady }, status: 'success' }),
			series: query({
				data: {
					name: 'usage_bytes',
					measure: 'value',
					unit: 'bytes',
					step_s: 3600,
					sampled_since: null,
					series: [{ labels: { storage: 'localfs' }, points: [[1, 100], [2, 200]] }]
				},
				status: 'success'
			}),
			owners: query({ data: [], status: 'empty' })
		});
		render(StorageInsightsPlanet);

		expect(screen.getByRole('heading', { name: t('storage-insights.growth.title') })).toBeInTheDocument();
	});
});

describe('MetricsCatalogPlanet', () => {
	beforeEach(() => {
		vi.mocked(createQuery).mockReset();
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the page heading and the scrape guidance with the exposure warning', () => {
		useQueries({ 'metrics-catalog': query({ data: { generated_at: '', families: [] }, status: 'success' }) });
		render(MetricsCatalogPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('storage-insights.metrics.title') })).toBeInTheDocument();
		expect(screen.getByRole('heading', { name: t('insights.metrics.scrape.title') })).toBeInTheDocument();
		expect(screen.getByText(t('storage-insights.metrics.exposure.title'))).toBeInTheDocument();
		expect(screen.getByText('/metrics')).toBeInTheDocument();
	});

	it('lists every metric family with its plain meaning and reported status', () => {
		useQueries({ 'metrics-catalog': query({ data: { generated_at: '', families }, status: 'success' }) });
		render(MetricsCatalogPlanet);

		expect(screen.getByText('callfs_http_requests_total')).toBeInTheDocument();
		expect(screen.getByText('callfs_active_locks')).toBeInTheDocument();
		expect(screen.getByText('callfs_errors_total')).toBeInTheDocument();
		expect(screen.getByText(t('storage-insights.metric.http_requests.meaning'))).toBeInTheDocument();
		expect(screen.getAllByText(t('status.metric.reported')).length).toBeGreaterThan(0);
		expect(screen.getAllByText(t('status.metric.not_reported')).length).toBeGreaterThan(0);
		expect(screen.getByText('15')).toBeInTheDocument();
	});

	it('links each named metric to the explore page', () => {
		useQueries({ 'metrics-catalog': query({ data: { generated_at: '', families }, status: 'success' }) });
		render(MetricsCatalogPlanet);

		const link = document.querySelector(`a[href="${base}/insights/explore?metric=callfs_http_requests_total"]`);
		expect(link).not.toBeNull();
		expect(link?.textContent).toBe('callfs_http_requests_total');
		expect(link).toHaveAttribute('title', t('storage-insights.metrics.column.explore'));
	});

	it('shows the catalog error inside the table with plain copy', () => {
		useQueries({ 'metrics-catalog': query({ data: undefined, status: 'error', error: usageError }) });
		render(MetricsCatalogPlanet);

		expect(screen.getByText(usageError.title)).toBeInTheDocument();
		expect(screen.getByText(usageError.body)).toBeInTheDocument();
	});
});
