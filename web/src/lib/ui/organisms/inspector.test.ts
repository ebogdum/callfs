import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { SplitFileDetail, Stat } from '$lib/api/types';
import Inspector from './Inspector.svelte';
import type { InspectorTab } from './Inspector.svelte';

afterEach(() => {
	cleanup();
});

const stat = (overrides: Partial<Stat> = {}): Stat => ({
	path: '/docs/report.pdf',
	name: 'report.pdf',
	type: 'file',
	size: 2048,
	mode: '0644',
	owner: 'alice',
	etag: '"abc"',
	storage: 'localfs',
	instance_id: 'callfs-1',
	is_local: true,
	erasure_coded: false,
	symlink_target: null,
	mtime: '2026-10-01T10:00:00Z',
	atime: '2026-10-02T10:00:00Z',
	ctime: '2026-10-03T10:00:00Z',
	created_at: '2026-09-30T10:00:00Z',
	updated_at: '2026-10-01T10:00:00Z',
	locked: false,
	children_count: null,
	...overrides
});

const splitFile: SplitFileDetail = {
	path: '/docs/report.pdf',
	original_size: 2048,
	profile: { data_shards: 2, parity_shards: 1, shard_size: 1024 },
	shards: [
		{ index: 0, kind: 'data', instance_id: 'callfs-1', storage: 'localfs', path: 'p0', size: 1024, checksum: 'c0', state: 'ok' },
		{ index: 1, kind: 'data', instance_id: 'callfs-2', storage: 'localfs', path: 'p1', size: 1024, checksum: 'c1', state: 'missing' },
		{ index: 2, kind: 'parity', instance_id: 'callfs-3', storage: 'localfs', path: 'p2', size: 1024, checksum: 'c2', state: 'ok' }
	],
	tolerance: 1,
	last_checked_at: null
};

const tabs = (...ids: InspectorTab['id'][]): InspectorTab[] => ids.map((id) => ({ id }));

const renderInspector = (overrides: Record<string, unknown> = {}) =>
	render(Inspector, {
		props: {
			entity: stat(),
			tabs: tabs('details', 'placement', 'durability', 'links', 'permissions', 'activity'),
			actions: [],
			onclose: vi.fn(),
			ondownload: vi.fn(),
			...overrides
		}
	});

describe('Inspector', () => {
	it('exposes a labelled complementary landmark with a labelled tab list', () => {
		renderInspector({ tabs: tabs('details', 'placement') });
		expect(screen.getByRole('complementary', { name: t('inspector.label') })).toBeTruthy();
		expect(screen.getByRole('tablist', { name: t('inspector.tabs.label') })).toBeTruthy();
		expect(screen.getByRole('tab', { name: t('inspector.tab.details') })).toBeTruthy();
		expect(screen.getByRole('tab', { name: t('inspector.tab.placement') })).toBeTruthy();
	});

	it('shows the empty state when nothing is selected', () => {
		renderInspector({ entity: null });
		expect(screen.getByText(t('inspector.empty.title'))).toBeTruthy();
		expect(screen.queryByRole('tablist')).toBeNull();
	});

	it('shows the file path in the details tab', () => {
		renderInspector({ value: 'details' });
		expect(screen.getByText('/docs/report.pdf')).toBeTruthy();
	});

	it('renders the placement panel for the active value', () => {
		renderInspector({ value: 'placement' });
		expect(screen.getByText(t('inspector.route.local'))).toBeTruthy();
	});

	it('calls onclose when Escape is pressed inside the inspector', () => {
		const onclose = vi.fn();
		renderInspector({ onclose });
		fireEvent.keyDown(screen.getByRole('complementary'), { key: 'Escape' });
		expect(onclose).toHaveBeenCalledTimes(1);
	});

	it('calls ondownload from the Download button for files', () => {
		const ondownload = vi.fn();
		renderInspector({ ondownload });
		fireEvent.click(screen.getByRole('button', { name: t('inspector.action.download') }));
		expect(ondownload).toHaveBeenCalledTimes(1);
	});

	it('omits the Download button for folders', () => {
		renderInspector({ entity: stat({ type: 'directory' }) });
		expect(screen.queryByRole('button', { name: t('inspector.action.download') })).toBeNull();
	});

	it('reflects the pinned state on the pin toggle', async () => {
		renderInspector({ pinned: false });
		const toggle = screen.getByRole('button', { name: t('inspector.pin') });
		expect(toggle.getAttribute('aria-pressed')).toBe('false');
		await fireEvent.click(toggle);
		expect(toggle.getAttribute('aria-pressed')).toBe('true');
	});

	it('renders the links snippet on the links tab', () => {
		const links = createRawSnippet(() => ({ render: () => '<p>Share one</p>' }));
		renderInspector({ value: 'links', links });
		expect(screen.getByText('Share one')).toBeTruthy();
	});

	it('renders the permissions snippet on the permissions tab', () => {
		const permissions = createRawSnippet(() => ({ render: () => '<p>Owner only</p>' }));
		renderInspector({ value: 'permissions', permissions });
		expect(screen.getByText('Owner only')).toBeTruthy();
	});

	it('shows the durability result for split files', () => {
		renderInspector({ value: 'durability', erasure: splitFile, busy: false });
		expect(screen.getByText(t('inspector.durability.result', { healthy: '2', total: '3' }))).toBeTruthy();
		expect(screen.getByRole('button', { name: t('inspector.durability.verify') })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('inspector.durability.repair') })).toBeTruthy();
	});

	it('shows the not split note on the durability tab for unsplit files', () => {
		renderInspector({ value: 'durability', erasure: null });
		expect(screen.getByText(t('inspector.durability.not_split'))).toBeTruthy();
	});

	it('shows the activity error alert when activity fails to load', () => {
		renderInspector({ value: 'activity', activityStatus: 'error', events: [] });
		expect(screen.getByText(t('inspector.activity.error.title'))).toBeTruthy();
	});

	it('shows activity events in a timeline on the activity tab', () => {
		renderInspector({
			value: 'activity',
			activityStatus: 'success',
			events: [{ at: '2026-10-01T10:00:00Z', title: 'Renamed to report.pdf', tone: 'neutral' }]
		});
		expect(screen.getByText('Renamed to report.pdf')).toBeTruthy();
	});
});
