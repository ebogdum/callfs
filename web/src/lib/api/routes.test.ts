import { describe, expect, it } from 'vitest';
import { API_ROUTES } from './routes';

const SCREEN_IDS = Array.from({ length: 40 }, (_, index) => `S${String(index + 1).padStart(2, '0')}`);

describe('API route parity', () => {
	it('gives every non-internal route a screen or a command', () => {
		const uncovered = API_ROUTES.filter((route) => true !== route.internal)
			.filter((route) => 0 === route.screens.length && 0 === route.commands.length)
			.map((route) => route.id);
		expect(uncovered).toEqual([]);
	});

	it('marks internal routes with no screens and a reason', () => {
		const internal = API_ROUTES.filter((route) => true === route.internal);
		expect(internal.length).toBeGreaterThan(0);
		expect(internal.filter((route) => 0 !== route.screens.length)).toEqual([]);
		expect(internal.filter((route) => undefined === route.reason || '' === route.reason)).toEqual([]);
	});

	it('references every screen id S01 to S40', () => {
		const referenced = new Set(API_ROUTES.flatMap((route) => route.screens));
		expect(SCREEN_IDS.filter((id) => !referenced.has(id))).toEqual([]);
	});
});
