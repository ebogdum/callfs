import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ConflictResolver, { type ConflictRow } from './ConflictResolver.svelte';
import TransferTray, { type TrayAggregate, type TransferItem, type TransferState } from './TransferTray.svelte';

const announce = vi.hoisted(() => vi.fn());

vi.mock('../quarks/announce', () => ({ announce }));

const transfer = (id: string, state: TransferState, overrides: Partial<TransferItem> = {}): TransferItem => ({
	id,
	name: `${id}.bin`,
	destination: '/team/',
	state,
	done: 0,
	total: 100,
	partsDone: null,
	partsTotal: null,
	bytesPerSecond: 0,
	eta: null,
	resumedAt: null,
	error: null,
	...overrides
});

const aggregate: TrayAggregate = { done: 50, total: 200, bytesPerSecond: 0, eta: null };

const conflict = (path: string, size: number): ConflictRow => ({
	path,
	existing: { size, mtime: '2026-01-01T00:00:00Z' },
	incoming: { size: size + 2, mtime: '2026-02-01T00:00:00Z' }
});

const conflicts: ConflictRow[] = [
	conflict('/team/a.pdf', 10),
	conflict('/team/b.pdf', 20),
	conflict('/team/c.pdf', 30)
];

describe('TransferTray', () => {
	beforeEach(() => {
		announce.mockClear();
	});

	it('names the tray toggle and shows the overall percent', () => {
		render(TransferTray, { props: { items: [transfer('a', 'uploading')], aggregate } });

		const toggle = screen.getByRole('button', { name: /Transfers/ });
		expect(toggle).toHaveAttribute('aria-expanded', 'true');
		expect(screen.getByText(/25%/)).toBeInTheDocument();
	});

	it('collapses and expands the body from the toggle', async () => {
		render(TransferTray, { props: { items: [transfer('a', 'uploading')], aggregate, collapsed: false } });

		const toggle = screen.getByRole('button', { name: /Transfers/ });
		await fireEvent.click(toggle);

		expect(toggle).toHaveAttribute('aria-expanded', 'false');
		expect(document.getElementById('transfer-tray-body')).toHaveAttribute('hidden');
	});

	it('lists only in-progress items on the Active tab', () => {
		render(TransferTray, {
			props: {
				items: [transfer('a', 'uploading'), transfer('b', 'queued'), transfer('c', 'done')],
				aggregate
			}
		});

		expect(screen.getByText('a.bin')).toBeInTheDocument();
		expect(screen.queryByText('b.bin')).toBeNull();
		expect(screen.queryByText('c.bin')).toBeNull();
	});

	it('switches to the Queued tab and lists waiting items', async () => {
		render(TransferTray, {
			props: {
				items: [transfer('a', 'uploading'), transfer('b', 'queued')],
				aggregate
			}
		});

		await fireEvent.click(screen.getByRole('tab', { name: /Queued/ }));

		expect(screen.getByText('b.bin')).toBeInTheDocument();
		expect(screen.queryByText('a.bin')).toBeNull();
	});

	it('calls onpause with the item id', async () => {
		const onpause = vi.fn();
		render(TransferTray, { props: { items: [transfer('a', 'uploading')], aggregate, onpause } });

		await fireEvent.click(screen.getByRole('button', { name: 'Pause a.bin' }));

		expect(onpause).toHaveBeenCalledWith('a');
	});

	it('shows the stalled reason and retries the item', async () => {
		const onretry = vi.fn();
		render(TransferTray, { props: { items: [transfer('s', 'stalled')], aggregate, onretry } });

		expect(screen.getByText('No data for 15 seconds. Check the connection, then retry.')).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: 'Retry s.bin' }));

		expect(onretry).toHaveBeenCalledWith('s');
	});

	it('retries every failed item from the Failed tab footer', async () => {
		const onretryfailed = vi.fn();
		render(TransferTray, {
			props: { items: [transfer('f', 'failed')], aggregate, onretryfailed }
		});

		await fireEvent.click(screen.getByRole('button', { name: 'Retry failed' }));

		expect(onretryfailed).toHaveBeenCalledTimes(1);
	});

	it('clears finished items through onclear', async () => {
		const onclear = vi.fn();
		render(TransferTray, { props: { items: [transfer('d', 'done')], aggregate, onclear } });

		await fireEvent.click(screen.getByRole('button', { name: 'Clear finished' }));

		expect(onclear).toHaveBeenCalledTimes(1);
	});

	it('shows the empty copy when there are no transfers', () => {
		render(TransferTray, { props: { items: [], aggregate } });

		expect(screen.getByText('No transfers yet')).toBeInTheDocument();
		expect(screen.getByText('Uploads and downloads show here while they run.')).toBeInTheDocument();
	});

	it('announces a finished transfer politely', async () => {
		const { rerender } = render(TransferTray, {
			props: { items: [transfer('a', 'uploading')], aggregate }
		});

		await rerender({ items: [transfer('a', 'done')], aggregate });

		expect(announce).toHaveBeenCalledWith('1 transfer finished.', 'polite');
	});
});

describe('ConflictResolver', () => {
	it('defaults to Skip, marked as recommended', () => {
		render(ConflictResolver, { props: { conflicts, policy: 'skip', overrides: {} } });

		expect(screen.getByRole('radio', { name: /Skip/ })).toBeChecked();
		expect(screen.getByText('Recommended')).toBeInTheDocument();
		expect(screen.getByText('3 skip')).toBeInTheDocument();
	});

	it('lists every conflicting path in the table', () => {
		render(ConflictResolver, { props: { conflicts, policy: 'skip', overrides: {} } });

		expect(screen.getByRole('rowheader', { name: '/team/a.pdf' })).toBeInTheDocument();
		expect(screen.getByRole('rowheader', { name: '/team/b.pdf' })).toBeInTheDocument();
		expect(screen.getByRole('rowheader', { name: '/team/c.pdf' })).toBeInTheDocument();
	});

	it('summarises the effective action of each row after a per-row override', async () => {
		render(ConflictResolver, { props: { conflicts, policy: 'skip', overrides: {} } });

		await fireEvent.change(screen.getByRole('combobox', { name: 'Action for /team/a.pdf' }), {
			target: { value: 'replace' }
		});

		expect(screen.getByText('1 replace')).toBeInTheDocument();
		expect(screen.getByText('2 skip')).toBeInTheDocument();
	});

	it('applies a batch policy change to rows without an override', async () => {
		render(ConflictResolver, { props: { conflicts, policy: 'skip', overrides: {} } });

		await fireEvent.click(screen.getByRole('radio', { name: /Keep both/ }));

		expect(screen.getByRole('radio', { name: /Keep both/ })).toBeChecked();
		expect(screen.getByText('3 keep both')).toBeInTheDocument();
	});
});
