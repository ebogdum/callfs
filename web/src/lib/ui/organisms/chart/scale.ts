export interface Scale {
	readonly domain: readonly [number, number];
	readonly range: readonly [number, number];
	apply(value: number): number;
	invert(position: number): number;
	ticks(count: number): number[];
}

export interface PlotBox {
	left: number;
	top: number;
	right: number;
	bottom: number;
}

export type Point = [number, number];
export type SeriesPoint = [number, number | null];

export const AXIS_LEFT = 56;
export const AXIS_BOTTOM = 24;
export const TICK_GAP = 8;
export const PLOT_TOP = 8;
export const DEFAULT_RIGHT_INSET = 16;

const MIN_SAMPLES = 3;

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const TIME_STEPS = [
	SECOND,
	5 * SECOND,
	15 * SECOND,
	30 * SECOND,
	MINUTE,
	5 * MINUTE,
	15 * MINUTE,
	30 * MINUTE,
	HOUR,
	3 * HOUR,
	6 * HOUR,
	12 * HOUR,
	DAY,
	2 * DAY,
	7 * DAY
];

const ticksWithin = (from: number, to: number, step: number): number[] => {
	const first = Math.ceil(from / step);
	const last = Math.floor(to / step);
	return Array.from({ length: Math.max(0, last - first + 1) }, (_, index) => (first + index) * step);
};

const niceStep = (span: number, count: number): number => {
	const raw = span / Math.max(1, count);
	const magnitude = 10 ** Math.floor(Math.log10(raw));
	const normalized = raw / magnitude;
	if (Math.sqrt(50) <= normalized) return 10 * magnitude;
	if (Math.sqrt(10) <= normalized) return 5 * magnitude;
	if (Math.SQRT2 <= normalized) return 2 * magnitude;
	return magnitude;
};

export const tickDigits = (step: number): number => Math.min(6, Math.max(0, Math.ceil(-Math.log10(step) - 1e-9)));

export const niceTicks = (min: number, max: number, count: number): number[] => {
	if (max <= min) return [min];
	return ticksWithin(min, max, niceStep(max - min, count));
};

export const niceExtent = (min: number, max: number, count: number): [number, number] => {
	const low = max === min ? min - 1 : min;
	const high = max === min ? max + 1 : max;
	const step = niceStep(high - low, count);
	return [Math.floor(low / step) * step, Math.ceil(high / step) * step];
};

export const timeTicks = (from: number, to: number, count: number): number[] => {
	const desired = (to - from) / Math.max(1, count);
	const step = TIME_STEPS.find((candidate) => desired <= candidate) ?? 7 * DAY;
	return ticksWithin(from, to, step);
};

export const linearScale = (domain: readonly [number, number], range: readonly [number, number]): Scale => {
	const low = domain[0] === domain[1] ? domain[0] - 1 : domain[0];
	const high = domain[0] === domain[1] ? domain[1] + 1 : domain[1];
	const [start, end] = range;
	const span = high - low;
	const rangeSpan = end - start;
	return {
		domain: [low, high],
		range,
		apply: (value) => start + ((value - low) / span) * rangeSpan,
		invert: (position) => (0 === rangeSpan ? low : low + ((position - start) / rangeSpan) * span),
		ticks: (count) => niceTicks(low, high, count)
	};
};

export const timeScale = (domain: readonly [number, number], range: readonly [number, number], msPerUnit = 1): Scale => {
	const base = linearScale(domain, range);
	return {
		...base,
		ticks: (count) =>
			timeTicks(base.domain[0] * msPerUnit, base.domain[1] * msPerUnit, count).map((tick) => tick / msPerUnit)
	};
};

export const plotBox = (width: number, height: number, rightInset: number): PlotBox => ({
	left: AXIS_LEFT,
	top: PLOT_TOP,
	right: Math.max(AXIS_LEFT, width - rightInset),
	bottom: Math.max(PLOT_TOP, height - AXIS_BOTTOM)
});

export const runsOf = (points: readonly SeriesPoint[]): Point[][] => {
	const runs: Point[][] = [[]];
	for (const [x, y] of points) {
		if (null === y) {
			runs.push([]);
		} else {
			runs[runs.length - 1].push([x, y]);
		}
	}
	return runs.filter((run) => 0 < run.length);
};

export const lttb = (points: readonly Point[], threshold: number): Point[] => {
	if (points.length <= threshold || threshold < MIN_SAMPLES) return [...points];
	const sampled: Point[] = [points[0]];
	const bucketSize = (points.length - 2) / (threshold - 2);
	let anchor = 0;
	for (let bucket = 0; bucket < threshold - 2; bucket += 1) {
		const averageStart = Math.floor((bucket + 1) * bucketSize) + 1;
		const averageEnd = Math.min(Math.floor((bucket + 2) * bucketSize) + 1, points.length);
		let averageX = 0;
		let averageY = 0;
		for (let index = averageStart; index < averageEnd; index += 1) {
			averageX += points[index][0];
			averageY += points[index][1];
		}
		const averageCount = averageEnd - averageStart;
		averageX /= averageCount;
		averageY /= averageCount;
		const rangeStart = Math.floor(bucket * bucketSize) + 1;
		const rangeEnd = Math.min(Math.floor((bucket + 1) * bucketSize) + 1, points.length);
		const [anchorX, anchorY] = points[anchor];
		let bestArea = -1;
		let best = rangeStart;
		for (let index = rangeStart; index < rangeEnd; index += 1) {
			const area = Math.abs((anchorX - averageX) * (points[index][1] - anchorY) - (anchorX - points[index][0]) * (averageY - anchorY));
			if (area > bestArea) {
				bestArea = area;
				best = index;
			}
		}
		sampled.push(points[best]);
		anchor = best;
	}
	sampled.push(points[points.length - 1]);
	return sampled;
};

export const downsampleSeries = (points: readonly SeriesPoint[], width: number): SeriesPoint[] => {
	const threshold = Math.max(MIN_SAMPLES, Math.floor(width));
	if (points.length <= threshold) return [...points];
	const runs = runsOf(points);
	if (0 === runs.length) return [...points];
	const total = runs.reduce((sum, run) => sum + run.length, 0);
	const output: SeriesPoint[] = [];
	runs.forEach((run, index) => {
		if (0 < index) output.push([run[0][0], null]);
		const budget = Math.max(MIN_SAMPLES, Math.round((threshold * run.length) / total));
		for (const [x, y] of lttb(run, budget)) output.push([x, y]);
	});
	return output;
};

export const nearestIndex = (values: readonly number[], target: number): number => {
	if (0 === values.length) return -1;
	let low = 0;
	let high = values.length - 1;
	while (low < high) {
		const middle = (low + high) >> 1;
		if (values[middle] < target) {
			low = middle + 1;
		} else {
			high = middle;
		}
	}
	if (0 < low && Math.abs(values[low - 1] - target) <= Math.abs(values[low] - target)) return low - 1;
	return low;
};
