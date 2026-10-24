import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { t } from '$lib/copy/t';
import BarChart from './BarChart.svelte';
import DonutChart from './DonutChart.svelte';
import Heatmap from './Heatmap.svelte';
import Histogram from './Histogram.svelte';

const barLabels = (container: HTMLElement): string[] =>
	Array.from(container.querySelectorAll('[data-bar]'), (element) => element.getAttribute('data-bar') ?? '');

describe('BarChart', () => {
	it('names the chart image by its label', () => {
		render(BarChart, {
			props: {
				data: [
					{ label: 'Local disk', value: 10 },
					{ label: 'S3 bucket', value: 30 }
				],
				unit: 'count',
				label: 'Storage by place'
			}
		});
		expect(screen.getByRole('img', { name: /Storage by place/ })).toBeTruthy();
	});

	it('shows an empty state instead of a blank plot when there is no data', () => {
		render(BarChart, { props: { data: [], unit: 'count', label: 'Pieces per server' } });
		expect(screen.queryByRole('img')).toBeNull();
		expect(screen.getByText(t('chart.empty.title'))).toBeTruthy();
	});

	it('sorts bars by value descending by default', () => {
		const { container } = render(BarChart, {
			props: {
				data: [
					{ label: 'Local disk', value: 10 },
					{ label: 'S3 bucket', value: 30 },
					{ label: 'Spare', value: 20 }
				],
				unit: 'count',
				label: 'Storage by place'
			}
		});
		expect(barLabels(container)).toEqual(['S3 bucket', 'Spare', 'Local disk']);
	});

	it('keeps input order when sorted is false', () => {
		const { container } = render(BarChart, {
			props: {
				data: [
					{ label: 'Local disk', value: 10 },
					{ label: 'S3 bucket', value: 30 }
				],
				unit: 'count',
				label: 'Storage by place',
				sorted: false
			}
		});
		expect(barLabels(container)).toEqual(['Local disk', 'S3 bucket']);
	});

	it('draws one vertical bar per datum when orientation is vertical', () => {
		const { container } = render(BarChart, {
			props: {
				data: [
					{ label: 'Mon', value: 4 },
					{ label: 'Tue', value: 8 }
				],
				unit: 'count',
				label: 'Requests',
				orientation: 'vertical'
			}
		});
		expect(container.querySelectorAll('rect.bar')).toHaveLength(2);
	});

	it('draws one segment per stacked series for each datum', () => {
		const { container } = render(BarChart, {
			props: {
				data: [
					{ label: 'Route A', value: 0 },
					{ label: 'Route B', value: 0 }
				],
				unit: 'count',
				label: 'Requests by status',
				stacked: [
					{ id: '2xx', label: '2xx', unit: 'count', points: [[0, 5], [0, 7]] },
					{ id: '5xx', label: '5xx', unit: 'count', points: [[0, 1], [0, 2]] }
				]
			}
		});
		expect(container.querySelectorAll('rect.bar')).toHaveLength(4);
	});
});

describe('Histogram', () => {
	it('draws one bar per bucket and names the image', () => {
		const { container } = render(Histogram, {
			props: {
				buckets: [
					{ le: 1024, count: 3 },
					{ le: 2048, count: 5 }
				],
				unit: 'bytes',
				label: 'File sizes'
			}
		});
		expect(screen.getByRole('img', { name: /File sizes/ })).toBeTruthy();
		expect(barLabels(container)).toHaveLength(2);
	});
});

describe('Heatmap', () => {
	it('draws one tile per cell with a value tooltip', () => {
		const { container } = render(Heatmap, {
			props: {
				cells: [
					{ x: 0, y: 0, value: 1 },
					{ x: 1, y: 0, value: 4 },
					{ x: 0, y: 1, value: 2 },
					{ x: 1, y: 1, value: 8 }
				],
				xUnit: 'count',
				yUnit: 'count',
				label: 'Latency by hour',
				scale: 'seq'
			}
		});
		expect(screen.getByRole('img', { name: /Latency by hour/ })).toBeTruthy();
		const tiles = container.querySelectorAll('rect.cell');
		expect(tiles).toHaveLength(4);
		expect(tiles[0].querySelector('title')?.textContent).toContain(':');
	});

	it('uses the highest sequential step for the largest value', () => {
		const { container } = render(Heatmap, {
			props: {
				cells: [
					{ x: 0, y: 0, value: 1 },
					{ x: 1, y: 0, value: 9 }
				],
				xUnit: 'count',
				yUnit: 'count',
				label: 'Latency by hour',
				scale: 'seq'
			}
		});
		const fills = Array.from(container.querySelectorAll<SVGRectElement>('rect.cell'), (tile) => tile.style.fill);
		expect(fills[1]).toBe('var(--seq-7)');
		expect(fills[0]).toBe('var(--seq-1)');
	});
});

describe('DonutChart', () => {
	it('draws one slice per part when there are five parts or fewer', () => {
		const { container } = render(DonutChart, {
			props: {
				parts: [
					{ label: 'Healthy', value: 11 },
					{ label: 'Less protected', value: 1 }
				],
				label: 'Durability'
			}
		});
		expect(screen.getByRole('img', { name: /Durability/ })).toBeTruthy();
		expect(container.querySelectorAll('[data-slice]')).toHaveLength(2);
	});

	it('renders a bar chart instead when there are more than five parts', () => {
		const parts = [1, 2, 3, 4, 5, 6].map((value) => ({ label: `Part ${value}`, value }));
		const { container } = render(DonutChart, {
			props: { parts, label: 'Storage by place' }
		});
		expect(container.querySelectorAll('[data-slice]')).toHaveLength(0);
		expect(barLabels(container)).toHaveLength(6);
	});
});
