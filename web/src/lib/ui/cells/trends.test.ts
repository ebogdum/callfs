import { cleanup, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { t } from '$lib/copy/t';
import { formatBytes } from '$lib/format/bytes';
import { formatNumber } from '$lib/format/number';
import { STATUS } from '$lib/status/catalog';
import BarCell from './BarCell.svelte';
import CountCell from './CountCell.svelte';
import DeltaCell from './DeltaCell.svelte';
import ProgressCell from './ProgressCell.svelte';
import SparklineCell from './SparklineCell.svelte';

afterEach(() => {
	cleanup();
});

describe('SparklineCell', () => {
	it('is a labelled image whose polyline skips null values and summarises the trend', () => {
		const { container } = render(SparklineCell, { props: { values: [1, null, 3, 5] } });
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('role')).toBe('img');
		expect(svg?.getAttribute('aria-label')).toBe(t('trends.summary', { trend: t('trends.rising'), count: 4 }));
		expect(container.querySelector('polyline')?.getAttribute('points')?.split(' ')).toHaveLength(3);
	});

	it('labels a falling series as falling', () => {
		const { container } = render(SparklineCell, { props: { values: [5, 1] } });
		expect(container.querySelector('svg')?.getAttribute('aria-label')).toBe(
			t('trends.summary', { trend: t('trends.falling'), count: 2 })
		);
	});

	it('labels a series with equal ends as flat', () => {
		const { container } = render(SparklineCell, { props: { values: [2, 4, 2] } });
		expect(container.querySelector('svg')?.getAttribute('aria-label')).toBe(
			t('trends.summary', { trend: t('trends.flat'), count: 3 })
		);
	});

	it('shows no line and the no-data copy when every value is null', () => {
		const { container } = render(SparklineCell, { props: { values: [null, null] } });
		expect(container.querySelector('svg')?.getAttribute('aria-label')).toBe(t('trends.empty'));
		expect(container.querySelector('polyline')).toBeNull();
	});

	it('exposes the tone on the root', () => {
		const { container } = render(SparklineCell, { props: { values: [1, 2], tone: 'ok' } });
		expect(container.querySelector('svg')?.getAttribute('data-tone')).toBe('ok');
	});
});

describe('BarCell', () => {
	it('sets the fill width from value over max', () => {
		const { container } = render(BarCell, { props: { value: 50, max: 100 } });
		expect(container.querySelector<HTMLElement>('.fill')?.style.width).toBe('50%');
	});

	it('clamps the fill to the full track and to empty', () => {
		const over = render(BarCell, { props: { value: 150, max: 100 } });
		expect(over.container.querySelector<HTMLElement>('.fill')?.style.width).toBe('100%');
		cleanup();
		const noMax = render(BarCell, { props: { value: 5, max: 0 } });
		expect(noMax.container.querySelector<HTMLElement>('.fill')?.style.width).toBe('0%');
	});

	it('renders bytes with the SI text', () => {
		const { container } = render(BarCell, { props: { value: 1500, max: 3000, unit: 'bytes' } });
		expect(container.querySelector('.value')?.textContent).toContain(formatBytes(1500).text);
	});

	it('renders a percent value with its unit', () => {
		const { container } = render(BarCell, { props: { value: 50, max: 100, unit: 'percent' } });
		const text = container.querySelector('.value')?.textContent ?? '';
		expect(text).toContain(formatNumber(50, 'int'));
		expect(text).toContain('%');
	});

	it('defaults the tone to accent and exposes an explicit tone', () => {
		const defaults = render(BarCell, { props: { value: 1, max: 2 } });
		expect(defaults.container.querySelector('.bar-cell')?.getAttribute('data-tone')).toBe('accent');
		cleanup();
		const warned = render(BarCell, { props: { value: 1, max: 2, tone: 'warn' } });
		expect(warned.container.querySelector('.bar-cell')?.getAttribute('data-tone')).toBe('warn');
	});
});

describe('DeltaCell', () => {
	it('shows the period and a good tone when the change moves the right way', () => {
		const { container } = render(DeltaCell, { props: { value: 5, unit: 'count', goodWhen: 'up', period: 'vs previous hour' } });
		const root = container.querySelector('.delta');
		expect(root?.getAttribute('data-tone')).toBe('ok');
		expect(root?.textContent).toContain('vs previous hour');
	});

	it('marks a rise as a danger tone when a fall is good', () => {
		const { container } = render(DeltaCell, { props: { value: 5, unit: 'count', goodWhen: 'down', period: 'vs previous day' } });
		expect(container.querySelector('.delta')?.getAttribute('data-tone')).toBe('danger');
	});

	it('stays neutral when no direction is good', () => {
		const { container } = render(DeltaCell, { props: { value: 5, unit: 'count', goodWhen: 'none', period: 'vs previous day' } });
		expect(container.querySelector('.delta')?.getAttribute('data-tone')).toBe('neutral');
	});
});

describe('ProgressCell', () => {
	it('shows the count of done over total and a labelled progress bar', () => {
		const { container } = render(ProgressCell, { props: { done: 40, total: 120, state: 'transfer.uploading' } });
		const bar = container.querySelector('[role="progressbar"]');
		expect(bar?.getAttribute('aria-valuenow')).toBe('40');
		expect(bar?.getAttribute('aria-valuemax')).toBe('120');
		expect(bar?.getAttribute('aria-label')).toBe(t(STATUS['transfer.uploading'].word));
		expect(container.querySelector('.count')?.textContent).toBe(
			`${formatNumber(40, 'int')} ${t('number.of')} ${formatNumber(120, 'int')}`
		);
	});

	it('shows bytes and throughput with an indeterminate bar when total is unknown', () => {
		const { container } = render(ProgressCell, {
			props: { done: 1500, total: null, state: 'transfer.downloading', bytesPerSecond: 2000000 },
		});
		expect(container.querySelector('.progress-bar')?.getAttribute('data-indeterminate')).toBe('true');
		expect(container.querySelector('.bytes')?.textContent).toBe(formatBytes(1500).text);
		expect(container.querySelector('.speed')?.textContent).toBe(`${formatBytes(2000000).text}/s`);
	});

	it('omits throughput when it is not given', () => {
		const { container } = render(ProgressCell, { props: { done: 1500, total: null, state: 'transfer.queued' } });
		expect(container.querySelector('.speed')).toBeNull();
	});

	it('maps the status tone and treats progress as accent', () => {
		const stalled = render(ProgressCell, { props: { done: 1, total: 2, state: 'transfer.stalled' } });
		expect(stalled.container.querySelector('.progress-cell')?.getAttribute('data-tone')).toBe('warn');
		cleanup();
		const uploading = render(ProgressCell, { props: { done: 1, total: 2, state: 'transfer.uploading' } });
		expect(uploading.container.querySelector('.progress-cell')?.getAttribute('data-tone')).toBe('accent');
	});
});

describe('CountCell', () => {
	it('shows the count first and the rate in brackets', () => {
		const { container } = render(CountCell, { props: { num: 312, den: 1204 } });
		const text = container.querySelector('.percent')?.textContent ?? '';
		expect(text).toContain(formatNumber(312, 'int'));
		expect(text).toContain(`(${Math.round((100 * 312) / 1204)}%)`);
	});

	it('shows the count without a rate when the base is zero', () => {
		const { container } = render(CountCell, { props: { num: 3, den: 0 } });
		expect(container.querySelector('.percent')?.textContent?.trim()).toBe(formatNumber(3, 'int'));
	});

	it('forwards the class to the root', () => {
		const { container } = render(CountCell, { props: { num: 1, den: 2, class: 'extra' } });
		expect(container.querySelector('.percent.extra')).not.toBeNull();
	});
});
