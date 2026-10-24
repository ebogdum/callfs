import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Cluster from './Cluster.svelte';
import Frame from './Frame.svelte';
import Grid from './Grid.svelte';
import Stack from './Stack.svelte';
import Surface from './Surface.svelte';

const content = createRawSnippet(() => ({
	render: () => '<span class="content">content</span>'
}));

const rootOf = (container: HTMLElement): HTMLElement => container.firstElementChild as HTMLElement;

describe('Stack', () => {
	it('renders a flex column div with the gap token and renders children', () => {
		const { container } = render(Stack, { props: { children: content } });
		const root = rootOf(container);
		expect(root.tagName).toBe('DIV');
		expect(root.classList.contains('stack')).toBe(true);
		expect(root.style.getPropertyValue('--stack-gap')).toBe('var(--space-4)');
		expect(container.querySelector('.content')?.textContent).toBe('content');
	});

	it('maps gap tokens to space tokens, including fractional ones', () => {
		const { container } = render(Stack, { props: { gap: '1-5', children: content } });
		expect(rootOf(container).style.getPropertyValue('--stack-gap')).toBe('var(--space-1-5)');
	});

	it('passes align, as, class and rest attributes to the root element', () => {
		const { container } = render(Stack, {
			props: {
				as: 'section',
				align: 'end',
				class: 'extra',
				'aria-label': 'Members',
				children: content
			}
		});
		const root = rootOf(container);
		expect(root.tagName).toBe('SECTION');
		expect(root.classList.contains('extra')).toBe(true);
		expect(root.getAttribute('aria-label')).toBe('Members');
		expect(root.style.getPropertyValue('--stack-align')).toBe('end');
	});
});

describe('Cluster', () => {
	it('renders a wrapping row with default gap 2 and centred alignment', () => {
		const { container } = render(Cluster, { props: { children: content } });
		const root = rootOf(container);
		expect(root.classList.contains('cluster')).toBe(true);
		expect(root.style.getPropertyValue('--cluster-gap')).toBe('var(--space-2)');
		expect(root.style.getPropertyValue('--cluster-align')).toBe('center');
		expect(root.style.getPropertyValue('--cluster-wrap')).toBe('wrap');
	});

	it('turns wrapping off and applies justify, gap and as', () => {
		const { container } = render(Cluster, {
			props: {
				as: 'ul',
				gap: '6',
				justify: 'space-between',
				wrap: false,
				children: content
			}
		});
		const root = rootOf(container);
		expect(root.tagName).toBe('UL');
		expect(root.style.getPropertyValue('--cluster-gap')).toBe('var(--space-6)');
		expect(root.style.getPropertyValue('--cluster-justify')).toBe('space-between');
		expect(root.style.getPropertyValue('--cluster-wrap')).toBe('nowrap');
	});

	it('forwards class and rest attributes', () => {
		const { container } = render(Cluster, {
			props: { class: 'toolbar', 'data-testid': 'cluster', children: content }
		});
		const root = rootOf(container);
		expect(root.classList.contains('toolbar')).toBe(true);
		expect(root.getAttribute('data-testid')).toBe('cluster');
	});
});

describe('Grid', () => {
	it('uses auto-fill minmax columns with the default 16rem minimum and gap 4', () => {
		const { container } = render(Grid, { props: { children: content } });
		const root = rootOf(container);
		expect(root.classList.contains('grid')).toBe(true);
		expect(root.style.getPropertyValue('--grid-columns')).toBe(
			'repeat(auto-fill, minmax(min(100%, 16rem), 1fr))'
		);
		expect(root.style.getPropertyValue('--grid-gap')).toBe('var(--space-4)');
	});

	it('uses the given minimum and gap tokens', () => {
		const { container } = render(Grid, {
			props: { min: '24', gap: '3', children: content }
		});
		const root = rootOf(container);
		expect(root.style.getPropertyValue('--grid-columns')).toBe(
			'repeat(auto-fill, minmax(min(100%, 24rem), 1fr))'
		);
		expect(root.style.getPropertyValue('--grid-gap')).toBe('var(--space-3)');
	});

	it('uses a fixed column count when cols is given', () => {
		const { container } = render(Grid, { props: { cols: 3, children: content } });
		expect(rootOf(container).style.getPropertyValue('--grid-columns')).toBe(
			'repeat(3, minmax(0, 1fr))'
		);
	});
});

describe('Surface', () => {
	it('defaults to raised level with border and lg radius', () => {
		const { container } = render(Surface, { props: { children: content } });
		const root = rootOf(container);
		expect(root.classList.contains('surface')).toBe(true);
		expect(root.classList.contains('raised')).toBe(true);
		expect(root.classList.contains('bordered')).toBe(true);
		expect(root.style.getPropertyValue('--surface-radius')).toBe('var(--radius-lg)');
		expect(root.style.getPropertyValue('--surface-pad')).toBe('');
	});

	it('renders each level as its own class', () => {
		const levels = ['base', 'sunken', 'overlay'] as const;
		for (const level of levels) {
			const { container, unmount } = render(Surface, { props: { level, children: content } });
			expect(rootOf(container).classList.contains(level)).toBe(true);
			unmount();
		}
	});

	it('drops the border class when border is false', () => {
		const { container } = render(Surface, { props: { border: false, children: content } });
		expect(rootOf(container).classList.contains('bordered')).toBe(false);
	});

	it('maps the card pad keyword to the card padding token', () => {
		const { container } = render(Surface, { props: { pad: 'card', children: content } });
		expect(rootOf(container).style.getPropertyValue('--surface-pad')).toBe('var(--card-pad)');
	});

	it('maps gap tokens to space padding and radius props to radius tokens', () => {
		const { container } = render(Surface, {
			props: { pad: '2', radius: 'sm', as: 'article', children: content }
		});
		const root = rootOf(container);
		expect(root.tagName).toBe('ARTICLE');
		expect(root.style.getPropertyValue('--surface-pad')).toBe('var(--space-2)');
		expect(root.style.getPropertyValue('--surface-radius')).toBe('var(--radius-sm)');
	});
});

describe('Frame', () => {
	it('sets the aspect ratio from the ratio prop', () => {
		const { container } = render(Frame, { props: { ratio: '4/3', children: content } });
		const root = rootOf(container);
		expect(root.classList.contains('frame')).toBe(true);
		expect(root.style.getPropertyValue('--frame-ratio')).toBe('4 / 3');
	});

	it.each([
		['1/1', '1 / 1'],
		['16/9', '16 / 9'],
		['3/2', '3 / 2']
	] as const)('formats ratio %s as %s', (ratio, expected) => {
		const { container } = render(Frame, { props: { ratio, children: content } });
		expect(rootOf(container).style.getPropertyValue('--frame-ratio')).toBe(expected);
	});

	it('forwards class and rest attributes', () => {
		const { container } = render(Frame, {
			props: { ratio: '16/9', class: 'thumb', 'aria-hidden': 'true', children: content }
		});
		const root = rootOf(container);
		expect(root.classList.contains('thumb')).toBe(true);
		expect(root.getAttribute('aria-hidden')).toBe('true');
	});
});
