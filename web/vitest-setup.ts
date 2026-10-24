import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach, vi } from 'vitest';

if ('undefined' === typeof globalThis.ResizeObserver) {
	globalThis.ResizeObserver = class {
		observe() {}
		unobserve() {}
		disconnect() {}
	} as unknown as typeof ResizeObserver;
}

if ('function' !== typeof window.matchMedia) {
	window.matchMedia = ((query: string) => ({
		matches: false,
		media: query,
		onchange: null,
		addEventListener() {},
		removeEventListener() {},
		addListener() {},
		removeListener() {},
		dispatchEvent: () => false
	})) as unknown as typeof window.matchMedia;
}

if ('function' !== typeof Element.prototype.animate) {
	Element.prototype.animate = function () {
		return { cancel() {}, finished: Promise.resolve(), onfinish: null } as unknown as Animation;
	};
}

afterEach(async () => {
	cleanup();
	const body = document.body;
	if (body && ('hidden' === body.style.overflow || body.style.getPropertyValue('--scrollbar-width'))) {
		await new Promise((resolve) => setTimeout(resolve, 40));
	}
});

vi.mock('$app/navigation', () => ({
	goto: async () => undefined,
	invalidate: async () => undefined,
	invalidateAll: async () => undefined,
	preloadData: async () => ({ type: 'loaded', status: 200, data: {} }),
	preloadCode: async () => undefined,
	beforeNavigate: () => undefined,
	afterNavigate: () => undefined,
	onNavigate: () => undefined,
	pushState: () => undefined,
	replaceState: () => undefined
}));

vi.mock('$app/state', () => ({
	page: {
		url: new URL('http://127.0.0.1:4173/ui/'),
		params: {},
		route: { id: null },
		status: 200,
		error: null,
		data: {},
		form: null,
		state: {}
	},
	navigating: { from: null, to: null, type: null, willUnload: null, complete: null },
	updated: { current: false, check: async () => false }
}));

vi.mock('$app/environment', () => ({
	browser: true,
	dev: true,
	building: false,
	version: 'test'
}));

vi.mock('$app/paths', () => ({
	base: '/ui',
	assets: '/ui',
	resolve: (path: string) => `/ui${path}`,
	asset: (file: string) => `/ui${file}`
}));
