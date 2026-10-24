import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import { base } from '$app/paths';
import type { Series } from '../../types';
import TimeSeriesChart from './TimeSeriesChart.svelte';
import { CrosshairState, crosshairGroup } from './crosshair.svelte';
import {
	downsampleSeries,
	linearScale,
	lttb,
	nearestIndex,
	niceTicks,
	tickDigits,
	timeScale,
	type Point,
	type SeriesPoint
} from './scale';

const MINUTE = 60_000;
const HOUR = 3_600_000;

const samples = (values: (number | null)[]): SeriesPoint[] => values.map((value, index): SeriesPoint => [index * MINUTE, value]);

const okSeries: Series = { id: 'ok', label: 'OK', unit: 'count', points: samples([10, 12, 14, 16]) };
const errorSeries: Series = { id: 'errors', label: 'Errors', unit: 'count', points: samples([0, 1, null, 2]) };

const manySeries: Series[] = Array.from({ length: 6 }, (_, index) => ({
	id: `s${index + 1}`,
	label: `S${index + 1}`,
	unit: 'count' as const,
	points: samples([index, index + 1])
}));

afterEach(() => {
	cleanup();
});

describe('scale', () => {
	it('produces nice ticks inside the domain', () => {
		expect(niceTicks(0, 97, 5)).toEqual([0, 20, 40, 60, 80]);
	});

	it('maps values to pixels and back', () => {
		const scale = linearScale([0, 10], [0, 100]);
		expect(scale.apply(5)).toBe(50);
		expect(scale.invert(50)).toBe(5);
	});

	it('spaces time ticks on a readable step', () => {
		expect(timeScale([0, 2 * HOUR], [0, 100]).ticks(4)).toHaveLength(5);
	});

	it('keeps the first and last point when downsampling with LTTB', () => {
		const input = Array.from({ length: 100 }, (_, index): Point => [index, Math.sin(index)]);
		const output = lttb(input, 10);
		expect(output).toHaveLength(10);
		expect(output[0]).toEqual(input[0]);
		expect(output[9]).toEqual(input[99]);
	});

	it('keeps a single gap marker between runs when downsampling', () => {
		const input = samples(Array.from({ length: 40 }, (_, index) => (20 === index ? null : index)));
		const output = downsampleSeries(input, 10);
		expect(output.filter(([, y]) => null === y)).toHaveLength(1);
		expect(output.length).toBeLessThanOrEqual(12);
	});

	it('finds the nearest sorted index', () => {
		expect(nearestIndex([0, 10, 20], 14)).toBe(1);
		expect(nearestIndex([], 5)).toBe(-1);
	});
});

describe('crosshair', () => {
	it('shares one position per sync key', () => {
		const shared = crosshairGroup('crosshair-test');
		expect(crosshairGroup('crosshair-test')).toBe(shared);
		expect(shared).toBeInstanceOf(CrosshairState);
		shared.place(5, 'a');
		expect(crosshairGroup('crosshair-test').time).toBe(5);
		shared.clear('b');
		expect(shared.time).toBe(5);
		shared.clear('a');
		expect(shared.time).toBeNull();
	});
});

describe('TimeSeriesChart', () => {
	it('renders the plot with a summary label', () => {
		render(TimeSeriesChart, { props: { series: [okSeries, errorSeries], unit: 'count', status: 'success' } });
		expect(screen.getByRole('img', { name: t('chart.summary', { count: 2, names: 'OK, Errors' }) })).toBeTruthy();
	});

	it('shows a skeleton while loading', () => {
		const { container } = render(TimeSeriesChart, { props: { series: [okSeries], unit: 'count', status: 'loading' } });
		expect(container.querySelector('[data-status="loading"]')).not.toBeNull();
		expect(screen.queryByRole('img')).toBeNull();
	});

	it('shows the empty range state by default', () => {
		render(TimeSeriesChart, { props: { series: [], unit: 'count', status: 'empty' } });
		expect(screen.getByText(t('chart.empty_range.title'))).toBeTruthy();
	});

	it('shows the no samples state', () => {
		render(TimeSeriesChart, {
			props: { series: [], unit: 'count', status: 'empty', emptyReason: 'no_samples' }
		});
		expect(screen.getByText(t('chart.no_samples.title'))).toBeTruthy();
	});

	it('shows the not reported state with a link to the metrics catalog', () => {
		render(TimeSeriesChart, {
			props: { series: [], unit: 'count', status: 'empty', emptyReason: 'not_reported' }
		});
		expect(screen.getByText(t('chart.not_reported.title'))).toBeTruthy();
		expect(screen.getByRole('link', { name: t('chart.not_reported.action') }).getAttribute('href')).toBe(
			`${base}/insights/metrics`
		);
	});

	it('shows the query error state and calls onretry', async () => {
		const onretry = vi.fn();
		render(TimeSeriesChart, { props: { series: [], unit: 'count', status: 'error', onretry } });
		expect(screen.getByText(t('chart.error.title'))).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));
		expect(onretry).toHaveBeenCalledOnce();
	});

	it('shows a partial alert above the chart', () => {
		render(TimeSeriesChart, { props: { series: [okSeries], unit: 'count', status: 'partial' } });
		expect(screen.getByText(t('chart.partial.title'))).toBeTruthy();
		expect(screen.getByRole('img')).toBeTruthy();
	});

	it('moves the shared crosshair with the arrow keys', async () => {
		render(TimeSeriesChart, {
			props: { series: [okSeries, errorSeries], unit: 'count', status: 'success', syncKey: 'keyboard-test' }
		});
		const plot = screen.getByRole('img');
		await fireEvent.keyDown(plot, { key: 'ArrowRight' });
		expect(crosshairGroup('keyboard-test').time).toBe(0);
		await fireEvent.keyDown(plot, { key: 'ArrowRight' });
		expect(crosshairGroup('keyboard-test').time).toBe(MINUTE);
		await fireEvent.keyDown(plot, { key: 'ArrowLeft' });
		expect(crosshairGroup('keyboard-test').time).toBe(0);
	});

	it('keeps a screen reader data table without a toggle button', () => {
		render(TimeSeriesChart, { props: { series: [okSeries, errorSeries], unit: 'count', status: 'success' } });
		expect(screen.queryByRole('button', { name: 'Show data table' })).toBeNull();
		expect(screen.getByRole('columnheader', { name: t('chart.table.time') })).toBeTruthy();
		expect(screen.getByRole('columnheader', { name: 'Errors' })).toBeTruthy();
	});

	it('uses a legend instead of end labels for many series', () => {
		const { container } = render(TimeSeriesChart, {
			props: { series: manySeries, unit: 'count', status: 'success' }
		});
		expect(container.querySelectorAll('text.end-label')).toHaveLength(0);
		expect(screen.getByRole('button', { name: 'S6' })).toBeTruthy();
	});

	it('shows a legend row for two series', () => {
		render(TimeSeriesChart, { props: { series: [okSeries, errorSeries], unit: 'count', status: 'success' } });
		expect(screen.getByRole('button', { name: 'Errors' })).toBeTruthy();
	});
});

describe('tickDigits', () => {
	it('keeps small tick steps distinct', () => {
		expect(tickDigits(0.05)).toBe(2);
		expect(tickDigits(0.1)).toBe(1);
		expect(tickDigits(1)).toBe(0);
		expect(tickDigits(5)).toBe(0);
	});
});
