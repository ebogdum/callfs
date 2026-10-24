import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import Toast from './Toast.svelte';
import InlineAlert from './InlineAlert.svelte';
import Banner from './Banner.svelte';
import EmptyState from './EmptyState.svelte';
import ErrorState from './ErrorState.svelte';
import type { UiError } from '../types';

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const baseError: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	status: 500,
	code: 'INTERNAL_ERROR',
	requestId: 'req-1',
	retryable: true
};

afterEach(() => {
	cleanup();
	vi.useRealTimers();
	Reflect.deleteProperty(navigator, 'clipboard');
});

describe('Toast', () => {
	it('renders a status region named by its title and body for non-danger tones', () => {
		render(Toast, { props: { tone: 'ok', title: 'Saved', body: 'Your changes are kept.', onclose: vi.fn() } });
		const toast = screen.getByRole('status');
		expect(toast.textContent).toContain('Saved');
		expect(toast.textContent).toContain('Your changes are kept.');
		expect(toast.getAttribute('data-tone')).toBe('ok');
	});

	it('uses the alert role for danger tone', () => {
		render(Toast, { props: { tone: 'danger', title: 'Upload failed', onclose: vi.fn() } });
		expect(screen.getByRole('alert').textContent).toContain('Upload failed');
	});

	it('runs the action and closes through the close button', async () => {
		const run = vi.fn();
		const onclose = vi.fn();
		render(Toast, { props: { tone: 'info', title: 'File moved', action: { label: 'Undo', run }, onclose } });
		await fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
		expect(run).toHaveBeenCalledTimes(1);
		await fireEvent.click(screen.getByRole('button', { name: t('common.close') }));
		expect(onclose).toHaveBeenCalledTimes(1);
	});

	it('closes after six seconds when no action is offered', () => {
		vi.useFakeTimers();
		const onclose = vi.fn();
		render(Toast, { props: { tone: 'ok', title: 'Saved', onclose } });
		flushSync();
		vi.advanceTimersByTime(5999);
		expect(onclose).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(onclose).toHaveBeenCalledTimes(1);
	});

	it('waits ten seconds when an action is offered', () => {
		vi.useFakeTimers();
		const onclose = vi.fn();
		render(Toast, { props: { tone: 'info', title: 'File moved', action: { label: 'Undo', run: vi.fn() }, onclose } });
		flushSync();
		vi.advanceTimersByTime(9999);
		expect(onclose).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(onclose).toHaveBeenCalledTimes(1);
	});

	it('pauses while hovered and resumes the remaining time on leave', async () => {
		vi.useFakeTimers();
		const onclose = vi.fn();
		render(Toast, { props: { tone: 'ok', title: 'Saved', onclose } });
		flushSync();
		const toast = screen.getByRole('status');
		vi.advanceTimersByTime(2000);
		await fireEvent.pointerEnter(toast);
		vi.advanceTimersByTime(10000);
		expect(onclose).not.toHaveBeenCalled();
		await fireEvent.pointerLeave(toast);
		vi.advanceTimersByTime(3999);
		expect(onclose).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(onclose).toHaveBeenCalledTimes(1);
	});
});

describe('InlineAlert', () => {
	it('renders a status region with title, body and action buttons', () => {
		const actions = createRawSnippet(() => ({
			render: () => '<button type="button">Keep theirs</button>'
		}));
		render(InlineAlert, {
			props: { tone: 'warn', title: 'Some items could not be listed.', body: 'Open details to see them.', actions }
		});
		const alert = screen.getByRole('status');
		expect(alert.getAttribute('data-tone')).toBe('warn');
		expect(alert.textContent).toContain('Some items could not be listed.');
		expect(alert.textContent).toContain('Open details to see them.');
		expect(screen.getByRole('button', { name: 'Keep theirs' })).toBeTruthy();
	});

	it('uses the alert role for danger tone', () => {
		render(InlineAlert, { props: { tone: 'danger', title: 'This file changed after you opened the folder.' } });
		expect(screen.getByRole('alert').textContent).toContain('This file changed after you opened the folder.');
	});
});

describe('Banner', () => {
	it('renders the title and runs the action', async () => {
		const run = vi.fn();
		render(Banner, { props: { tone: 'warn', title: 'Restart callfs to use saved changes.', action: { label: 'Restart now', run } } });
		expect(screen.getByRole('status').textContent).toContain('Restart callfs to use saved changes.');
		await fireEvent.click(screen.getByRole('button', { name: 'Restart now' }));
		expect(run).toHaveBeenCalledTimes(1);
	});

	it('shows no close button unless dismissible', () => {
		render(Banner, { props: { tone: 'info', title: 'Maintenance tonight.' } });
		expect(screen.queryByRole('button', { name: t('common.close') })).toBeNull();
	});

	it('hides itself and reports the dismissal when dismissed', async () => {
		const ondismiss = vi.fn();
		render(Banner, { props: { tone: 'danger', title: 'No leader.', dismissible: true, ondismiss } });
		expect(screen.getByRole('alert').textContent).toContain('No leader.');
		await fireEvent.click(screen.getByRole('button', { name: t('common.close') }));
		expect(ondismiss).toHaveBeenCalledTimes(1);
		expect(screen.queryByRole('alert')).toBeNull();
	});
});

describe('EmptyState', () => {
	it('renders the variant, title and body as a heading', () => {
		const { container } = render(EmptyState, {
			props: { variant: 'first-use', icon: 'upload', title: 'No files yet', body: 'Upload a file to start.' }
		});
		const root = container.querySelector('.empty-state');
		expect(root?.getAttribute('data-variant')).toBe('first-use');
		expect(screen.getByRole('heading', { level: 2, name: 'No files yet' })).toBeTruthy();
		expect(screen.getByText('Upload a file to start.')).toBeTruthy();
	});

	it('runs the primary action when it is a button', async () => {
		const onclick = vi.fn();
		render(EmptyState, {
			props: {
				variant: 'first-use',
				icon: 'upload',
				title: 'No files yet',
				body: 'Upload a file to start.',
				primary: { label: 'Upload files', onclick }
			}
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Upload files' }));
		expect(onclick).toHaveBeenCalledTimes(1);
	});

	it('renders the primary action and the secondary link with their hrefs', () => {
		render(EmptyState, {
			props: {
				variant: 'not-available',
				icon: 'network',
				title: 'This server runs alone.',
				body: 'Add members to keep the file index on more than one server.',
				primary: { label: 'Open setting', href: '/cluster/nodes' },
				secondary: { label: 'How to add members', href: '/docs/members' }
			}
		});
		expect(screen.getByRole('link', { name: 'Open setting' }).getAttribute('href')).toBe('/cluster/nodes');
		expect(screen.getByRole('link', { name: 'How to add members' }).getAttribute('href')).toBe('/docs/members');
	});

	it('renders the example snippet when given', () => {
		render(EmptyState, {
			props: {
				variant: 'no-results',
				icon: 'search',
				title: 'No files match acmee.',
				body: 'Check the spelling.',
				example: textSnippet('Example: news')
			}
		});
		expect(screen.getByText('Example: news')).toBeTruthy();
	});
});

describe('ErrorState', () => {
	it('renders the error title and body in a region', () => {
		const { container } = render(ErrorState, { props: { error: baseError } });
		expect(screen.getByRole('heading', { level: 2, name: 'We cannot reach callfs.' })).toBeTruthy();
		expect(screen.getByText('Check your connection. We will try again.')).toBeTruthy();
		expect(container.querySelector('[data-state="error"]')?.getAttribute('data-variant')).toBe('region');
	});

	it('shows Retry for retryable errors and runs it', async () => {
		const onretry = vi.fn();
		render(ErrorState, { props: { error: baseError, onretry } });
		await fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));
		expect(onretry).toHaveBeenCalledTimes(1);
	});

	it('hides Retry when the error is not retryable', () => {
		render(ErrorState, { props: { error: { ...baseError, retryable: false }, onretry: vi.fn() } });
		expect(screen.queryByRole('button', { name: t('common.retry') })).toBeNull();
	});

	it('runs the fix action button', async () => {
		const run = vi.fn();
		render(ErrorState, {
			props: { error: { ...baseError, retryable: false, fixAction: { label: 'Show permissions', run } } }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Show permissions' }));
		expect(run).toHaveBeenCalledTimes(1);
	});

	it('links the fix action when it has an href', () => {
		render(ErrorState, {
			props: { error: { ...baseError, retryable: false, fixAction: { label: 'Open Topology', href: '/cluster' } } }
		});
		expect(screen.getByRole('link', { name: 'Open Topology' }).getAttribute('href')).toBe('/cluster');
	});

	it('copies status, code, request id and time through Copy details', async () => {
		const writeText = vi.fn().mockResolvedValue(undefined);
		Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
		render(ErrorState, { props: { error: baseError } });
		await fireEvent.click(screen.getByRole('button', { name: t('errors.server.fix') }));
		expect(writeText).toHaveBeenCalledTimes(1);
		const copied = String(writeText.mock.calls[0]?.[0]);
		expect(copied).toContain('500');
		expect(copied).toContain('INTERNAL_ERROR');
		expect(copied).toContain('req-1');
	});
});
