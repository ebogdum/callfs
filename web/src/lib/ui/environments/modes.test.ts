import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import AuthLayout from './AuthLayout.svelte';
import PrintLayout from './PrintLayout.svelte';
import WallLayout from './WallLayout.svelte';

const content = createRawSnippet(() => ({
	render: () => '<span class="content">content</span>'
}));

const footer = createRawSnippet(() => ({
	render: () => '<a href="/docs">docs</a>'
}));

afterEach(() => {
	cleanup();
	vi.useRealTimers();
	delete document.documentElement.dataset.contrast;
	delete document.documentElement.dataset.theme;
	document.documentElement.style.removeProperty('--font-scale');
});

describe('AuthLayout', () => {
	it('renders children inside the centred card container', () => {
		const { container } = render(AuthLayout, { props: { children: content } });
		const root = container.querySelector('.auth-layout') as HTMLElement;
		expect(root).not.toBeNull();
		expect(root.querySelector('.card .content')?.textContent).toBe('content');
	});

	it('renders the default footer unless a footer snippet is passed', () => {
		const { container } = render(AuthLayout, { props: { children: content } });
		expect(container.querySelector('.auth-footer span')?.textContent).toBe(t('shell.login.footer'));
		expect(container.querySelector('.auth-footer a')).toBeNull();
		cleanup();
		const withFooter = render(AuthLayout, { props: { children: content, footer } });
		expect(withFooter.container.querySelector('.auth-footer a')?.getAttribute('href')).toBe('/docs');
	});

	it('forwards class and rest attributes to the root element', () => {
		const { container } = render(AuthLayout, {
			props: { children: content, class: 'extra', 'data-testid': 'auth' }
		});
		const root = container.querySelector('[data-testid="auth"]') as HTMLElement;
		expect(root.classList.contains('auth-layout')).toBe(true);
		expect(root.classList.contains('extra')).toBe(true);
	});
});

describe('WallLayout', () => {
	it('renders children inside the wall grid', () => {
		const { container } = render(WallLayout, { props: { children: content } });
		expect(container.querySelector('.wall-layout .wall-grid .content')?.textContent).toBe('content');
	});

	it('forces high contrast and a 1.5 font scale on the document while mounted and restores them on unmount', () => {
		render(WallLayout, { props: { children: content } });
		expect(document.documentElement.dataset.contrast).toBe('more');
		expect(document.documentElement.style.getPropertyValue('--font-scale')).toBe('1.5');
		cleanup();
		expect(document.documentElement.dataset.contrast).toBeUndefined();
		expect(document.documentElement.style.getPropertyValue('--font-scale')).toBe('');
	});

	it('hides the cursor after 3 seconds idle and shows it again on pointer movement', async () => {
		vi.useFakeTimers();
		const { container } = render(WallLayout, { props: { children: content } });
		const root = container.querySelector('.wall-layout') as HTMLElement;
		expect(root.dataset.state).toBe('active');
		vi.advanceTimersByTime(2999);
		await tick();
		expect(root.dataset.state).toBe('active');
		vi.advanceTimersByTime(1);
		await tick();
		expect(root.dataset.state).toBe('idle');
		await fireEvent(window, new Event('pointermove'));
		await tick();
		expect(root.dataset.state).toBe('active');
	});
});

describe('PrintLayout', () => {
	it('renders children in the main body and a footer with the generated date', () => {
		const { container } = render(PrintLayout, { props: { children: content } });
		expect(container.querySelector('.print-body .content')?.textContent).toBe('content');
		const time = container.querySelector('.print-footer time') as HTMLTimeElement;
		expect(time.getAttribute('datetime')).not.toBeNull();
		expect(time.textContent?.trim().length).toBeGreaterThan(0);
	});

	it('forces the light theme on the document while mounted and restores it on unmount', () => {
		document.documentElement.dataset.theme = 'dark';
		render(PrintLayout, { props: { children: content } });
		expect(document.documentElement.dataset.theme).toBe('light');
		cleanup();
		expect(document.documentElement.dataset.theme).toBe('dark');
	});

	it('forwards class and rest attributes to the root element', () => {
		const { container } = render(PrintLayout, {
			props: { children: content, class: 'extra', 'data-testid': 'print' }
		});
		const root = container.querySelector('[data-testid="print"]') as HTMLElement;
		expect(root.classList.contains('print-layout')).toBe(true);
		expect(root.classList.contains('extra')).toBe(true);
	});
});
