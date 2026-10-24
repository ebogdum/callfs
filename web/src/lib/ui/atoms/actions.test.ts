import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import Button from './Button.svelte';
import IconButton from './IconButton.svelte';
import Link from './Link.svelte';
import ToggleButton from './ToggleButton.svelte';

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

afterEach(() => {
	cleanup();
});

describe('Link', () => {
	it('renders an anchor named by its visible text without external attributes', () => {
		render(Link, { props: { href: '/sharing', children: textSnippet('Sharing') } });
		const link = screen.getByRole('link', { name: 'Sharing' });
		expect(link.getAttribute('href')).toBe('/sharing');
		expect(link.hasAttribute('target')).toBe(false);
		expect(link.hasAttribute('rel')).toBe(false);
	});

	it('adds new-tab target, safe rel and hidden new-tab text for external links', () => {
		render(Link, { props: { href: 'https://example.com', external: true, children: textSnippet('Docs') } });
		const link = screen.getByRole('link', { name: /Docs/ });
		expect(link.getAttribute('target')).toBe('_blank');
		expect(link.getAttribute('rel')).toBe('noopener noreferrer');
		expect(link.textContent).toContain(t('common.opens_new_tab'));
	});

	it('exposes the muted tone as a data attribute', () => {
		render(Link, { props: { href: '/x', tone: 'muted', children: textSnippet('Quiet') } });
		expect(screen.getByRole('link', { name: 'Quiet' }).getAttribute('data-tone')).toBe('muted');
	});
});

describe('Button', () => {
	it('renders a real button type with secondary and md defaults', () => {
		render(Button, { props: { children: textSnippet('Save') } });
		const button = screen.getByRole('button', { name: 'Save' });
		expect(button.getAttribute('type')).toBe('button');
		expect(button.getAttribute('data-variant')).toBe('secondary');
		expect(button.getAttribute('data-size')).toBe('md');
	});

	it.each(['primary', 'secondary', 'ghost', 'danger'] as const)('sets data-variant for %s', (variant) => {
		render(Button, { props: { variant, children: textSnippet('Act') } });
		expect(screen.getByRole('button', { name: 'Act' }).getAttribute('data-variant')).toBe(variant);
	});

	it.each(['sm', 'md', 'lg'] as const)('sets data-size for %s', (size) => {
		render(Button, { props: { size, children: textSnippet('Act') } });
		expect(screen.getByRole('button', { name: 'Act' }).getAttribute('data-size')).toBe(size);
	});

	it('fires onclick with the event on click', async () => {
		const onclick = vi.fn();
		render(Button, { props: { onclick, children: textSnippet('Open') } });
		await fireEvent.click(screen.getByRole('button', { name: 'Open' }));
		expect(onclick).toHaveBeenCalledTimes(1);
		expect(onclick.mock.calls[0]?.[0]).toBeInstanceOf(MouseEvent);
	});

	it('marks loading as busy and ignores clicks while loading', async () => {
		const onclick = vi.fn();
		render(Button, { props: { loading: true, onclick, children: textSnippet('Upload') } });
		const button = screen.getByRole('button', { name: 'Upload' });
		expect(button.getAttribute('aria-busy')).toBe('true');
		expect(button.getAttribute('data-state')).toBe('loading');
		await fireEvent.click(button);
		expect(onclick).not.toHaveBeenCalled();
	});

	it('exposes disabledReason as aria-disabled, stays focusable and ignores clicks', async () => {
		const onclick = vi.fn();
		render(Button, {
			props: { disabledReason: 'Select a file first', onclick, children: textSnippet('Share') }
		});
		const button = screen.getByRole('button', { name: 'Share' });
		expect(button.getAttribute('aria-disabled')).toBe('true');
		expect(button.tabIndex).not.toBe(-1);
		button.focus();
		expect(document.activeElement).toBe(button);
		await fireEvent.click(button);
		expect(onclick).not.toHaveBeenCalled();
	});

	it('shows the shortcut hint only when size is not sm', () => {
		const { container, unmount } = render(Button, {
			props: { size: 'md', shortcut: ['mod', 'k'], children: textSnippet('Find') }
		});
		expect(container.querySelector('kbd')).not.toBeNull();
		unmount();
		const small = render(Button, {
			props: { size: 'sm', shortcut: ['mod', 'k'], children: textSnippet('Find') }
		});
		expect(small.container.querySelector('kbd')).toBeNull();
	});
});

describe('IconButton', () => {
	it('names the button by its label', () => {
		render(IconButton, { props: { icon: 'trash', label: 'Delete' } });
		const button = screen.getByRole('button', { name: 'Delete' });
		expect(button.getAttribute('aria-label')).toBe('Delete');
		expect(button.getAttribute('data-variant')).toBe('ghost');
	});

	it('exposes pressed state through aria-pressed only when pressed is defined', () => {
		render(IconButton, { props: { icon: 'pin', label: 'Pin', pressed: true } });
		expect(screen.getByRole('button', { name: 'Pin' }).getAttribute('aria-pressed')).toBe('true');
	});

	it('omits aria-pressed when pressed is undefined', () => {
		render(IconButton, { props: { icon: 'pin', label: 'Pin' } });
		expect(screen.getByRole('button', { name: 'Pin' }).hasAttribute('aria-pressed')).toBe(false);
	});

	it('renders the badge count beside the button', () => {
		render(IconButton, { props: { icon: 'bell', label: 'Alerts', badge: 3 } });
		expect(screen.getByRole('button', { name: 'Alerts' }).closest('.anchor')?.textContent).toContain('3');
	});

	it('exposes disabledReason as aria-disabled and ignores clicks', async () => {
		const onclick = vi.fn();
		render(IconButton, { props: { icon: 'share', label: 'Share', disabledReason: 'Read only', onclick } });
		const button = screen.getByRole('button', { name: 'Share' });
		expect(button.getAttribute('aria-disabled')).toBe('true');
		await fireEvent.click(button);
		expect(onclick).not.toHaveBeenCalled();
	});

	it('fires onclick and takes keyboard focus', async () => {
		const onclick = vi.fn();
		render(IconButton, { props: { icon: 'copy', label: 'Copy path', onclick } });
		const button = screen.getByRole('button', { name: 'Copy path' });
		button.focus();
		expect(document.activeElement).toBe(button);
		await fireEvent.click(button);
		expect(onclick).toHaveBeenCalledTimes(1);
	});
});

describe('ToggleButton', () => {
	it('names the toggle by its label and reflects the pressed state', () => {
		render(ToggleButton, { props: { pressed: false, label: 'Favourite' } });
		const button = screen.getByRole('button', { name: 'Favourite' });
		expect(button.getAttribute('aria-pressed')).toBe('false');
		expect(button.hasAttribute('data-state')).toBe(false);
	});

	it('flips aria-pressed and data-state on click and calls onclick', async () => {
		const onclick = vi.fn();
		render(ToggleButton, { props: { pressed: false, label: 'Favourite', icon: 'star', onclick } });
		const button = screen.getByRole('button', { name: 'Favourite' });
		await fireEvent.click(button);
		expect(button.getAttribute('aria-pressed')).toBe('true');
		expect(button.getAttribute('data-state')).toBe('selected');
		expect(onclick).toHaveBeenCalledTimes(1);
	});
});
