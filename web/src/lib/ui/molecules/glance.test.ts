import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Legend from './Legend.svelte';
import Sparkline from './Sparkline.svelte';
import StatTile from './StatTile.svelte';
import ThresholdBar from './ThresholdBar.svelte';

describe('StatTile', () => {
	it('renders a named region with the label and the value', () => {
		render(StatTile, { props: { label: 'Requests', value: 3, unit: 'count' } });

		const region = screen.getByRole('region', { name: 'Requests' });
		expect(region).toHaveAttribute('data-state', 'success');
		expect(screen.getByText('3')).toBeInTheDocument();
	});

	it('shows one reason line when the value is null', () => {
		render(StatTile, { props: { label: 'Requests', value: null, unit: 'count', help: 'No requests finished.' } });

		expect(screen.getByText('No requests finished.')).toBeInTheDocument();
		expect(screen.getByRole('region', { name: 'Requests' })).toHaveAttribute('data-state', 'empty');
	});

	it('shows a placeholder only while the value is loading', () => {
		render(StatTile, { props: { label: 'Requests', value: null, unit: 'count', loading: true } });

		const region = screen.getByRole('region', { name: 'Requests' });
		expect(region).toHaveAttribute('data-state', 'loading');
		expect(region.querySelector('.skeleton')).not.toBeNull();
	});

	it('shows the base line with the total and the rate', () => {
		render(StatTile, { props: { label: 'Failed', value: 3, unit: 'count', base: { num: 3, den: 12408 } } });

		expect(screen.getByText('of 12,408')).toBeInTheDocument();
	});

	it('renders a View details link only when href is set', () => {
		const { unmount } = render(StatTile, { props: { label: 'Requests', value: 3, unit: 'count', href: '/insights' } });

		expect(screen.getByRole('link', { name: 'View details' })).toHaveAttribute('href', '/insights');
		unmount();

		render(StatTile, { props: { label: 'Requests', value: 3, unit: 'count' } });

		expect(screen.queryByRole('link', { name: 'View details' })).not.toBeInTheDocument();
	});

	it('renders help text linked to the region', () => {
		render(StatTile, { props: { label: 'Requests', value: 3, unit: 'count', help: 'Last hour only.' } });

		const region = screen.getByRole('region', { name: 'Requests' });
		const helpId = region.getAttribute('aria-describedby');
		expect(helpId).not.toBeNull();
		expect(document.getElementById(helpId ?? '')).toHaveTextContent('Last hour only.');
	});

	it('renders the delta and the sparkline when given', () => {
		const { container } = render(StatTile, {
			props: {
				label: 'Requests',
				value: 3,
				unit: 'count',
				delta: { value: 2, goodWhen: 'down', period: 'vs previous hour' },
				sparkline: [1, 2, 3],
			},
		});

		expect(container.querySelector('.delta')).not.toBeNull();
		expect(screen.getByRole('img', { name: 'Rising. Last 3. Low 1. High 3.' })).toBeInTheDocument();
	});
});

describe('Sparkline', () => {
	it('names the chart with trend, last, low and high', () => {
		render(Sparkline, { props: { values: [1, 2, 3, 4, 5] } });

		expect(screen.getByRole('img', { name: 'Rising. Last 5. Low 1. High 5.' })).toBeInTheDocument();
	});

	it('names falling and flat series', () => {
		const { unmount } = render(Sparkline, { props: { values: [5, 4, 1] } });

		expect(screen.getByRole('img', { name: 'Falling. Last 1. Low 1. High 5.' })).toBeInTheDocument();
		unmount();

		render(Sparkline, { props: { values: [2, null, 2] } });

		expect(screen.getByRole('img', { name: 'Flat. Last 2. Low 2. High 2.' })).toBeInTheDocument();
	});

	it('names an empty series with the no points copy', () => {
		render(Sparkline, { props: { values: [null, null] } });

		expect(screen.getByRole('img', { name: 'No points to draw.' })).toBeInTheDocument();
	});

	it('exposes tone and height on the root', () => {
		render(Sparkline, { props: { values: [1, 2], tone: 'warn', height: 'md' } });

		const chart = screen.getByRole('img', { name: 'Rising. Last 2. Low 1. High 2.' });
		expect(chart).toHaveAttribute('data-tone', 'warn');
		expect(chart).toHaveAttribute('data-height', 'md');
	});
});

describe('Legend', () => {
	const series = [
		{ id: 'reads', label: 'Reads', colorIndex: 0 },
		{ id: 'writes', label: 'Writes', colorIndex: 1, dashed: true, hidden: true },
	];

	it('names each series button and exposes its pressed state', () => {
		render(Legend, { props: { series } });

		expect(screen.getByRole('group', { name: 'Chart series' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Reads' })).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByRole('button', { name: 'Writes' })).toHaveAttribute('aria-pressed', 'false');
	});

	it('calls ontoggle with the series id when a button is pressed', async () => {
		const ontoggle = vi.fn();
		render(Legend, { props: { series, ontoggle } });

		await fireEvent.click(screen.getByRole('button', { name: 'Writes' }));

		expect(ontoggle).toHaveBeenCalledWith('writes');
	});

	it('marks dashed series on the swatch', () => {
		const { container } = render(Legend, { props: { series } });

		expect(container.querySelector('.sample[data-dashed="true"]')).not.toBeNull();
		expect(container.querySelector('.sample[data-dashed="false"]')).not.toBeNull();
	});
});

describe('ThresholdBar', () => {
	const thresholds = [
		{ value: 80, tone: 'warn' as const, label: 'Warning at 80' },
		{ value: 90, tone: 'danger' as const, label: 'Critical at 90' },
	];

	it('exposes the meter and the reading sentence', () => {
		render(ThresholdBar, { props: { value: 71, max: 100, thresholds, label: 'Disk', unit: 'count' } });

		expect(screen.getByRole('meter', { name: 'Disk' })).toHaveAttribute('aria-valuenow', '71');
		expect(screen.getByText('29 free of 100 (29%).')).toBeInTheDocument();
		expect(screen.getByText('71 used.')).toBeInTheDocument();
	});

	it('lists each threshold with its warning or critical wording', () => {
		render(ThresholdBar, { props: { value: 71, max: 100, thresholds, label: 'Disk', unit: 'count' } });

		expect(screen.getByText('Warning when less than 20 is free.')).toHaveAttribute('data-tone', 'warn');
		expect(screen.getByText('Critical when less than 10 is free.')).toHaveAttribute('data-tone', 'danger');
	});
});
