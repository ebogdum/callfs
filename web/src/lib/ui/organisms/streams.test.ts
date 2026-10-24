import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { base } from '$app/paths';
import { t } from '$lib/copy/t';
import EventFeed, { type FeedEvent } from './EventFeed.svelte';
import LogViewer, { type LogLine } from './LogViewer.svelte';
import Timeline, { type TimelineItem } from './Timeline.svelte';

const logLines: LogLine[] = [
	{
		id: 'log-1',
		at: '2026-10-23T10:00:00Z',
		level: 'error',
		message: 'disk full',
		logger: 'storage',
		fields: { instance_id: 'callfs-1', bytes: 12 },
		request_id: 'req-42',
	},
	{
		id: 'log-2',
		at: '2026-10-23T10:00:05Z',
		level: 'info',
		message: 'server started',
		logger: 'server',
		fields: {},
		request_id: null,
	},
];

const feedEvents: FeedEvent[] = [
	{
		id: 'evt-1',
		at: '2026-10-23T10:00:10Z',
		type: 'file.moved',
		level: 'info',
		actor: 'alice',
		instance_id: 'callfs-1',
		message: 'alice moved /a to /b',
		entities: [{ kind: 'file', id: '/b' }],
		fields: {},
	},
	{
		id: 'evt-2',
		at: '2026-10-23T10:01:20Z',
		type: 'security.auth_failed',
		level: 'warn',
		actor: null,
		instance_id: 'callfs-1',
		message: 'Sign-in failed for key k1',
		entities: [{ kind: 'link', id: 'lnk-9' }],
		fields: {},
	},
];

const timelineItems: TimelineItem[] = [
	{ at: '2026-10-23T09:00:00Z', title: 'Created', tone: 'ok', actor: 'alice' },
	{ at: '2026-10-23T11:00:00Z', title: 'Opened', detail: 'Downloaded once', tone: 'info', href: '/sharing/lnk-9' },
];

describe('LogViewer', () => {
	it('renders a named region with each line level word and message', () => {
		render(LogViewer, { props: { entries: logLines, status: 'success', live: false, level: [], query: '' } });
		expect(screen.getByRole('region', { name: t('u106logs.label') })).toBeInTheDocument();
		expect(screen.getByText('disk full')).toBeInTheDocument();
		expect(screen.getAllByText(t('u106logs.level.info')).length).toBeGreaterThan(0);
	});

	it('marks the region busy and shows no rows while loading an empty log', () => {
		render(LogViewer, { props: { entries: [], status: 'loading', live: false, level: [], query: '' } });
		expect(screen.getByRole('region', { name: t('u106logs.label') })).toHaveAttribute('aria-busy', 'true');
		expect(screen.queryByText('disk full')).not.toBeInTheDocument();
	});

	it('shows the empty state with plain copy when no lines match', () => {
		render(LogViewer, { props: { entries: [], status: 'empty', live: false, level: [], query: 'nothing' } });
		expect(screen.getByText(t('u106logs.empty.title'))).toBeInTheDocument();
		expect(screen.getByText(t('u106logs.empty.body'))).toBeInTheDocument();
	});

	it('shows the error alert with what happened and what to do next', () => {
		render(LogViewer, { props: { entries: [], status: 'error', live: false, level: [], query: '' } });
		expect(screen.getByText(t('u106logs.error.title'))).toBeInTheDocument();
		expect(screen.getByText(t('u106logs.error.body'))).toBeInTheDocument();
	});

	it('offers older lines from the partial alert and calls the handler', async () => {
		const onloadolder = vi.fn();
		render(LogViewer, {
			props: { entries: logLines, status: 'partial', live: false, level: [], query: '', onloadolder },
		});
		expect(screen.getByText(t('u106logs.partial.title'))).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: t('u106logs.load_older') }));
		expect(onloadolder).toHaveBeenCalledTimes(1);
	});

	it('expands a row to show fields, request ID and a JSON copy control', async () => {
		render(LogViewer, { props: { entries: logLines, status: 'success', live: false, level: [], query: '' } });
		const row = screen.getByText('disk full').closest('button');
		expect(row).toHaveAttribute('aria-expanded', 'false');
		await fireEvent.click(row as HTMLButtonElement);
		expect(row).toHaveAttribute('aria-expanded', 'true');
		expect(screen.getByText(t('u106logs.field.request_id'))).toBeInTheDocument();
		expect(screen.getByText('req-42')).toBeInTheDocument();
		expect(screen.getByText('12')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('u106logs.copy_json') })).toBeInTheDocument();
	});

	it('toggles level chips between pressed and released', async () => {
		render(LogViewer, { props: { entries: logLines, status: 'success', live: false, level: [], query: '' } });
		const errors = screen.getByRole('button', { name: t('u106logs.filter.error') });
		expect(errors).toHaveAttribute('aria-pressed', 'false');
		await fireEvent.click(errors);
		expect(errors).toHaveAttribute('aria-pressed', 'true');
		await fireEvent.click(errors);
		expect(errors).toHaveAttribute('aria-pressed', 'false');
	});

	it('exposes the live tail toggle as a pressed button', async () => {
		render(LogViewer, { props: { entries: logLines, status: 'success', live: false, level: [], query: '' } });
		const live = screen.getByRole('button', { name: t('u106logs.live') });
		expect(live).toHaveAttribute('aria-pressed', 'false');
		await fireEvent.click(live);
		expect(live).toHaveAttribute('aria-pressed', 'true');
	});

	it('labels the search field persistently', () => {
		render(LogViewer, { props: { entries: logLines, status: 'success', live: false, level: [], query: '' } });
		expect(screen.getByRole('searchbox', { name: t('u106logs.search.label') })).toBeInTheDocument();
	});
});

describe('EventFeed', () => {
	it('groups events under minute headings and links entities', () => {
		render(EventFeed, { props: { events: feedEvents, live: true, types: [] } });
		expect(screen.getByText('alice moved /a to /b')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: '/b' })).toHaveAttribute('href', `${base}/files/b`);
		expect(screen.getByRole('link', { name: 'lnk-9' })).toHaveAttribute('href', `${base}/sharing/lnk-9`);
		expect(screen.getAllByRole('heading', { level: 3 })).toHaveLength(2);
	});

	it('shows only the selected type group when a group is pressed', async () => {
		render(EventFeed, { props: { events: feedEvents, live: true, types: [] } });
		await fireEvent.click(screen.getByRole('button', { name: t('u106logs.group.files') }));
		expect(screen.getByText('alice moved /a to /b')).toBeInTheDocument();
		expect(screen.queryByText('Sign-in failed for key k1')).not.toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('u106logs.group.files') })).toHaveAttribute('aria-pressed', 'true');
	});

	it('shows the empty copy when there are no events', () => {
		render(EventFeed, { props: { events: [], live: false, types: [] } });
		expect(screen.getByText(t('u106logs.events.empty'))).toBeInTheDocument();
	});

	it('states whether the feed is live or paused in words', () => {
		const { unmount } = render(EventFeed, { props: { events: [], live: true, types: [] } });
		expect(screen.getByText(t('u106logs.events.live'))).toBeInTheDocument();
		unmount();
		render(EventFeed, { props: { events: [], live: false, types: [] } });
		expect(screen.getByText(t('u106logs.events.paused'))).toBeInTheDocument();
	});
});

describe('Timeline', () => {
	it('names the list and renders items newest first', () => {
		render(Timeline, { props: { items: timelineItems } });
		expect(screen.getByRole('list', { name: t('u106logs.timeline.label') })).toBeInTheDocument();
		const titles = screen.getAllByRole('listitem').map((item) => item.textContent ?? '');
		expect(titles[0]).toContain('Opened');
		expect(titles[1]).toContain('Created');
	});

	it('links titles that have an href and shows plain text otherwise', () => {
		render(Timeline, { props: { items: timelineItems } });
		expect(screen.getByRole('link', { name: 'Opened' })).toHaveAttribute('href', `${base}/sharing/lnk-9`);
		expect(screen.queryByRole('link', { name: 'Created' })).not.toBeInTheDocument();
		expect(screen.getByText('Created')).toBeInTheDocument();
	});

	it('shows the actor and detail lines when given', () => {
		render(Timeline, { props: { items: timelineItems } });
		expect(screen.getByText(t('u106logs.timeline.by', { actor: 'alice' }))).toBeInTheDocument();
		expect(screen.getByText('Downloaded once')).toBeInTheDocument();
	});

	it('shows the empty copy when nothing is recorded', () => {
		render(Timeline, { props: { items: [] } });
		expect(screen.getByText(t('u106logs.timeline.empty'))).toBeInTheDocument();
	});
});
