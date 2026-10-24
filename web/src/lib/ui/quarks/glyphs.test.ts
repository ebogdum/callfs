import { cleanup, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { createRawSnippet, tick } from 'svelte';
import type { IconName } from '../types';
import Dot from './Dot.svelte';
import Icon from './Icon.svelte';
import { icons } from './icons';
import Scroll from './Scroll.svelte';
import StatusGlyph from './StatusGlyph.svelte';

const iconNames: IconName[] = [
	'file',
	'file-text',
	'file-code',
	'file-json',
	'file-image',
	'file-video',
	'file-audio',
	'file-archive',
	'file-pdf',
	'folder',
	'folder-open',
	'folder-plus',
	'home',
	'search',
	'upload',
	'download',
	'link',
	'link-off',
	'share',
	'trash',
	'restore',
	'rename',
	'move',
	'copy',
	'clipboard',
	'check',
	'x',
	'plus',
	'minus',
	'chevron-right',
	'chevron-left',
	'chevron-up',
	'chevron-down',
	'chevrons-up-down',
	'more',
	'more-vertical',
	'filter',
	'sort-asc',
	'sort-desc',
	'columns',
	'density',
	'grid',
	'list',
	'eye',
	'eye-off',
	'lock',
	'unlock',
	'key',
	'shield',
	'shield-alert',
	'server',
	'database',
	'hard-drive',
	'cloud',
	'network',
	'cpu',
	'activity',
	'bar-chart',
	'line-chart',
	'gauge',
	'bell',
	'settings',
	'sliders',
	'sun',
	'moon',
	'monitor',
	'contrast',
	'palette',
	'keyboard',
	'help',
	'info',
	'warn',
	'danger',
	'ok',
	'fail',
	'circle',
	'circle-dot',
	'clock',
	'history',
	'refresh',
	'play',
	'pause',
	'stop',
	'external',
	'logout',
	'user',
	'users',
	'admin',
	'layers',
	'pieces',
	'split',
	'backup',
	'cache',
	'limit',
	'hourglass',
	'logs',
	'jobs',
	'terminal',
	'code',
	'braces',
	'diff',
	'fix',
	'mute',
	'pin',
	'unpin',
	'star',
	'maximize',
	'minimize',
	'inspector',
	'sidebar',
	'command',
	'enter',
	'arrow-up',
	'arrow-down',
	'arrow-right',
	'arrow-left',
	'wifi',
	'wifi-off',
	'plug',
	'power',
	'glossary',
	'leader',
	'lock-open',
	'hash',
	'tag',
	'filter-x',
	'calendar',
	'globe',
	'route',
	'qr',
	'save',
	'undo',
	'redo',
	'expand',
	'crosshair',
	'heat',
	'table'
];

const bodyContent = createRawSnippet(() => ({ render: () => '<p>Body</p>' }));

const stubOverflow = () => {
	Object.defineProperty(HTMLElement.prototype, 'scrollHeight', { configurable: true, get: () => 300 });
	Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, get: () => 100 });
};

afterEach(() => {
	cleanup();
	Reflect.deleteProperty(HTMLElement.prototype, 'scrollHeight');
	Reflect.deleteProperty(HTMLElement.prototype, 'clientHeight');
});

describe('icons', () => {
	it('maps every icon name to a Lucide component', () => {
		expect(Object.keys(icons).sort()).toEqual([...iconNames].sort());
		for (const name of iconNames) {
			expect(icons[name]).toBeTypeOf('function');
		}
	});
});

describe('Icon', () => {
	it('hides a decorative icon from assistive technology', () => {
		const { container } = render(Icon, { props: { name: 'file' } });
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('aria-hidden')).toBe('true');
		expect(svg?.hasAttribute('role')).toBe(false);
	});

	it('exposes a labelled icon as an image', () => {
		const { container } = render(Icon, { props: { name: 'warn', label: 'Warning' } });
		const svg = container.querySelector('svg');
		expect(svg?.getAttribute('role')).toBe('img');
		expect(svg?.getAttribute('aria-label')).toBe('Warning');
		expect(svg?.hasAttribute('aria-hidden')).toBe(false);
	});

	it('sizes the glyph from the size scale', () => {
		const { container } = render(Icon, { props: { name: 'ok', size: 'lg' } });
		expect(container.querySelector('svg')?.getAttribute('width')).toBe('20');
	});

	it('uses a 1.5 stroke width by default', () => {
		const { container } = render(Icon, { props: { name: 'ok' } });
		expect(container.querySelector('svg')?.getAttribute('stroke-width')).toBe('1.5');
	});
});

describe('StatusGlyph', () => {
	it('renders the tone shape hidden from assistive technology', () => {
		const { container } = render(StatusGlyph, { props: { tone: 'danger' } });
		const glyph = container.querySelector('[data-tone="danger"]');
		expect(glyph?.getAttribute('aria-hidden')).toBe('true');
		expect(glyph?.querySelector('svg')).not.toBeNull();
	});

	it('renders progress as a ring without an icon', () => {
		const { container } = render(StatusGlyph, { props: { tone: 'progress' } });
		const glyph = container.querySelector('[data-tone="progress"]');
		expect(glyph?.querySelector('.ring')).not.toBeNull();
		expect(glyph?.querySelector('svg')).toBeNull();
	});
});

describe('Dot', () => {
	it('renders a decorative pulsing dot', () => {
		const { container } = render(Dot, { props: { tone: 'warn', pulse: true } });
		const dot = container.querySelector('[data-tone="warn"]');
		expect(dot?.getAttribute('aria-hidden')).toBe('true');
		expect(dot?.getAttribute('data-pulse')).toBe('true');
	});

	it('renders a steady dot when pulse is off', () => {
		const { container } = render(Dot, { props: { tone: 'ok' } });
		expect(container.querySelector('[data-tone="ok"]')?.getAttribute('data-pulse')).toBe('false');
	});
});

describe('Scroll', () => {
	it('stays out of the tab order when content fits', async () => {
		const { container } = render(Scroll, { props: { label: 'Results', children: bodyContent } });
		await tick();
		const region = container.querySelector('[data-axis="y"]');
		expect(region?.hasAttribute('tabindex')).toBe(false);
		expect(region?.hasAttribute('role')).toBe(false);
	});

	it('becomes a labelled keyboard region when content overflows', async () => {
		stubOverflow();
		const { container } = render(Scroll, { props: { label: 'Results', children: bodyContent } });
		await tick();
		const region = container.querySelector('[data-axis="y"]');
		expect(region?.getAttribute('tabindex')).toBe('0');
		expect(region?.getAttribute('role')).toBe('region');
		expect(region?.getAttribute('aria-label')).toBe('Results');
	});
});
