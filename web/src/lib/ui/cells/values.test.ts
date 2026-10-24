import { cleanup, render } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import BytesCell from './BytesCell.svelte';
import DurationCell from './DurationCell.svelte';
import NumberCell from './NumberCell.svelte';
import TextCell from './TextCell.svelte';
import TimeCell from './TimeCell.svelte';

const NBSP = ' ';
const DASH = '—';
const HOUR_MS = 60 * 60 * 1000;

class ResizeObserverStub {
	observe(): void {}
	unobserve(): void {}
	disconnect(): void {}
}

beforeEach(() => {
	vi.stubGlobal('ResizeObserver', ResizeObserverStub);
});

afterEach(() => {
	cleanup();
	vi.unstubAllGlobals();
});

describe('TextCell', () => {
	it('renders the full text on title and aria-label of the middle-truncated value', () => {
		const { container } = render(TextCell, { props: { value: 'report.pdf' } });
		const element = container.querySelector('.text-cell .middle-truncate');
		expect(element?.getAttribute('title')).toBe('report.pdf');
		expect(element?.getAttribute('aria-label')).toBe('report.pdf');
	});

	it('applies the muted class only when muted is true', () => {
		const { container } = render(TextCell, { props: { value: 'notes.txt', muted: true } });
		expect(container.querySelector('.text-cell.muted')).not.toBeNull();
	});

	it('renders the mono flag on the inner value', () => {
		const { container } = render(TextCell, { props: { value: 'abc123', mono: true } });
		expect(container.querySelector('.middle-truncate')?.hasAttribute('data-mono')).toBe(true);
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(TextCell, {
			props: { value: 'a.txt', class: 'extra', 'data-testid': 'text' }
		});
		expect(container.querySelector('.text-cell.extra[data-testid="text"]')).not.toBeNull();
	});
});

describe('NumberCell', () => {
	it('formats the number through the Num atom with grouping', () => {
		const { container } = render(NumberCell, { props: { value: 1234567 } });
		expect(container.querySelector('.number-cell .num')?.textContent).toContain('1,234,567');
	});

	it('appends the unit after the number', () => {
		const { container } = render(NumberCell, { props: { value: 12, unit: 'files' } });
		expect(container.querySelector('.number-cell .num')?.textContent).toContain(`12${NBSP}files`);
	});

	it('renders a dash for null', () => {
		const { container } = render(NumberCell, { props: { value: null } });
		expect(container.querySelector('.number-cell .num')?.textContent).toContain(DASH);
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(NumberCell, {
			props: { value: 3, class: 'extra', 'data-testid': 'number' }
		});
		expect(container.querySelector('.number-cell.extra[data-testid="number"]')).not.toBeNull();
	});
});

describe('BytesCell', () => {
	it('renders SI bytes with a non-breaking space and exact bytes on the title', () => {
		const { container } = render(BytesCell, { props: { value: 1500 } });
		const element = container.querySelector('.bytes-cell .bytes');
		expect(element?.textContent).toContain(`1.5${NBSP}kB`);
		expect(element?.getAttribute('title')).toBe(`1,500${NBSP}B`);
	});

	it('renders a dash with the unknown label for null', () => {
		const { container } = render(BytesCell, { props: { value: null } });
		const element = container.querySelector('.bytes-cell .bytes');
		expect(element?.textContent).toContain(DASH);
		expect(element?.textContent).toContain(t('common.unknown'));
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(BytesCell, {
			props: { value: 0, class: 'extra', 'data-testid': 'bytes' }
		});
		expect(container.querySelector('.bytes-cell.extra[data-testid="bytes"]')).not.toBeNull();
	});
});

describe('TimeCell', () => {
	it('renders a relative time element carrying the ISO timestamp', () => {
		const value = new Date(Date.now() - 3 * HOUR_MS).toISOString();
		const { container } = render(TimeCell, { props: { value } });
		expect(container.querySelector('.time-cell time')?.getAttribute('datetime')).toBe(value);
	});

	it('renders a dash with the unknown label and no time element for null', () => {
		const { container } = render(TimeCell, { props: { value: null } });
		expect(container.querySelector('.time-cell time')).toBeNull();
		expect(container.querySelector('.time-cell')?.textContent).toContain(DASH);
		expect(container.querySelector('.time-cell')?.textContent).toContain(t('common.unknown'));
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(TimeCell, {
			props: { value: null, class: 'extra', 'data-testid': 'time' }
		});
		expect(container.querySelector('.time-cell.extra[data-testid="time"]')).not.toBeNull();
	});
});

describe('DurationCell', () => {
	it('renders the short form with the long form on the title', () => {
		const { container } = render(DurationCell, { props: { value: 30000 } });
		const element = container.querySelector('.duration-cell .duration');
		expect(element?.textContent).toContain('30');
		expect(element?.getAttribute('title')).toBe('30 seconds');
	});

	it('parses a Go duration string into the long form on the title', () => {
		const { container } = render(DurationCell, { props: { value: '2m5s' } });
		expect(container.querySelector('.duration-cell .duration')?.getAttribute('title')).toBe(
			'2 minutes 5 seconds'
		);
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(DurationCell, {
			props: { value: 1000, class: 'extra', 'data-testid': 'duration' }
		});
		expect(container.querySelector('.duration-cell.extra[data-testid="duration"]')).not.toBeNull();
	});
});
