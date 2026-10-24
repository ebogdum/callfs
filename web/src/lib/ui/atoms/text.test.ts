import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Code from './Code.svelte';
import Heading from './Heading.svelte';
import Kbd from './Kbd.svelte';
import Term from './Term.svelte';
import Text from './Text.svelte';

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const flushPromises = async () => {
	await Promise.resolve();
	await Promise.resolve();
	await tick();
};

const setClipboard = (writeText: ((text: string) => Promise<void>) | undefined) => {
	Object.defineProperty(navigator, 'clipboard', {
		value: undefined === writeText ? undefined : { writeText },
		configurable: true
	});
};

afterEach(() => {
	cleanup();
	setClipboard(undefined);
});

describe('Text', () => {
	it('renders a span with the given text and merged class', () => {
		const { container } = render(Text, {
			props: { class: 'extra', children: textSnippet('Hello') }
		});
		const element = container.querySelector('span.text');
		expect(element?.textContent).toBe('Hello');
		expect(element?.classList.contains('extra')).toBe(true);
	});

	it('renders the element named by as', () => {
		const { container } = render(Text, {
			props: { as: 'p', children: textSnippet('Para') }
		});
		expect(container.querySelector('p.text')?.textContent).toBe('Para');
	});

	it('passes rest attributes to the root element', () => {
		const { container } = render(Text, {
			props: { 'data-testid': 'label', children: textSnippet('x') }
		});
		expect(container.querySelector('[data-testid="label"]')).not.toBeNull();
	});

	it.each([
		['caption', 'var(--text-caption-size)'],
		['sm', 'var(--text-sm-size)'],
		['body', 'var(--text-body-size)'],
		['title-sm', 'var(--text-title-sm-size)'],
		['title', 'var(--text-title-size)'],
		['heading', 'var(--text-heading-size)'],
		['display', 'var(--text-display-size)']
	] as const)('size %s maps to its size token', (size, token) => {
		const { container } = render(Text, {
			props: { size, children: textSnippet('x') }
		});
		const element = container.querySelector<HTMLElement>('.text');
		expect(element?.style.getPropertyValue('--text-size-value')).toBe(token);
	});

	it('defaults to body size and the default tone', () => {
		const { container } = render(Text, { props: { children: textSnippet('x') } });
		const element = container.querySelector<HTMLElement>('.text');
		expect(element?.style.getPropertyValue('--text-size-value')).toBe('var(--text-body-size)');
		expect(element?.style.getPropertyValue('--text-tone-color')).toBe('var(--text)');
	});

	it.each([
		['default', 'var(--text)'],
		['muted', 'var(--text-muted)'],
		['faint', 'var(--text-faint)'],
		['accent', 'var(--accent-text)'],
		['ok', 'var(--ok)'],
		['warn', 'var(--warn)'],
		['danger', 'var(--danger)']
	] as const)('tone %s sets its colour token', (tone, token) => {
		const { container } = render(Text, {
			props: { tone, children: textSnippet('x') }
		});
		const element = container.querySelector<HTMLElement>('.text');
		expect(element?.style.getPropertyValue('--text-tone-color')).toBe(token);
	});

	it('applies an explicit weight and falls back to the size weight token', () => {
		const { container, unmount } = render(Text, {
			props: { weight: 600, children: textSnippet('x') }
		});
		expect(container.querySelector<HTMLElement>('.text')?.style.getPropertyValue('--text-weight-value')).toBe('600');
		unmount();
		const second = render(Text, { props: { children: textSnippet('y') } });
		expect(second.container.querySelector<HTMLElement>('.text')?.style.getPropertyValue('--text-weight-value')).toBe(
			'var(--text-body-weight)'
		);
	});

	it('sets mono, tabular, balance and truncate classes from props', () => {
		const { container } = render(Text, {
			props: { mono: true, tabular: true, balance: true, truncate: true, children: textSnippet('x') }
		});
		const element = container.querySelector('.text');
		expect(element?.classList.contains('mono')).toBe(true);
		expect(element?.classList.contains('tabular')).toBe(true);
		expect(element?.classList.contains('balance')).toBe(true);
		expect(element?.classList.contains('truncate')).toBe(true);
		expect(element?.classList.contains('clamp')).toBe(false);
	});

	it('clamps to the given number of lines when truncate is 2', () => {
		const { container } = render(Text, {
			props: { truncate: 2, children: textSnippet('x') }
		});
		const element = container.querySelector<HTMLElement>('.text');
		expect(element?.classList.contains('clamp')).toBe(true);
		expect(element?.classList.contains('truncate')).toBe(false);
		expect(element?.style.getPropertyValue('--text-clamp')).toBe('2');
	});
});

describe('Heading', () => {
	it('renders the heading level with an accessible name', () => {
		render(Heading, { props: { level: 2, children: textSnippet('Sharing') } });
		expect(screen.getByRole('heading', { level: 2, name: 'Sharing' })).not.toBeNull();
	});

	it('uses the title size for level 1 by default', () => {
		const { container } = render(Heading, { props: { level: 1, children: textSnippet('Files') } });
		expect(container.querySelector('h1')?.style.getPropertyValue('--text-size-value')).toBe('var(--text-title-size)');
	});

	it('uses the title-sm size for level 2 by default', () => {
		const { container } = render(Heading, { props: { level: 2, children: textSnippet('Files') } });
		expect(container.querySelector('h2')?.style.getPropertyValue('--text-size-value')).toBe(
			'var(--text-title-sm-size)'
		);
	});

	it('uses body size at weight 600 for levels 3 and 4', () => {
		const { container } = render(Heading, { props: { level: 3, children: textSnippet('Group') } });
		const heading = container.querySelector<HTMLElement>('h3');
		expect(heading?.style.getPropertyValue('--text-size-value')).toBe('var(--text-body-size)');
		expect(heading?.style.getPropertyValue('--text-weight-value')).toBe('600');
	});

	it('honours an explicit size', () => {
		const { container } = render(Heading, { props: { level: 1, size: 'display', children: textSnippet('Wall') } });
		expect(container.querySelector('h1')?.style.getPropertyValue('--text-size-value')).toBe(
			'var(--text-display-size)'
		);
	});
});

describe('Code', () => {
	it('renders the code text without a copy button by default', () => {
		const { container } = render(Code, { props: { children: textSnippet('/team/report.pdf') } });
		expect(container.querySelector('code')?.textContent).toBe('/team/report.pdf');
		expect(screen.queryByRole('button')).toBeNull();
	});

	it('shows a labelled copy button when copy is true', () => {
		render(Code, { props: { copy: true, children: textSnippet('abc') } });
		expect(screen.getByRole('button', { name: 'Copy' })).not.toBeNull();
	});

	it('copies the code text and shows the copied label', async () => {
		const writeText = vi.fn((_text: string) => Promise.resolve());
		setClipboard(writeText);
		const { container } = render(Code, { props: { copy: true, children: textSnippet('abc') } });
		await fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
		await flushPromises();
		expect(writeText).toHaveBeenCalledWith('abc');
		expect(container.querySelector('[data-copy-state]')?.getAttribute('data-copy-state')).toBe('copied');
		expect(screen.getByRole('button', { name: 'Copied' })).not.toBeNull();
	});

	it('shows the copy failure label when the clipboard rejects', async () => {
		setClipboard(() => Promise.reject(new Error('denied')));
		render(Code, { props: { copy: true, children: textSnippet('abc') } });
		await fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
		await flushPromises();
		expect(screen.getByRole('button', { name: 'Copy failed.' })).not.toBeNull();
	});

	it('shows the copy failure label when no clipboard is available', async () => {
		setClipboard(undefined);
		render(Code, { props: { copy: true, children: textSnippet('abc') } });
		await fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
		await tick();
		expect(screen.getByRole('button', { name: 'Copy failed.' })).not.toBeNull();
	});
});

describe('Kbd', () => {
	it('renders one kbd element per key', () => {
		const { container } = render(Kbd, { props: { keys: ['shift', 'enter'] } });
		expect(container.querySelectorAll('kbd')).toHaveLength(2);
	});

	it('maps modifier names to their labels on non-Apple platforms', () => {
		const { container } = render(Kbd, { props: { keys: ['mod', 'shift', 'alt'] } });
		const labels = Array.from(container.querySelectorAll('kbd'), (element) => element.textContent);
		expect(labels).toEqual(['Ctrl', 'Shift', 'Alt']);
	});

	it('renders enter as the return symbol', () => {
		const { container } = render(Kbd, { props: { keys: ['enter'] } });
		expect(container.querySelector('kbd')?.textContent).toBe('↵');
	});

	it('renders unknown keys as given', () => {
		const { container } = render(Kbd, { props: { keys: ['F2'] } });
		expect(container.querySelector('kbd')?.textContent).toBe('F2');
	});

	it('passes class and rest attributes to the group', () => {
		const { container } = render(Kbd, { props: { keys: ['/'], class: 'hint', 'data-testid': 'hint' } });
		expect(container.querySelector('[data-testid="hint"]')?.classList.contains('hint')).toBe(true);
	});
});

describe('Term', () => {
	it('renders the glossary word', () => {
		render(Term, { props: { term: 'leader' } });
		expect(screen.getByText('Leader')).not.toBeNull();
	});

	it('renders children in place of the glossary word when given', () => {
		render(Term, { props: { term: 'leader', children: textSnippet('Lead node') } });
		expect(screen.getByText('Lead node')).not.toBeNull();
		expect(screen.queryByText('Leader')).toBeNull();
	});

	it('is keyboard focusable', () => {
		render(Term, { props: { term: 'quorum' } });
		expect(screen.getByText('Quorum').getAttribute('tabindex')).toBe('0');
	});

	it('points aria-describedby at the glossary explanation', () => {
		render(Term, { props: { term: 'quorum' } });
		const trigger = screen.getByText('Quorum');
		const describedBy = trigger.getAttribute('aria-describedby');
		expect(describedBy).not.toBeNull();
		expect(document.getElementById(describedBy ?? '')?.textContent).toBe(
			'Enough cluster members agree to accept changes.'
		);
	});
});
