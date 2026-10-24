import { cleanup, render, screen, waitFor, within } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import ActivitySystem from './ActivitySystem.svelte';
import InsightsSystem from './InsightsSystem.svelte';
import { parseTimeRange, presetValue, TimeRangeStore, writeTimeRange } from '$lib/stores/time-range.svelte';

const NOW = 1_800_000_000_000;
const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

const sessionState = vi.hoisted(() => ({ isAdmin: true }));
const pageState = vi.hoisted(() => ({ url: new URL('http://127.0.0.1:4173/insights'), state: {} }));
const navigation = vi.hoisted(() => ({ replaceState: vi.fn() }));

vi.mock('$app/state', () => ({ page: pageState }));
vi.mock('$app/navigation', () => ({ replaceState: navigation.replaceState }));
vi.mock('$lib/stores/session.svelte', () => ({ session: sessionState }));

const body = createRawSnippet(() => ({ render: () => '<p>Section body</p>' }));

beforeEach(() => {
	sessionState.isAdmin = true;
	pageState.url = new URL('http://127.0.0.1:4173/insights');
	navigation.replaceState.mockClear();
});

afterEach(() => {
	cleanup();
});

function linkHrefs(name: string): string[] {
	const nav = screen.getByRole('tablist', { name });
	return within(nav)
		.getAllByRole('tab')
		.map((link) => link.getAttribute('href') ?? '');
}

describe('time range URL codec', () => {
	it('defaults to the 24 hour preset when the URL has no range', () => {
		expect(parseTimeRange(new URLSearchParams(), NOW)).toEqual({ from: NOW - DAY, to: NOW, preset: '24h', live: false });
	});

	it('reads a preset and the live flag from the URL', () => {
		expect(parseTimeRange(new URLSearchParams('preset=1h&live=true'), NOW)).toEqual({
			from: NOW - HOUR,
			to: NOW,
			preset: '1h',
			live: true
		});
	});

	it('reads an absolute range when no preset is given', () => {
		expect(parseTimeRange(new URLSearchParams('from=1000&to=2000'), NOW)).toEqual({ from: 1000, to: 2000, live: false });
	});

	it('falls back to the default preset when the absolute range is reversed or not numeric', () => {
		const fallback = { from: NOW - DAY, to: NOW, preset: '24h', live: false };
		expect(parseTimeRange(new URLSearchParams('from=2000&to=1000'), NOW)).toEqual(fallback);
		expect(parseTimeRange(new URLSearchParams('from=soon&to=2000'), NOW)).toEqual(fallback);
	});

	it('ignores unknown presets', () => {
		expect(parseTimeRange(new URLSearchParams('preset=5y'), NOW)).toEqual({ from: NOW - DAY, to: NOW, preset: '24h', live: false });
	});

	it('writes a preset as one parameter and keeps unrelated parameters', () => {
		const params = new URLSearchParams('from=1&to=2&q=errors');
		writeTimeRange(params, presetValue('6h', NOW, true));
		expect(params.get('preset')).toBe('6h');
		expect(params.has('from')).toBe(false);
		expect(params.has('to')).toBe(false);
		expect(params.get('live')).toBe('true');
		expect(params.get('q')).toBe('errors');
	});

	it('writes an absolute range as from and to and drops the preset and live flag', () => {
		const params = new URLSearchParams('preset=1h&live=true');
		writeTimeRange(params, { from: 1000, to: 2000, live: false });
		expect(params.get('from')).toBe('1000');
		expect(params.get('to')).toBe('2000');
		expect(params.has('preset')).toBe(false);
		expect(params.has('live')).toBe(false);
	});
});

describe('TimeRangeStore', () => {
	it('moves a live preset window forward on refresh', () => {
		const store = new TimeRangeStore(presetValue('1h', NOW, true));
		store.refresh(NOW + 5000);
		expect(store.value).toEqual({ from: NOW + 5000 - HOUR, to: NOW + 5000, preset: '1h', live: true });
	});

	it('keeps a paused preset window on refresh', () => {
		const store = new TimeRangeStore(presetValue('1h', NOW, false));
		const before = store.value;
		store.refresh(NOW + 5000);
		expect(store.value).toBe(before);
	});

	it('keeps an absolute range on refresh', () => {
		const store = new TimeRangeStore({ from: 1000, to: 2000, live: false });
		const before = store.value;
		store.refresh(NOW);
		expect(store.value).toBe(before);
	});
});

describe('InsightsSystem', () => {
	it('lists the four insight pages in the sub-navigation for admins', () => {
		render(InsightsSystem, { props: { children: body } });
		expect(linkHrefs(t('nav.insights'))).toEqual([
			'/ui/insights',
			'/ui/insights/explore',
			'/ui/insights/storage',
			'/ui/insights/metrics'
		]);
	});

	it('marks the page that matches the URL as current', () => {
		pageState.url = new URL('http://127.0.0.1:4173/insights/explore');
		render(InsightsSystem, { props: { children: body } });
		expect(screen.getByRole('tab', { name: t('nav.insights.explore') }).getAttribute('aria-selected')).toBe('true');
	});

	it('hides admin-only insight pages from members', () => {
		sessionState.isAdmin = false;
		render(InsightsSystem, { props: { children: body } });
		expect(screen.queryByRole('tab', { name: t('nav.insights.dashboards') })).toBeNull();
	});

	it('renders the page content', () => {
		render(InsightsSystem, { props: { children: body } });
		expect(screen.getByText('Section body')).not.toBeNull();
	});

	it('writes the shared time range into the URL', async () => {
		render(InsightsSystem, { props: { children: body } });
		await waitFor(() => expect(navigation.replaceState).toHaveBeenCalled());
		const [url] = navigation.replaceState.mock.calls[0] as [URL];
		expect(url.searchParams.get('preset')).toBe('24h');
	});
});

describe('ActivitySystem', () => {
	it('lists all five activity pages in the sub-navigation for admins', () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity');
		render(ActivitySystem, { props: { children: body } });
		expect(linkHrefs(t('nav.activity'))).toEqual([
			'/ui/activity',
			'/ui/activity/audit',
			'/ui/activity/logs',
			'/ui/activity/jobs',
			'/ui/activity/sign-ins'
		]);
	});

	it('shows members only their own jobs', () => {
		sessionState.isAdmin = false;
		pageState.url = new URL('http://127.0.0.1:4173/activity');
		render(ActivitySystem, { props: { children: body } });
		expect(linkHrefs(t('nav.activity'))).toEqual(['/ui/activity/jobs']);
	});

	it('marks the page that matches the URL as current', () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/audit');
		render(ActivitySystem, { props: { children: body } });
		expect(screen.getByRole('tab', { name: t('nav.activity.audit') }).getAttribute('aria-selected')).toBe('true');
	});

	it('renders the page content', () => {
		render(ActivitySystem, { props: { children: body } });
		expect(screen.getByText('Section body')).not.toBeNull();
	});
});
