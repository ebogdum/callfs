import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { announce } from './announce';
import Hairline from './Hairline.svelte';
import { holdMinimum, holdRemainingMs } from './motion';
import Spinner from './Spinner.svelte';
import VisuallyHidden from './VisuallyHidden.svelte';

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

describe('Spinner', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		cleanup();
		vi.useRealTimers();
	});

	it('stays hidden until the default 150ms delay has elapsed', async () => {
		const { container } = render(Spinner);
		await tick();
		vi.advanceTimersByTime(149);
		await tick();
		expect(container.querySelector('.spinner')).toBeNull();
		vi.advanceTimersByTime(1);
		await tick();
		expect(container.querySelector('.spinner')).not.toBeNull();
	});

	it('honours a custom delay', async () => {
		const { container } = render(Spinner, { props: { delay: 500 } });
		await tick();
		vi.advanceTimersByTime(499);
		await tick();
		expect(container.querySelector('.spinner')).toBeNull();
		vi.advanceTimersByTime(1);
		await tick();
		expect(container.querySelector('.spinner')).not.toBeNull();
	});

	it('exposes a labelled status when a label is given', async () => {
		render(Spinner, { props: { label: 'Saving', delay: 0 } });
		await tick();
		vi.advanceTimersByTime(0);
		await tick();
		expect(screen.getByRole('status')).toHaveTextContent('Saving');
	});

	it('is hidden from assistive technology when unlabelled', async () => {
		const { container } = render(Spinner, { props: { delay: 0 } });
		await tick();
		vi.advanceTimersByTime(0);
		await tick();
		expect(container.querySelector('.spinner')?.getAttribute('aria-hidden')).toBe('true');
	});

	it('keeps the spinner visible for at least 400ms after it was shown', () => {
		vi.setSystemTime(new Date(1000));
		expect(holdRemainingMs(1000, 1000)).toBe(400);
		expect(holdRemainingMs(1000, 1399)).toBe(1);
		expect(holdRemainingMs(1000, 1400)).toBe(0);
		expect(holdRemainingMs(1000, 2000)).toBe(0);
	});

	it('holds the outro for the remaining minimum visible time', () => {
		vi.setSystemTime(new Date(1200));
		const config = holdMinimum(document.createElement('span'), { shownAt: 1000 });
		expect(config.duration).toBe(200);
	});
});

describe('announce', () => {
	const frames: FrameRequestCallback[] = [];

	const flushFrames = () => {
		for (const callback of frames.splice(0)) {
			callback(0);
		}
	};

	beforeEach(() => {
		vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
			frames.push(callback);
			return frames.length;
		});
		document.body.innerHTML = '<div id="live-polite"></div><div id="live-assertive"></div>';
	});

	afterEach(() => {
		vi.restoreAllMocks();
		frames.splice(0);
		document.body.innerHTML = '';
	});

	it('clears the polite region, then sets the text on the next frame', () => {
		const region = document.getElementById('live-polite');
		announce('Saved', 'polite');
		expect(region?.textContent).toBe('');
		flushFrames();
		expect(region?.textContent).toBe('Saved');
	});

	it('writes assertive announcements to the assertive region', () => {
		announce('Connection lost', 'assertive');
		flushFrames();
		expect(document.getElementById('live-assertive')?.textContent).toBe('Connection lost');
		expect(document.getElementById('live-polite')?.textContent).toBe('');
	});

	it('clears a repeated message so the repeat is read again', () => {
		announce('Copied', 'polite');
		flushFrames();
		announce('Copied', 'polite');
		expect(document.getElementById('live-polite')?.textContent).toBe('');
		flushFrames();
		expect(document.getElementById('live-polite')?.textContent).toBe('Copied');
	});

	it('does nothing when the region is not rendered', () => {
		document.body.innerHTML = '';
		expect(() => announce('Saved', 'polite')).not.toThrow();
		expect(frames).toHaveLength(0);
	});
});

describe('Hairline', () => {
	afterEach(() => {
		cleanup();
	});

	it('is hidden from assistive technology by default', () => {
		const { container } = render(Hairline);
		const hairline = container.querySelector('.hairline');
		expect(hairline?.getAttribute('aria-hidden')).toBe('true');
		expect(hairline?.getAttribute('role')).toBeNull();
	});

	it('is a named separator with orientation when semantic', () => {
		render(Hairline, { props: { decorative: false, orientation: 'vertical' } });
		const separator = screen.getByRole('separator');
		expect(separator.getAttribute('aria-orientation')).toBe('vertical');
		expect(separator.getAttribute('aria-hidden')).toBeNull();
	});
});

describe('VisuallyHidden', () => {
	afterEach(() => {
		cleanup();
	});

	it('renders its text for assistive technology', () => {
		render(VisuallyHidden, { props: { children: textSnippet('Opens in a new tab') } });
		expect(screen.getByText('Opens in a new tab')).toBeInTheDocument();
	});

	it('reveals on focus when focusable', () => {
		const { container } = render(VisuallyHidden, {
			props: { children: textSnippet('Skip to content'), focusable: true }
		});
		expect(container.querySelector('.visually-hidden.focusable')).not.toBeNull();
	});
});
