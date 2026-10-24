import { replaceState } from '$app/navigation';
import { page } from '$app/state';
import { setContext } from 'svelte';

const TIME_RANGE_PRESETS =['15m', '1h', '6h', '24h', '7d'] as const;
export type TimeRangePreset = (typeof TIME_RANGE_PRESETS)[number];

export interface TimeRangeValue {
	from: number;
	to: number;
	preset?: TimeRangePreset;
	live: boolean;
}

const TIME_RANGE_DEFAULT_PRESET: TimeRangePreset = '24h';
const TIME_RANGE_CONTEXT ='callfs.timeRange';

const LIVE_REFRESH_MS = 5000;

const PRESET_MS: Readonly<Record<TimeRangePreset, number>> = {
	'15m': 15 * 60 * 1000,
	'1h': 60 * 60 * 1000,
	'6h': 6 * 60 * 60 * 1000,
	'24h': 24 * 60 * 60 * 1000,
	'7d': 7 * 24 * 60 * 60 * 1000
};

function isTimeRangePreset(raw: string): raw is TimeRangePreset {
	return (TIME_RANGE_PRESETS as readonly string[]).includes(raw);
}

function readPreset(raw: string | null): TimeRangePreset | undefined {
	return null !== raw && isTimeRangePreset(raw) ? raw : undefined;
}

function readTimestamp(raw: string | null): number | null {
	if (null === raw) {
		return null;
	}
	const value = Number(raw);
	return Number.isSafeInteger(value) && 0 < value ? value : null;
}

export function presetValue(preset: TimeRangePreset, now: number, live: boolean): TimeRangeValue {
	return { from: now - PRESET_MS[preset], to: now, preset, live };
}

export function parseTimeRange(params: URLSearchParams, now: number): TimeRangeValue {
	const live = 'true' === params.get('live');
	const preset = readPreset(params.get('preset'));
	if (undefined !== preset) {
		return presetValue(preset, now, live);
	}
	const from = readTimestamp(params.get('from'));
	const to = readTimestamp(params.get('to'));
	if (null !== from && null !== to && from < to) {
		return { from, to, live: false };
	}
	return presetValue(TIME_RANGE_DEFAULT_PRESET, now, live);
}

export function writeTimeRange(params: URLSearchParams, value: TimeRangeValue): void {
	params.delete('from');
	params.delete('to');
	params.delete('preset');
	params.delete('live');
	if (undefined === value.preset) {
		params.set('from', String(value.from));
		params.set('to', String(value.to));
	} else {
		params.set('preset', value.preset);
	}
	if (value.live) {
		params.set('live', 'true');
	}
}

export class TimeRangeStore {
	value = $state.raw<TimeRangeValue>(presetValue(TIME_RANGE_DEFAULT_PRESET, 0, false));

	constructor(initial: TimeRangeValue) {
		this.value = initial;
	}

	refresh(now: number): void {
		const { preset, live } = this.value;
		if (false === live || undefined === preset) {
			return;
		}
		this.value = presetValue(preset, now, live);
	}
}

export function provideTimeRange(): TimeRangeStore {
	const store = new TimeRangeStore(parseTimeRange(page.url.searchParams, Date.now()));
	setContext(TIME_RANGE_CONTEXT, store);

	$effect(() => {
		const params = new URLSearchParams(page.url.searchParams);
		writeTimeRange(params, store.value);
		if (params.toString() !== page.url.searchParams.toString()) {
			const url = new URL(page.url);
			url.search = params.toString();
			replaceState(url, page.state);
		}
	});

	const liveRange = $derived(store.value.live && undefined !== store.value.preset);

	$effect(() => {
		if (false === liveRange) {
			return;
		}
		const timer = setInterval(() => {
			if (false === document.hidden) {
				store.refresh(Date.now());
			}
		}, LIVE_REFRESH_MS);
		return () => {
			clearInterval(timer);
		};
	});

	return store;
}
