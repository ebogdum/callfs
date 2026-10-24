import { cleanup, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { t } from '$lib/copy/t';
import Bytes from './Bytes.svelte';
import Delta from './Delta.svelte';
import Duration from './Duration.svelte';
import Num from './Num.svelte';
import Percent from './Percent.svelte';
import RelTime from './RelTime.svelte';

const NBSP = ' ';
const HOUR_MS = 60 * 60 * 1000;

afterEach(() => {
	cleanup();
});

describe('Bytes', () => {
	it('renders SI units with a non-breaking space and exact bytes on the title', () => {
		const { container } = render(Bytes, { props: { value: 1500 } });
		const element = container.querySelector('span.bytes');
		expect(element?.textContent).toContain(`1.5${NBSP}kB`);
		expect(element?.getAttribute('title')).toBe(`1,500${NBSP}B`);
	});

	it('omits the title when exactOnHover is false', () => {
		const { container } = render(Bytes, { props: { value: 1500, exactOnHover: false } });
		expect(container.querySelector('span.bytes')?.hasAttribute('title')).toBe(false);
	});

	it('renders a dash with an unknown label for null', () => {
		const { container } = render(Bytes, { props: { value: null } });
		const element = container.querySelector('span.bytes');
		expect(element?.textContent).toContain('—');
		expect(element?.textContent).toContain(t('common.unknown'));
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(Bytes, { props: { value: 0, class: 'extra', 'data-testid': 'bytes' } });
		const element = container.querySelector('span.bytes.extra');
		expect(element?.getAttribute('data-testid')).toBe('bytes');
	});
});

describe('Num', () => {
	it('renders whole numbers with a true minus sign', () => {
		const { container } = render(Num, { props: { value: -5 } });
		expect(container.querySelector('span.num')?.textContent).toContain('−5');
	});

	it('uses compact notation and puts the full value on the title', () => {
		const { container } = render(Num, { props: { value: 12345, style: 'compact' } });
		const element = container.querySelector('span.num');
		expect(element?.textContent).toContain('12.3K');
		expect(element?.getAttribute('title')).toBe('12,345');
	});

	it('applies precision and unit with a non-breaking space', () => {
		const { container } = render(Num, { props: { value: 2.5, style: 'decimal', precision: 2, unit: 'ms' } });
		expect(container.querySelector('span.num')?.textContent).toContain(`2.5${NBSP}ms`);
	});

	it('renders a dash with an unknown label for null', () => {
		const { container } = render(Num, { props: { value: null } });
		const element = container.querySelector('span.num');
		expect(element?.textContent).toContain('—');
		expect(element?.textContent).toContain(t('common.unknown'));
	});
});

describe('Percent', () => {
	it('shows counts first and the rate second in parentheses', () => {
		const { container } = render(Percent, { props: { num: 312, den: 1204 } });
		const element = container.querySelector('span.percent');
		expect(element?.querySelector('.count')?.textContent).toBe(`312 ${t('number.of')} 1,204`);
		expect(element?.querySelector('.rate')?.textContent).toBe('(26%)');
	});

	it('shows only the rate for rate-only style', () => {
		const { container } = render(Percent, { props: { num: 1, den: 4, style: 'rate-only' } });
		const element = container.querySelector('span.percent');
		expect(element?.querySelector('.count')).toBeNull();
		expect(element?.querySelector('.rate')?.textContent).toBe('(25%)');
	});

	it('caps the rate at 100 percent', () => {
		const { container } = render(Percent, { props: { num: 9, den: 4 } });
		expect(container.querySelector('span.percent .rate')?.textContent).toBe('(100%)');
	});

	it('shows only the count when the denominator is zero', () => {
		const { container } = render(Percent, { props: { num: 0, den: 0 } });
		expect(container.querySelector('span.percent .rate')).toBeNull();
		expect(container.querySelector('span.percent .count')?.textContent).toBe('0');
	});
});

describe('RelTime', () => {
	it('renders a time element with an ISO datetime and a relative phrase', () => {
		const at = new Date(Date.now() - 3 * HOUR_MS);
		const { container } = render(RelTime, { props: { at } });
		const element = container.querySelector('time.reltime');
		expect(element?.getAttribute('datetime')).toBe(at.toISOString());
		expect(element?.textContent).toContain('hours ago');
		expect(element?.getAttribute('title')).toBeTruthy();
	});

	it('renders an absolute date in absolute mode', () => {
		const at = new Date(Date.now() - 3 * HOUR_MS);
		const { container } = render(RelTime, { props: { at, mode: 'absolute' } });
		expect(container.querySelector('time.reltime')?.textContent).not.toContain('ago');
	});

	it('renders a dash and no datetime for an invalid value', () => {
		const { container } = render(RelTime, { props: { at: 'not a date' } });
		const element = container.querySelector('time.reltime');
		expect(element?.textContent).toBe('—');
		expect(element?.hasAttribute('datetime')).toBe(false);
	});
});

describe('Duration', () => {
	it('renders the long form for milliseconds', () => {
		const { container } = render(Duration, { props: { value: 30000 } });
		expect(container.querySelector('span.duration')?.textContent).toContain('30 seconds');
	});

	it('parses a Go duration string into the long form', () => {
		const { container } = render(Duration, { props: { value: '2m5s' } });
		expect(container.querySelector('span.duration')?.textContent).toContain('2 minutes 5 seconds');
	});

	it('puts the long form on the title for the short style', () => {
		const { container } = render(Duration, { props: { value: 30000, style: 'short' } });
		const element = container.querySelector('span.duration');
		expect(element?.textContent).not.toContain('seconds');
		expect(element?.getAttribute('title')).toBe('30 seconds');
	});

	it('renders a dash with an unknown label for an invalid Go duration', () => {
		const { container } = render(Duration, { props: { value: 'soon' } });
		const element = container.querySelector('span.duration');
		expect(element?.textContent).toContain('—');
		expect(element?.textContent).toContain(t('common.unknown'));
	});
});

describe('Delta', () => {
	it('shows a plus sign, the value, a better verdict and the period when an up change is good', () => {
		const { container } = render(Delta, { props: { value: 12, unit: 'count', goodWhen: 'up', period: 'vs previous hour' } });
		const element = container.querySelector('span.delta');
		expect(element?.getAttribute('data-tone')).toBe('ok');
		expect(element?.getAttribute('data-direction')).toBe('up');
		expect(element?.querySelector('.change')?.textContent).toBe('+12');
		expect(element?.querySelector('.verdict')?.textContent).toBe(t('delta.better'));
		expect(element?.querySelector('.period')?.textContent).toBe('vs previous hour');
	});

	it('shows a true minus sign and a worse verdict when a down change is bad', () => {
		const { container } = render(Delta, { props: { value: -3, unit: 'count', goodWhen: 'up', period: 'vs yesterday' } });
		const element = container.querySelector('span.delta');
		expect(element?.getAttribute('data-tone')).toBe('danger');
		expect(element?.querySelector('.change')?.textContent).toBe('−3');
		expect(element?.querySelector('.verdict')?.textContent).toBe(t('delta.worse'));
	});

	it('formats byte changes with SI units', () => {
		const { container } = render(Delta, { props: { value: 1500, unit: 'bytes', goodWhen: 'down', period: 'vs last day' } });
		const element = container.querySelector('span.delta');
		expect(element?.querySelector('.change')?.textContent).toBe(`+1.5${NBSP}kB`);
		expect(element?.getAttribute('data-tone')).toBe('danger');
	});

	it('shows no verdict and a neutral tone when goodWhen is none', () => {
		const { container } = render(Delta, { props: { value: 4, unit: 'percent', goodWhen: 'none', period: 'vs previous hour' } });
		const element = container.querySelector('span.delta');
		expect(element?.getAttribute('data-tone')).toBe('neutral');
		expect(element?.querySelector('.change')?.textContent).toBe('+4%');
		expect(element?.querySelector('.verdict')).toBeNull();
	});

	it('shows a flat direction with no sign for zero change', () => {
		const { container } = render(Delta, { props: { value: 0, unit: 'count', goodWhen: 'up', period: 'vs previous hour' } });
		const element = container.querySelector('span.delta');
		expect(element?.getAttribute('data-direction')).toBe('flat');
		expect(element?.querySelector('.change')?.textContent).toBe('0');
		expect(element?.querySelector('.verdict')).toBeNull();
	});

	it('renders a dash with an unknown label when the value is not finite', () => {
		const { container } = render(Delta, { props: { value: Number.NaN, unit: 'count', goodWhen: 'up', period: 'vs previous hour' } });
		const element = container.querySelector('span.delta');
		expect(element?.querySelector('.change')?.textContent).toBe('—');
		expect(element?.textContent).toContain(t('common.unknown'));
	});
});
