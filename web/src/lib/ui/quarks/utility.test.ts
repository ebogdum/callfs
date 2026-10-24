import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { announce } from './announce';
import FocusRing from './FocusRing.svelte';
import Hairline from './Hairline.svelte';
import { collapse, fadeIn, holdMinimum, holdRemainingMs, prefersReducedMotion, rise, scaleIn, slideIn } from './motion';
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

describe('motion', () => {
	afterEach(() => {
		delete document.documentElement.dataset.motion;
		vi.restoreAllMocks();
	});

	it('follows the data-motion attribute before the OS preference', () => {
		document.documentElement.dataset.motion = 'reduced';
		expect(prefersReducedMotion()).toBe(true);
		document.documentElement.dataset.motion = 'none';
		expect(prefersReducedMotion()).toBe(true);
		vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: true } as MediaQueryList);
		document.documentElement.dataset.motion = 'full';
		expect(prefersReducedMotion()).toBe(false);
	});

	it('falls back to the OS preference when data-motion is unset', () => {
		vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: true } as MediaQueryList);
		expect(prefersReducedMotion()).toBe(true);
	});

	it('uses token durations when motion is full', () => {
		document.documentElement.dataset.motion = 'full';
		const element = document.createElement('div');
		expect(fadeIn(element).duration).toBe(120);
		expect(rise(element).duration).toBe(220);
		expect(rise(element, {}, { direction: 'out' }).duration).toBe(150);
		expect(scaleIn(element).duration).toBe(220);
		expect(slideIn(element, { side: 'left' }).duration).toBe(280);
		expect(collapse(element).duration).toBe(220);
	});

	it('returns zero-duration transitions under reduced motion', () => {
		document.documentElement.dataset.motion = 'reduced';
		const element = document.createElement('div');
		expect(fadeIn(element).duration).toBe(0);
		expect(rise(element).duration).toBe(0);
		expect(scaleIn(element).duration).toBe(0);
		expect(slideIn(element, { side: 'right' }).duration).toBe(0);
		expect(collapse(element).duration).toBe(0);
	});

	it('writes the eased css for each transition', () => {
		document.documentElement.dataset.motion = 'full';
		const element = document.createElement('div');
		expect(fadeIn(element).css?.(0.5, 0.5)).toBe('opacity: 0.5');
		expect(slideIn(element, { side: 'left' }).css?.(0.5, 0.5)).toBe('transform: translateX(-50%)');
		expect(scaleIn(element).css?.(1, 1)).toContain('scale(1)');
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

describe('FocusRing', () => {
	afterEach(() => {
		cleanup();
	});

	it('wraps its children and keeps their accessible name', () => {
		render(FocusRing, {
			props: { children: createRawSnippet(() => ({ render: () => '<button type="button">Open</button>' })) }
		});
		expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument();
	});

	it('marks the wrapper as inset when requested', () => {
		const { container } = render(FocusRing, { props: { children: textSnippet('Canvas'), inset: true } });
		expect(container.querySelector('.focus-ring')?.classList.contains('inset')).toBe(true);
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
