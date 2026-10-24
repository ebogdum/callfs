import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import Swatch from './Swatch.svelte';
import TokenSample from './TokenSample.svelte';

function stubComputedStyle(values: Record<string, string>, backgroundColor = ''): void {
	vi.spyOn(window, 'getComputedStyle').mockImplementation(
		() =>
			({
				backgroundColor,
				getPropertyValue: (name: string) => values[name] ?? ''
			}) as unknown as CSSStyleDeclaration
	);
}

afterEach(() => {
	vi.restoreAllMocks();
	Reflect.deleteProperty(Element.prototype, 'animate');
});

describe('Swatch', () => {
	it('renders the visible label beside a decorative chip', () => {
		stubComputedStyle({}, 'rgb(255, 0, 0)');
		const { container } = render(Swatch, { props: { token: '--accent', label: 'Accent' } });
		expect(screen.getByText('Accent')).toBeTruthy();
		expect(container.querySelector('.chip')?.getAttribute('aria-hidden')).toBe('true');
	});

	it('reaches the success state once the computed colour is read', () => {
		stubComputedStyle({}, 'rgb(255, 0, 0)');
		const { container } = render(Swatch, { props: { token: '--accent', label: 'Accent' } });
		expect(container.querySelector('.swatch')?.getAttribute('data-state')).toBe('success');
	});

	it('shows the computed colour as a hex value when showValue is set', () => {
		stubComputedStyle({}, 'rgb(255, 0, 0)');
		render(Swatch, { props: { token: '--accent', label: 'Accent', showValue: true } });
		expect(screen.getByText('#ff0000')).toBeTruthy();
	});

	it('hides the value by default', () => {
		stubComputedStyle({}, 'rgb(255, 0, 0)');
		render(Swatch, { props: { token: '--accent', label: 'Accent' } });
		expect(screen.queryByText('#ff0000')).toBeNull();
	});

	it('converts modern rgb syntax with alpha to hex', () => {
		stubComputedStyle({}, 'rgb(9 13 13 / 0.48)');
		render(Swatch, { props: { token: '--scrim', label: 'Scrim', showValue: true } });
		expect(screen.getByText('#090d0d')).toBeTruthy();
	});

	it('passes unparseable computed colours through unchanged', () => {
		stubComputedStyle({}, 'oklch(0.5 0.1 200)');
		render(Swatch, { props: { token: '--accent', label: 'Accent', showValue: true } });
		expect(screen.getByText('oklch(0.5 0.1 200)')).toBeTruthy();
	});

	it('merges class and forwards rest attributes to the root', () => {
		stubComputedStyle({}, 'rgb(0, 0, 0)');
		const { container } = render(Swatch, {
			props: { token: '--surface', label: 'Surface', class: 'extra', id: 'surface-swatch' }
		});
		const root = container.querySelector('.swatch.extra');
		expect(root?.id).toBe('surface-swatch');
	});
});

describe('TokenSample', () => {
	it('renders the label and the kind for a space specimen', () => {
		stubComputedStyle({});
		const { container } = render(TokenSample, {
			props: { kind: 'space', token: '--space-2', label: 'Space 2' }
		});
		expect(screen.getByText('Space 2')).toBeTruthy();
		expect(container.querySelector('.token-sample')?.getAttribute('data-kind')).toBe('space');
		expect(screen.queryByRole('button')).toBeNull();
	});

	it('renders the radius specimen with its label', () => {
		stubComputedStyle({});
		render(TokenSample, {
			props: { kind: 'radius', token: '--radius-md', label: 'Radius medium' }
		});
		expect(screen.getByText('Radius medium')).toBeTruthy();
	});

	it('renders the label once inside the specimen for a type specimen', () => {
		stubComputedStyle({});
		render(TokenSample, {
			props: { kind: 'type', token: '--text-body', label: 'Body text' }
		});
		expect(screen.getAllByText('Body text')).toHaveLength(1);
	});

	it('exposes a motion specimen as a button named by its label', () => {
		stubComputedStyle({});
		render(TokenSample, { props: { kind: 'motion', token: '--dur-small', label: 'Small motion' } });
		expect(screen.getByRole('button', { name: 'Small motion' })).toBeTruthy();
	});

	it('plays the motion specimen only on click with the token duration and easing', () => {
		stubComputedStyle({
			'--dur-small': '160ms',
			'--ease-in-out': 'cubic-bezier(0.65, 0, 0.35, 1)'
		});
		const animate = vi.fn((_keyframes: unknown, _options?: unknown) => ({}));
		Object.defineProperty(Element.prototype, 'animate', { value: animate, configurable: true });
		render(TokenSample, { props: { kind: 'motion', token: '--dur-small', label: 'Small motion' } });
		expect(animate).not.toHaveBeenCalled();
		fireEvent.click(screen.getByRole('button', { name: 'Small motion' }));
		expect(animate).toHaveBeenCalledTimes(1);
		expect(animate.mock.calls[0][1]).toEqual({
			duration: 160,
			easing: 'cubic-bezier(0.65, 0, 0.35, 1)'
		});
	});

	it('plays a zero-duration motion specimen when the token is 0ms', () => {
		stubComputedStyle({ '--dur-0': '0ms', '--ease-in-out': 'linear' });
		const animate = vi.fn((_keyframes: unknown, _options?: unknown) => ({}));
		Object.defineProperty(Element.prototype, 'animate', { value: animate, configurable: true });
		render(TokenSample, { props: { kind: 'motion', token: '--dur-0', label: 'Instant motion' } });
		fireEvent.click(screen.getByRole('button', { name: 'Instant motion' }));
		expect(animate.mock.calls[0][1]).toEqual({ duration: 0, easing: 'linear' });
	});

	it('merges class and forwards rest attributes to the root', () => {
		stubComputedStyle({});
		const { container } = render(TokenSample, {
			props: { kind: 'radius', token: '--radius-sm', label: 'Radius small', class: 'extra', id: 'sample' }
		});
		const root = container.querySelector('.token-sample.extra');
		expect(root?.id).toBe('sample');
	});
});
