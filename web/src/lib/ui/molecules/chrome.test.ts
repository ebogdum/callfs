import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import StaleIndicator from './StaleIndicator.svelte';
import ConnectionPill from './ConnectionPill.svelte';
import ScopeChip from './ScopeChip.svelte';
import ThemeToggle from './ThemeToggle.svelte';

describe('StaleIndicator', () => {
	it('names the refresh button and calls onrefresh on click', async () => {
		const onrefresh = vi.fn();
		render(StaleIndicator, {
			props: { updatedAt: Date.now(), intervalMs: 5000, state: 'live', onrefresh }
		});
		const button = screen.getByRole('button', { name: 'Refresh' });
		await fireEvent.click(button);
		expect(onrefresh).toHaveBeenCalledTimes(1);
	});

	it('shows the update time and no stale word for a fresh live indicator', () => {
		render(StaleIndicator, {
			props: { updatedAt: Date.now(), intervalMs: 5000, state: 'live', onrefresh: vi.fn() }
		});
		expect(screen.getByText(/^Updated /)).toBeTruthy();
		expect(screen.queryByText('Stale')).toBeNull();
	});

	it('shows the Stale word when the state is stale', () => {
		render(StaleIndicator, {
			props: { updatedAt: Date.now(), intervalMs: 5000, state: 'stale', onrefresh: vi.fn() }
		});
		expect(screen.getByText('Stale')).toBeTruthy();
	});

	it('shows the Stale word when the age passes twice the interval', () => {
		render(StaleIndicator, {
			props: {
				updatedAt: Date.now() - 60_000,
				intervalMs: 5000,
				state: 'live',
				onrefresh: vi.fn()
			}
		});
		expect(screen.getByText('Stale')).toBeTruthy();
	});

	it.each([
		['paused', 'Paused'],
		['offline', 'Offline']
	] as const)('shows the %s word for the %s state', (state, word) => {
		render(StaleIndicator, {
			props: { updatedAt: Date.now(), intervalMs: 5000, state, onrefresh: vi.fn() }
		});
		expect(screen.getByText(word)).toBeTruthy();
	});

	it('says the data was not updated yet when updatedAt is null', () => {
		render(StaleIndicator, {
			props: { updatedAt: null, intervalMs: 5000, state: 'live', onrefresh: vi.fn() }
		});
		expect(screen.getByText('Not updated yet')).toBeTruthy();
	});
});

describe('ConnectionPill', () => {
	it.each([
		['live', 'Live'],
		['stale', 'Stale'],
		['reconnecting', 'Reconnecting'],
		['offline', 'Offline'],
		['paused', 'Paused']
	] as const)('shows the word for the %s state', (state, word) => {
		const { container } = render(ConnectionPill, { props: { state } });
		expect(screen.getByText(word)).toBeTruthy();
		expect(container.firstElementChild?.getAttribute('data-state')).toBe(state);
	});

	it('announces changes politely', () => {
		const { container } = render(ConnectionPill, { props: { state: 'live' } });
		expect(container.firstElementChild?.getAttribute('aria-live')).toBe('polite');
	});
});

describe('ScopeChip', () => {
	it('shows the server name', () => {
		render(ScopeChip, { props: { server: 'callfs-instance-1' } });
		expect(screen.getByText('Server callfs-instance-1')).toBeTruthy();
	});

	it('shows the member and leader when given', () => {
		render(ScopeChip, { props: { server: 'callfs-instance-1', member: 'node-2', leader: true } });
		expect(screen.getByText('Member node-2')).toBeTruthy();
		expect(screen.getByText('Leader')).toBeTruthy();
	});

	it('omits the member and leader when not given', () => {
		render(ScopeChip, { props: { server: 'callfs-instance-1' } });
		expect(screen.queryByText('Leader')).toBeNull();
		expect(screen.queryByText(/^Member /)).toBeNull();
	});
});

describe('ThemeToggle', () => {
	it('names the group and shows each option', () => {
		render(ThemeToggle, { props: { value: 'system' } });
		expect(screen.getByLabelText('Theme')).toBeTruthy();
		expect(screen.getByText('System')).toBeTruthy();
		expect(screen.getByText('Light')).toBeTruthy();
		expect(screen.getByText('Dark')).toBeTruthy();
	});

	it('marks the bound option as on', () => {
		render(ThemeToggle, { props: { value: 'dark' } });
		expect(screen.getByText('Dark').closest('button')?.getAttribute('data-state')).toBe('on');
		expect(screen.getByText('Light').closest('button')?.getAttribute('data-state')).toBe('off');
	});

	it('selects an option on click', async () => {
		render(ThemeToggle, { props: { value: 'system' } });
		await fireEvent.click(screen.getByText('Light'));
		expect(screen.getByText('Light').closest('button')?.getAttribute('data-state')).toBe('on');
		expect(screen.getByText('System').closest('button')?.getAttribute('data-state')).toBe('off');
	});

	it('keeps a mode selected when the selected option is clicked again', async () => {
		render(ThemeToggle, { props: { value: 'system' } });
		await fireEvent.click(screen.getByText('System'));
		expect(screen.getByText('System').closest('button')?.getAttribute('data-state')).toBe('on');
	});
});
