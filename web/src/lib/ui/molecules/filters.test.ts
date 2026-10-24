import '@testing-library/jest-dom/vitest';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { formatAbsolute } from '$lib/format/time';
import type { Chip } from '../types';
import FilterChip from './FilterChip.svelte';
import ChipBar from './ChipBar.svelte';
import TimeRangePicker from './TimeRangePicker.svelte';
import DateTimeInput from './DateTimeInput.svelte';

const sizeChip: Chip = { facet: 'size', op: '>', value: '1GB', label: 'Size > 1 GB' };
const typeChip: Chip = { facet: 'type', op: ':', value: 'image', label: 'Type: image' };

afterEach(() => {
	cleanup();
});

describe('FilterChip', () => {
	it('shows the chip label and a remove button named after it', () => {
		render(FilterChip, { props: { chip: sizeChip, onremove: vi.fn() } });

		expect(screen.getByText('Size > 1 GB')).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Remove Size > 1 GB' })).toBeInTheDocument();
	});

	it('calls onremove with the chip when the remove button is clicked', async () => {
		const onremove = vi.fn();
		render(FilterChip, { props: { chip: sizeChip, onremove } });

		await fireEvent.click(screen.getByRole('button', { name: 'Remove Size > 1 GB' }));

		expect(onremove).toHaveBeenCalledWith(sizeChip);
	});

	it('removes the chip on Backspace', async () => {
		const onremove = vi.fn();
		render(FilterChip, { props: { chip: sizeChip, onremove } });

		await fireEvent.keyDown(screen.getByText('Size > 1 GB'), { key: 'Backspace' });

		expect(onremove).toHaveBeenCalledWith(sizeChip);
	});

	it('ignores keys other than Backspace', async () => {
		const onremove = vi.fn();
		render(FilterChip, { props: { chip: sizeChip, onremove } });

		await fireEvent.keyDown(screen.getByText('Size > 1 GB'), { key: 'a' });

		expect(onremove).not.toHaveBeenCalled();
	});

	it('renders no edit control when onedit is not given', () => {
		render(FilterChip, { props: { chip: sizeChip, onremove: vi.fn() } });

		expect(screen.getAllByRole('button')).toHaveLength(1);
	});

	it('renders an edit button named after the label that calls onedit with the chip', async () => {
		const onedit = vi.fn();
		render(FilterChip, { props: { chip: sizeChip, onremove: vi.fn(), onedit } });

		await fireEvent.click(screen.getByRole('button', { name: 'Edit Size > 1 GB' }));

		expect(onedit).toHaveBeenCalledWith(sizeChip);
	});
});

describe('ChipBar', () => {
	it('renders nothing when there are no chips', () => {
		render(ChipBar, {
			props: { chips: [], onremove: vi.fn(), onclear: vi.fn(), label: 'Active filters' }
		});

		expect(screen.queryByRole('group', { name: 'Active filters' })).toBeNull();
	});

	it('renders a named group with one remove button per chip', () => {
		render(ChipBar, {
			props: { chips: [typeChip, sizeChip], onremove: vi.fn(), onclear: vi.fn(), label: 'Active filters' }
		});

		expect(screen.getByRole('group', { name: 'Active filters' })).toBeInTheDocument();
		expect(screen.getAllByRole('button', { name: /^Remove / })).toHaveLength(2);
	});

	it('calls onremove with the chip whose remove button was clicked', async () => {
		const onremove = vi.fn();
		render(ChipBar, {
			props: { chips: [typeChip, sizeChip], onremove, onclear: vi.fn(), label: 'Active filters' }
		});

		await fireEvent.click(screen.getByRole('button', { name: 'Remove Size > 1 GB' }));

		expect(onremove).toHaveBeenCalledWith(sizeChip);
	});

	it('calls onclear when Clear all is clicked', async () => {
		const onclear = vi.fn();
		render(ChipBar, {
			props: { chips: [typeChip], onremove: vi.fn(), onclear, label: 'Active filters' }
		});

		await fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));

		expect(onclear).toHaveBeenCalledTimes(1);
	});
});

describe('TimeRangePicker', () => {
	const range = { from: 1_000_000, to: 2_000_000, live: false };

	it('renders the five default preset buttons with visible names', () => {
		render(TimeRangePicker, { props: { value: range } });

		expect(screen.getByRole('button', { name: 'Last 15 minutes' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Last hour' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Last 6 hours' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Last 24 hours' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Last 7 days' })).toBeInTheDocument();
	});

	it('renders only the presets it is given', () => {
		render(TimeRangePicker, { props: { value: range, presets: ['1h'] } });

		expect(screen.getAllByRole('button', { name: /^Last / })).toHaveLength(1);
		expect(screen.getByRole('button', { name: 'Last hour' })).toBeInTheDocument();
	});

	it('marks the clicked preset as pressed and the others as not pressed', async () => {
		render(TimeRangePicker, { props: { value: range } });

		await fireEvent.click(screen.getByRole('button', { name: 'Last hour' }));

		expect(screen.getByRole('button', { name: 'Last hour' })).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByRole('button', { name: 'Last 15 minutes' })).toHaveAttribute('aria-pressed', 'false');
	});

	it('shows the live switch on when value.live is true', () => {
		render(TimeRangePicker, { props: { value: { ...range, live: true } } });

		expect(screen.getByRole('switch', { name: /Live/ })).toBeChecked();
	});

	it('shows the live switch off when value.live is false', () => {
		render(TimeRangePicker, { props: { value: range } });

		expect(screen.getByRole('switch', { name: /Live/ })).not.toBeChecked();
	});

	it('toggles the compare checkbox', async () => {
		render(TimeRangePicker, { props: { value: range } });
		const compare = screen.getByRole('checkbox', { name: 'Compare with previous period' });

		expect(compare).not.toBeChecked();
		await fireEvent.click(compare);

		expect(compare).toBeChecked();
	});

	it('shows an error when the start is not before the end', () => {
		render(TimeRangePicker, { props: { value: { from: 5, to: 5, live: false } } });

		expect(screen.getByText('Start must be before end.')).toBeInTheDocument();
	});

	it('shows no order error for a valid range', () => {
		render(TimeRangePicker, { props: { value: range } });

		expect(screen.queryByText('Start must be before end.')).toBeNull();
	});
});

describe('DateTimeInput', () => {
	const at = new Date(2026, 9, 23, 14, 5).getTime();

	it('shows the value as YYYY-MM-DD HH:mm in a labelled text field', () => {
		render(DateTimeInput, { props: { value: at, label: 'From' } });

		expect(screen.getByRole('textbox', { name: 'From' })).toHaveValue('2026-10-23 14:05');
	});

	it('shows the local time zone line', () => {
		render(DateTimeInput, { props: { value: at, label: 'From' } });

		expect(screen.getByText(/Your time zone:/)).toBeInTheDocument();
	});

	it('marks the field invalid and explains the format when the text does not parse', async () => {
		render(DateTimeInput, { props: { value: at, label: 'From' } });
		const input = screen.getByRole('textbox', { name: 'From' });

		await fireEvent.input(input, { target: { value: 'tomorrow' } });
		await fireEvent.change(input);

		expect(input).toHaveAttribute('aria-invalid', 'true');
		expect(screen.getByText('Type a date like 2026-10-23 14:05.')).toBeInTheDocument();
	});

	it('opens a calendar dialog named after the label', async () => {
		render(DateTimeInput, { props: { value: at, label: 'From' } });

		await fireEvent.click(screen.getByRole('button', { name: 'Pick date' }));

		expect(await screen.findByRole('dialog', { name: 'Calendar for From' })).toBeInTheDocument();
	});

	it('moves the day in the field when a calendar day is picked', async () => {
		render(DateTimeInput, { props: { value: at, label: 'From' } });

		await fireEvent.click(screen.getByRole('button', { name: 'Pick date' }));
		await fireEvent.click(await screen.findByRole('button', { name: formatAbsolute(new Date(2026, 9, 24)) }));

		await waitFor(() =>
			expect(screen.getByRole('textbox', { name: 'From' })).toHaveValue('2026-10-24 14:05')
		);
	});
});
