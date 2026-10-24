import { createRawSnippet } from 'svelte';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { STATUS } from '$lib/status/catalog';
import { t } from '$lib/copy/t';
import Avatar from './Avatar.svelte';
import Badge from './Badge.svelte';
import StatusPill from './StatusPill.svelte';
import Tag from './Tag.svelte';

const text = (value: string) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));

describe('StatusPill', () => {
	it('shows the catalog word for its status', () => {
		const { container } = render(StatusPill, { props: { status: 'health.ok' } });
		expect(container.querySelector('.word')?.textContent).toBe(t(STATUS['health.ok'].word));
	});

	it('uses the label prop instead of the catalog word', () => {
		const { container } = render(StatusPill, { props: { status: 'health.ok', label: 'Custom' } });
		expect(container.querySelector('.word')?.textContent).toBe('Custom');
	});

	it('takes its tone from the catalog and maps progress to info', () => {
		const ok = render(StatusPill, { props: { status: 'health.ok' } });
		expect(ok.container.querySelector('.pill')?.getAttribute('data-tone')).toBe('ok');
		const progress = render(StatusPill, { props: { status: 'health.checking' } });
		expect(progress.container.querySelector('.pill')?.getAttribute('data-tone')).toBe('info');
	});

	it('applies the size as a data attribute', () => {
		const { container } = render(StatusPill, { props: { status: 'health.ok', size: 'sm' } });
		expect(container.querySelector('.pill')?.getAttribute('data-size')).toBe('sm');
	});

	it('keeps the word visible when a detail tooltip is attached', () => {
		const { container } = render(StatusPill, {
			props: { status: 'health.down', detail: 'Last check failed' }
		});
		expect(container.querySelector('.word')?.textContent).toBe(t(STATUS['health.down'].word));
	});

	it('passes class through to the root', () => {
		const { container } = render(StatusPill, {
			props: { status: 'health.ok', class: 'extra' }
		});
		expect(container.querySelector('.pill')?.classList.contains('extra')).toBe(true);
	});
});

describe('Badge', () => {
	it('renders its children', () => {
		const { container } = render(Badge, { props: { children: text('Beta') } });
		expect(container.querySelector('.badge')?.textContent).toBe('Beta');
	});

	it('sets tone and variant as data attributes', () => {
		const { container } = render(Badge, {
			props: { tone: 'warn', variant: 'outline', children: text('Two') }
		});
		const badge = container.querySelector('.badge');
		expect(badge?.getAttribute('data-tone')).toBe('warn');
		expect(badge?.getAttribute('data-variant')).toBe('outline');
	});

	it('defaults to neutral soft', () => {
		const { container } = render(Badge, { props: { children: text('Three') } });
		const badge = container.querySelector('.badge');
		expect(badge?.getAttribute('data-tone')).toBe('neutral');
		expect(badge?.getAttribute('data-variant')).toBe('soft');
	});

	it('renders an icon only when one is given', () => {
		const withIcon = render(Badge, { props: { icon: 'x', children: text('Four') } });
		expect(withIcon.container.querySelector('svg')).not.toBeNull();
		const withoutIcon = render(Badge, { props: { children: text('Five') } });
		expect(withoutIcon.container.querySelector('svg')).toBeNull();
	});

	it('passes class through to the root', () => {
		const { container } = render(Badge, { props: { class: 'extra', children: text('Six') } });
		expect(container.querySelector('.badge')?.classList.contains('extra')).toBe(true);
	});
});

describe('Tag', () => {
	it('renders the label without a remove control when not removable', () => {
		const { container } = render(Tag, { props: { label: 'Budget' } });
		expect(container.querySelector('.text')?.textContent).toBe('Budget');
		expect(container.querySelector('button')).toBeNull();
		expect(container.querySelector('.tag')?.getAttribute('tabindex')).toBeNull();
	});

	it('names the remove button with the label', () => {
		const { container } = render(Tag, { props: { label: 'Budget', removable: true } });
		const button = container.querySelector('button');
		expect(button?.getAttribute('aria-label')).toBe(t('u052atoms.remove_tag', { label: 'Budget' }));
		expect(button?.getAttribute('aria-label')).toContain('Budget');
	});

	it('calls onremove when the remove button is clicked', async () => {
		const onremove = vi.fn();
		const { container } = render(Tag, { props: { label: 'Budget', removable: true, onremove } });
		await fireEvent.click(container.querySelector('button') as HTMLButtonElement);
		expect(onremove).toHaveBeenCalledTimes(1);
	});

	it('calls onremove on Backspace when the tag is focused', async () => {
		const onremove = vi.fn();
		const { container } = render(Tag, { props: { label: 'Budget', removable: true, onremove } });
		await fireEvent.keyDown(container.querySelector('.tag') as HTMLElement, { key: 'Backspace' });
		expect(onremove).toHaveBeenCalledTimes(1);
	});

	it('ignores other keys', async () => {
		const onremove = vi.fn();
		const { container } = render(Tag, { props: { label: 'Budget', removable: true, onremove } });
		await fireEvent.keyDown(container.querySelector('.tag') as HTMLElement, { key: 'a' });
		expect(onremove).not.toHaveBeenCalled();
	});

	it('does not remove on Backspace when not removable', async () => {
		const onremove = vi.fn();
		const { container } = render(Tag, { props: { label: 'Budget', onremove } });
		await fireEvent.keyDown(container.querySelector('.tag') as HTMLElement, { key: 'Backspace' });
		expect(onremove).not.toHaveBeenCalled();
	});

	it('exposes the tone as a data attribute', () => {
		const { container } = render(Tag, { props: { label: 'Budget', tone: 'accent' } });
		expect(container.querySelector('.tag')?.getAttribute('data-tone')).toBe('accent');
	});
});

describe('Avatar', () => {
	it('shows initials of the first two words', () => {
		const { container } = render(Avatar, { props: { name: 'Ada Byron Lovelace' } });
		expect(container.querySelector('.avatar')?.textContent).toBe('AB');
	});

	it('shows one initial for a single word', () => {
		const { container } = render(Avatar, { props: { name: 'cher' } });
		expect(container.querySelector('.avatar')?.textContent).toBe('C');
	});

	it('is hidden from assistive technology', () => {
		const { container } = render(Avatar, { props: { name: 'Ada' } });
		expect(container.querySelector('.avatar')?.getAttribute('aria-hidden')).toBe('true');
	});

	it('keeps the same tone for the same name', () => {
		const first = render(Avatar, { props: { name: 'Grace Hopper' } });
		const second = render(Avatar, { props: { name: 'Grace Hopper' } });
		expect(first.container.querySelector('.avatar')?.getAttribute('data-tone')).toBe(
			second.container.querySelector('.avatar')?.getAttribute('data-tone')
		);
	});

	it('applies the size as a data attribute', () => {
		const { container } = render(Avatar, { props: { name: 'Ada', size: 'lg' } });
		expect(container.querySelector('.avatar')?.getAttribute('data-size')).toBe('lg');
	});

	it('renders no initials for an empty name', () => {
		const { container } = render(Avatar, { props: { name: '' } });
		expect(container.querySelector('.avatar')?.textContent).toBe('');
	});
});
