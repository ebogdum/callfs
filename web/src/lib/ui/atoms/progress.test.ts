import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { formatBytes } from '$lib/format/bytes';
import { formatNumber } from '$lib/format/number';
import Meter from './Meter.svelte';
import ProgressBar from './ProgressBar.svelte';
import ProgressRing from './ProgressRing.svelte';
import Skeleton from './Skeleton.svelte';

describe('ProgressBar', () => {
	afterEach(() => {
		cleanup();
	});

	it('exposes a progressbar with the label as its accessible name and the bounds', () => {
		render(ProgressBar, { props: { value: 40, label: 'Upload progress' } });
		const track = screen.getByRole('progressbar', { name: 'Upload progress' });
		expect(track.getAttribute('aria-valuemin')).toBe('0');
		expect(track.getAttribute('aria-valuemax')).toBe('100');
		expect(track.getAttribute('aria-valuenow')).toBe('40');
		expect(track.getAttribute('aria-valuetext')).toBe('40 %');
	});

	it('clamps the reported value to max', () => {
		render(ProgressBar, { props: { value: 250, label: 'Copy' } });
		expect(screen.getByRole('progressbar', { name: 'Copy' }).getAttribute('aria-valuenow')).toBe('100');
	});

	it('omits value attributes and marks the root when indeterminate', () => {
		const { container } = render(ProgressBar, { props: { label: 'Working', indeterminate: true } });
		const track = screen.getByRole('progressbar', { name: 'Working' });
		expect(track.getAttribute('aria-valuenow')).toBeNull();
		expect(track.getAttribute('aria-valuetext')).toBeNull();
		expect(container.querySelector('.progress-bar')?.getAttribute('data-indeterminate')).toBe('true');
	});

	it('treats a missing value as indeterminate', () => {
		const { container } = render(ProgressBar, { props: { label: 'Scanning' } });
		expect(container.querySelector('.progress-bar')?.getAttribute('data-indeterminate')).toBe('true');
	});

	it('defaults to the accent tone and applies the requested tone', () => {
		const { container, unmount } = render(ProgressBar, { props: { value: 10, label: 'A' } });
		expect(container.querySelector('.progress-bar')?.getAttribute('data-tone')).toBe('accent');
		unmount();
		const second = render(ProgressBar, { props: { value: 10, label: 'B', tone: 'warn' } });
		expect(second.container.querySelector('.progress-bar')?.getAttribute('data-tone')).toBe('warn');
	});

	it('shows the percentage text only when showValue is set', () => {
		const { container, unmount } = render(ProgressBar, { props: { value: 40, label: 'Plain' } });
		expect(container.querySelector('.value')).toBeNull();
		unmount();
		const shown = render(ProgressBar, { props: { value: 40, label: 'Shown', showValue: true } });
		expect(shown.container.querySelector('.value')?.textContent).toBe(`${formatNumber(40, 'int')} %`);
	});

	it('forwards class to the root', () => {
		const { container } = render(ProgressBar, { props: { value: 5, label: 'Forwarded', class: 'extra' } });
		expect(container.querySelector('.progress-bar')?.classList.contains('extra')).toBe(true);
	});
});

describe('ProgressRing', () => {
	afterEach(() => {
		cleanup();
	});

	it('exposes a progressbar with the label, bounds and percentage text', () => {
		render(ProgressRing, { props: { value: 25, label: 'Verifying' } });
		const ring = screen.getByRole('progressbar', { name: 'Verifying' });
		expect(ring.getAttribute('aria-valuemin')).toBe('0');
		expect(ring.getAttribute('aria-valuemax')).toBe('100');
		expect(ring.getAttribute('aria-valuenow')).toBe('25');
		expect(ring.getAttribute('aria-valuetext')).toBe('25 %');
	});

	it('clamps the value to max and scales against a custom max', () => {
		render(ProgressRing, { props: { value: 3, max: 4, label: 'Pieces' } });
		const ring = screen.getByRole('progressbar', { name: 'Pieces' });
		expect(ring.getAttribute('aria-valuemax')).toBe('4');
		expect(ring.getAttribute('aria-valuenow')).toBe('3');
		expect(ring.getAttribute('aria-valuetext')).toBe(`${formatNumber(75, 'int')} %`);
	});

	it('clamps values above max', () => {
		render(ProgressRing, { props: { value: 150, label: 'Over' } });
		expect(screen.getByRole('progressbar', { name: 'Over' }).getAttribute('aria-valuenow')).toBe('100');
	});

	it('sets the size on the root and hides the drawing from assistive technology', () => {
		const { container } = render(ProgressRing, { props: { value: 1, label: 'Sized', size: 'lg' } });
		const root = container.querySelector('.progress-ring');
		expect(root?.getAttribute('data-size')).toBe('lg');
		expect(container.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
	});

	it('forwards class to the root', () => {
		const { container } = render(ProgressRing, { props: { value: 1, label: 'Forwarded', class: 'extra' } });
		expect(container.querySelector('.progress-ring')?.classList.contains('extra')).toBe(true);
	});
});

describe('Meter', () => {
	afterEach(() => {
		cleanup();
	});

	it('exposes a meter with the label, bounds and the unit text', () => {
		render(Meter, { props: { value: 71, max: 100, label: 'Disk use', unit: 'percent' } });
		const meter = screen.getByRole('meter', { name: 'Disk use' });
		expect(meter.getAttribute('aria-valuemin')).toBe('0');
		expect(meter.getAttribute('aria-valuemax')).toBe('100');
		expect(meter.getAttribute('aria-valuenow')).toBe('71');
		expect(meter.getAttribute('aria-valuetext')).toBe(`${formatNumber(71, 'decimal')} %`);
	});

	it('formats byte units through the byte formatter', () => {
		render(Meter, { props: { value: 1536, max: 2048, label: 'Used', unit: 'bytes' } });
		expect(screen.getByRole('meter', { name: 'Used' }).getAttribute('aria-valuetext')).toBe(formatBytes(1536).text);
	});

	it('clamps the reported value to max', () => {
		render(Meter, { props: { value: 150, max: 100, label: 'Over' } });
		expect(screen.getByRole('meter', { name: 'Over' }).getAttribute('aria-valuenow')).toBe('100');
	});

	it('fills with the tone of the highest crossed threshold', () => {
		const thresholds = [
			{ value: 70, tone: 'warn' as const, label: 'Warning at 70 %' },
			{ value: 90, tone: 'danger' as const, label: 'Danger at 90 %' }
		];
		const danger = render(Meter, { props: { value: 95, max: 100, label: 'Danger', thresholds } });
		expect(danger.container.querySelector('.meter')?.getAttribute('data-tone')).toBe('danger');
		danger.unmount();
		const warn = render(Meter, { props: { value: 75, max: 100, label: 'Warn', thresholds } });
		expect(warn.container.querySelector('.meter')?.getAttribute('data-tone')).toBe('warn');
		warn.unmount();
		const calm = render(Meter, { props: { value: 10, max: 100, label: 'Calm', thresholds } });
		expect(calm.container.querySelector('.meter')?.getAttribute('data-tone')).toBe('accent');
	});

	it('draws one tick per threshold with its label as tooltip', () => {
		const { container } = render(Meter, {
			props: {
				value: 10,
				max: 100,
				label: 'Ticks',
				thresholds: [
					{ value: 70, tone: 'warn', label: 'Warning at 70 %' },
					{ value: 90, tone: 'danger', label: 'Danger at 90 %' }
				]
			}
		});
		const ticks = container.querySelectorAll('.tick');
		expect(ticks.length).toBe(2);
		expect(ticks[0]?.getAttribute('title')).toBe('Warning at 70 %');
		expect(ticks[1]?.getAttribute('title')).toBe('Danger at 90 %');
	});
});

describe('Skeleton', () => {
	afterEach(() => {
		cleanup();
	});

	it('is hidden from assistive technology and has no role', () => {
		const { container } = render(Skeleton);
		const root = container.querySelector('.skeleton');
		expect(root?.getAttribute('aria-hidden')).toBe('true');
		expect(root?.getAttribute('role')).toBeNull();
	});

	it('renders one text line by default', () => {
		const { container } = render(Skeleton);
		expect(container.querySelector('.skeleton')?.getAttribute('data-shape')).toBe('text');
		expect(container.querySelectorAll('.line').length).toBe(1);
	});

	it('renders the requested number of text lines', () => {
		const { container } = render(Skeleton, { props: { lines: 3 } });
		expect(container.querySelectorAll('.line').length).toBe(3);
	});

	it('renders a single block for rect and circle shapes', () => {
		const rect = render(Skeleton, { props: { shape: 'rect', lines: 4 } });
		expect(rect.container.querySelectorAll('.block').length).toBe(1);
		expect(rect.container.querySelector('.line')).toBeNull();
		expect(rect.container.querySelector('.skeleton')?.getAttribute('data-shape')).toBe('rect');
		rect.unmount();
		const circle = render(Skeleton, { props: { shape: 'circle' } });
		expect(circle.container.querySelector('.skeleton')?.getAttribute('data-shape')).toBe('circle');
		expect(circle.container.querySelectorAll('.block').length).toBe(1);
	});

	it('forwards class to the root', () => {
		const { container } = render(Skeleton, { props: { class: 'extra' } });
		expect(container.querySelector('.skeleton')?.classList.contains('extra')).toBe(true);
	});
});
