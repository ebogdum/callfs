import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import CopyButton from './CopyButton.svelte';
import MiddleTruncate from './MiddleTruncate.svelte';
import Thumb from './Thumb.svelte';
import Tooltip from './Tooltip.svelte';

const resizeCallbacks: ResizeObserverCallback[] = [];

class ResizeObserverStub {
	constructor(callback: ResizeObserverCallback) {
		resizeCallbacks.push(callback);
	}
	observe(): void {}
	unobserve(): void {}
	disconnect(): void {}
}

const buttonSnippet = (label: string) =>
	createRawSnippet(() => ({ render: () => `<button type="button">${label}</button>` }));

const visibleText = (element: HTMLElement): string =>
	Array.from(element.childNodes)
		.filter((node) => Node.TEXT_NODE === node.nodeType)
		.map((node) => node.textContent ?? '')
		.join('')
		.trim();

const stubLayout = (width: number): void => {
	vi.spyOn(Element.prototype, 'clientWidth', 'get').mockReturnValue(width);
	vi.spyOn(Element.prototype, 'scrollWidth', 'get').mockImplementation(function (this: Element) {
		return (this.textContent ?? '').length * 10;
	});
};

const clipboardWriter = (): { writeText: ReturnType<typeof vi.fn> } => {
	const writeText = vi.fn();
	Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
	return { writeText };
};

describe('Tooltip', () => {
	beforeEach(() => {
		vi.stubGlobal('ResizeObserver', ResizeObserverStub);
	});

	afterEach(() => {
		cleanup();
		vi.unstubAllGlobals();
	});

	it('renders the trigger child and no tooltip while closed', () => {
		render(Tooltip, { props: { content: 'Copy', children: buttonSnippet('Save') } });
		expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
		expect(screen.queryByRole('tooltip')).toBeNull();
	});

	it('opens the string content on focus and closes on focus out', async () => {
		render(Tooltip, { props: { content: 'Copy', children: buttonSnippet('Save') } });
		const trigger = screen.getByRole('button', { name: 'Save' });
		fireEvent.focusIn(trigger);
		await waitFor(() => {
			expect(screen.getByRole('tooltip')).toHaveTextContent('Copy');
		});
		fireEvent.focusOut(trigger);
		await waitFor(() => {
			expect(screen.queryByRole('tooltip')).toBeNull();
		});
	});

	it('renders a snippet as the tooltip content', async () => {
		const reason = createRawSnippet(() => ({ render: () => '<strong>Quorum at risk</strong>' }));
		render(Tooltip, { props: { content: reason, children: buttonSnippet('Remove') } });
		fireEvent.focusIn(screen.getByRole('button', { name: 'Remove' }));
		await waitFor(() => {
			expect(screen.getByRole('tooltip')).toHaveTextContent('Quorum at risk');
		});
	});
});

describe('MiddleTruncate', () => {
	const longName = 'folder-archive-2026-quarterly-financial-summary-final.pdf';

	beforeEach(() => {
		vi.stubGlobal('ResizeObserver', ResizeObserverStub);
		resizeCallbacks.length = 0;
	});

	afterEach(() => {
		cleanup();
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	it('shows the full text in title and aria-label when it fits', async () => {
		stubLayout(1000);
		render(MiddleTruncate, { props: { text: longName } });
		await tick();
		const root = screen.getByLabelText(longName);
		expect(root).toHaveAttribute('title', longName);
		expect(visibleText(root)).toBe(longName);
	});

	it('replaces the middle with an ellipsis when the text overflows', async () => {
		stubLayout(200);
		render(MiddleTruncate, { props: { text: longName } });
		await tick();
		const root = screen.getByLabelText(longName);
		const shown = visibleText(root);
		expect(shown).toContain('…');
		expect(shown.length).toBeLessThanOrEqual(20);
		expect(shown.startsWith('folder-')).toBe(true);
		expect(shown.endsWith(longName.slice(-8))).toBe(true);
	});

	it('never truncates a short number or identifier', async () => {
		stubLayout(40);
		const identifier = 'INV-2026-000481';
		render(MiddleTruncate, { props: { text: identifier } });
		await tick();
		expect(visibleText(screen.getByLabelText(identifier))).toBe(identifier);
	});

	it('refits when the container resizes', async () => {
		stubLayout(1000);
		render(MiddleTruncate, { props: { text: longName } });
		await tick();
		vi.spyOn(Element.prototype, 'clientWidth', 'get').mockReturnValue(200);
		for (const callback of resizeCallbacks) {
			callback([], {} as ResizeObserver);
		}
		await tick();
		expect(visibleText(screen.getByLabelText(longName))).toContain('…');
	});

	it('sets the mono marker when mono is requested', async () => {
		stubLayout(1000);
		render(MiddleTruncate, { props: { text: longName, mono: true } });
		await tick();
		expect(screen.getByLabelText(longName)).toHaveAttribute('data-mono');
	});
});

describe('CopyButton', () => {
	let liveRegion: HTMLDivElement;

	beforeEach(() => {
		liveRegion = document.createElement('div');
		liveRegion.id = 'live-polite';
		document.body.append(liveRegion);
	});

	afterEach(() => {
		cleanup();
		liveRegion.remove();
		vi.useRealTimers();
		Reflect.deleteProperty(navigator, 'clipboard');
	});

	it('names the icon variant with its label', () => {
		render(CopyButton, { props: { value: 'abc' } });
		const button = screen.getByRole('button', { name: 'Copy' });
		expect(button).toHaveAttribute('data-variant', 'icon');
	});

	it('names the button variant with its visible label', () => {
		render(CopyButton, { props: { value: 'abc', label: 'Copy link', variant: 'button' } });
		expect(screen.getByRole('button', { name: /Copy link/ })).toHaveAttribute('data-variant', 'button');
	});

	it('copies the value and announces the result', async () => {
		const { writeText } = clipboardWriter();
		writeText.mockResolvedValue(undefined);
		render(CopyButton, { props: { value: 'abc' } });
		const button = screen.getByRole('button', { name: 'Copy' });
		fireEvent.click(button);
		expect(writeText).toHaveBeenCalledWith('abc');
		await waitFor(() => {
			expect(button).toHaveAttribute('data-state', 'copied');
		});
		await waitFor(() => {
			expect(liveRegion.textContent).toBe('Copied');
		});
	});

	it('reports a failed copy with plain copy', async () => {
		const { writeText } = clipboardWriter();
		writeText.mockRejectedValue(new Error('denied'));
		render(CopyButton, { props: { value: 'abc' } });
		const button = screen.getByRole('button', { name: 'Copy' });
		fireEvent.click(button);
		await waitFor(() => {
			expect(button).toHaveAttribute('data-state', 'failed');
		});
		await waitFor(() => {
			expect(liveRegion.textContent).toBe('Copy failed.');
		});
	});

	it('returns to the idle glyph 1.5 seconds after copying', async () => {
		vi.useFakeTimers();
		const { writeText } = clipboardWriter();
		writeText.mockResolvedValue(undefined);
		render(CopyButton, { props: { value: 'abc' } });
		const button = screen.getByRole('button', { name: 'Copy' });
		fireEvent.click(button);
		await vi.advanceTimersByTimeAsync(0);
		await tick();
		expect(button).toHaveAttribute('data-state', 'copied');
		vi.advanceTimersByTime(1499);
		await tick();
		expect(button).toHaveAttribute('data-state', 'copied');
		vi.advanceTimersByTime(1);
		await tick();
		expect(button).toHaveAttribute('data-state', 'idle');
	});
});

describe('Thumb', () => {
	afterEach(() => {
		cleanup();
	});

	it('renders a lazy, async-decoded image with its alt text', () => {
		render(Thumb, { props: { src: '/cover.png', alt: 'Cover' } });
		const image = screen.getByRole('img', { name: 'Cover' });
		expect(image).toHaveAttribute('loading', 'lazy');
		expect(image).toHaveAttribute('decoding', 'async');
	});

	it('applies the default 4 / 3 aspect ratio', () => {
		const { container } = render(Thumb, { props: { src: '/cover.png', alt: 'Cover' } });
		expect(container.querySelector('.frame')?.getAttribute('style')).toContain('4 / 3');
	});

	it('applies a requested aspect ratio and passes the class through', () => {
		const { container } = render(Thumb, {
			props: { src: '/cover.png', alt: 'Cover', ratio: '16/9', class: 'extra' }
		});
		const frame = container.querySelector('.frame');
		expect(frame?.getAttribute('style')).toContain('16 / 9');
		expect(container.querySelector('.thumb.extra')).not.toBeNull();
	});
});
