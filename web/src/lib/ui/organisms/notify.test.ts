import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import Toaster from './Toaster.svelte';
import NotificationCenter from './NotificationCenter.svelte';
import type { NotificationEntry } from './NotificationCenter.svelte';
import type { ToastEntry } from './Toaster.svelte';

afterEach(() => {
	cleanup();
});

const toast = (overrides: Partial<ToastEntry> = {}): ToastEntry => ({
	id: 1,
	tone: 'ok',
	title: 'Saved',
	duration: 6000,
	...overrides
});

const entry = (overrides: Partial<NotificationEntry> = {}): NotificationEntry => ({
	id: 1,
	at: Date.now(),
	tone: 'danger',
	category: 'error',
	title: 'Upload failed',
	body: 'The file is too large.',
	requestId: 'req-1',
	read: false,
	...overrides
});

describe('Toaster', () => {
	it('renders a polite region named by the notifications copy', () => {
		render(Toaster, { props: { items: [], ondismiss: vi.fn() } });
		const region = screen.getByRole('region', { name: t('notifications.toasts.region') });
		expect(region.getAttribute('aria-live')).toBe('polite');
		expect(screen.queryAllByRole('status')).toHaveLength(0);
	});

	it('shows at most three toasts in order', () => {
		const items = [
			toast({ id: 1, title: 'First' }),
			toast({ id: 2, title: 'Second' }),
			toast({ id: 3, title: 'Third' }),
			toast({ id: 4, title: 'Fourth' })
		];
		render(Toaster, { props: { items, ondismiss: vi.fn() } });
		const region = screen.getByRole('region', { name: t('notifications.toasts.region') });
		expect(region.textContent).toContain('First');
		expect(region.textContent).toContain('Third');
		expect(region.textContent).not.toContain('Fourth');
	});

	it('dismisses the toast whose close button was pressed', async () => {
		const ondismiss = vi.fn();
		const items = [toast({ id: 7, title: 'Saved' }), toast({ id: 8, title: 'Moved' })];
		render(Toaster, { props: { items, ondismiss } });
		const closeButtons = screen.getAllByRole('button', { name: t('common.close') });
		await fireEvent.click(closeButtons[1]);
		expect(ondismiss).toHaveBeenCalledTimes(1);
		expect(ondismiss).toHaveBeenCalledWith(8);
	});

	it('runs the toast action when its button is pressed', async () => {
		const run = vi.fn();
		const items = [toast({ id: 2, title: 'File moved', action: { label: 'Undo', run }, duration: 10000 })];
		render(Toaster, { props: { items, ondismiss: vi.fn() } });
		await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
		expect(run).toHaveBeenCalledTimes(1);
	});
});

describe('NotificationCenter', () => {
	it('names the panel with the notifications title', () => {
		render(NotificationCenter, { props: { items: [], onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() } });
		expect(screen.getByRole('region', { name: t('notifications.center.title') })).toBeTruthy();
	});

	it('shows the empty state when there are no notifications', () => {
		render(NotificationCenter, { props: { items: [], onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() } });
		expect(screen.getByText(t('notifications.center.empty_title'))).toBeTruthy();
		expect(screen.queryAllByRole('listitem')).toHaveLength(0);
	});

	it('lists entries with unread markers, request IDs and the unread count', () => {
		const items = [
			entry({ id: 1, title: 'Upload failed', read: false, requestId: 'req-1' }),
			entry({ id: 2, title: 'Node joined', category: 'cluster', tone: 'ok', read: true, requestId: null })
		];
		render(NotificationCenter, { props: { items, onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() } });
		expect(screen.getAllByRole('listitem')).toHaveLength(2);
		expect(screen.getByText(t('notifications.entry.new'))).toBeTruthy();
		expect(screen.getByText(`${t('notifications.entry.request_id')} req-1`)).toBeTruthy();
		expect(screen.getByRole('button', { name: t('notifications.entry.copy_request_id') })).toBeTruthy();
		expect(screen.getByText('1 unread')).toBeTruthy();
	});

	it('filters entries by category through the segmented control', async () => {
		const items = [
			entry({ id: 1, title: 'Upload failed', category: 'error' }),
			entry({ id: 2, title: 'Backup finished', category: 'job', tone: 'ok' }),
			entry({ id: 3, title: 'Node lagging', category: 'cluster', tone: 'warn' })
		];
		render(NotificationCenter, { props: { items, onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() } });
		await fireEvent.click(screen.getByText(t('notifications.filter.jobs')));
		expect(screen.getAllByRole('listitem')).toHaveLength(1);
		expect(screen.getByText('Backup finished')).toBeTruthy();
		await fireEvent.click(screen.getByText(t('notifications.filter.errors')));
		expect(screen.getByText('Upload failed')).toBeTruthy();
		expect(screen.queryByText('Backup finished')).toBeNull();
	});

	it('shows the filtered empty state when the filter matches nothing', async () => {
		const items = [entry({ id: 1, category: 'error' })];
		render(NotificationCenter, { props: { items, onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() } });
		await fireEvent.click(screen.getByText(t('notifications.filter.cluster')));
		expect(screen.getByText(t('notifications.center.filtered_empty_title'))).toBeTruthy();
	});

	it('marks every unread entry read with mark all read', async () => {
		const onread = vi.fn();
		const items = [
			entry({ id: 1, read: false }),
			entry({ id: 2, read: true }),
			entry({ id: 3, read: false })
		];
		render(NotificationCenter, { props: { items, onread, onclear: vi.fn(), onopen: vi.fn() } });
		await fireEvent.click(screen.getByRole('button', { name: t('notifications.center.mark_all_read') }));
		expect(onread.mock.calls).toEqual([[1], [3]]);
	});

	it('reports single entry actions with the entry payload', async () => {
		const onread = vi.fn();
		const onclear = vi.fn();
		const onopen = vi.fn();
		const target = entry({ id: 9, title: 'Upload failed', read: false });
		render(NotificationCenter, { props: { items: [target], onread, onclear, onopen } });
		await fireEvent.click(screen.getByRole('button', { name: t('notifications.entry.mark_read') }));
		expect(onread).toHaveBeenCalledWith(9);
		await fireEvent.click(screen.getByRole('button', { name: t('notifications.entry.open') }));
		expect(onopen).toHaveBeenCalledWith(target);
		await fireEvent.click(screen.getByRole('button', { name: t('notifications.entry.clear') }));
		expect(onclear).toHaveBeenCalledWith(9);
	});

	it('hides the mark read action for entries that are already read', () => {
		render(NotificationCenter, {
			props: { items: [entry({ id: 4, read: true })], onread: vi.fn(), onclear: vi.fn(), onopen: vi.fn() }
		});
		expect(screen.queryByRole('button', { name: t('notifications.entry.mark_read') })).toBeNull();
	});
});
