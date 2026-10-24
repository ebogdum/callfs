<script lang="ts" module>
	export const TOKENS = ['--middle-truncate-fg'] as const;

	const ELLIPSIS = '…';
	const PROTECTED_MAX_LENGTH = 24;
	const NUMBER_OR_ID_PATTERN = /^[0-9A-Za-z_.:,+\-−]+$/;
	const DIGIT_PATTERN = /\d/;

	const isProtected = (value: string): boolean =>
		PROTECTED_MAX_LENGTH > value.length && NUMBER_OR_ID_PATTERN.test(value) && DIGIT_PATTERN.test(value);

	const tailLength = (value: string, keep: number, minTail: number): number =>
		Math.min(value.length, Math.max(minTail, Math.floor(keep / 2)));

	const candidate = (value: string, keep: number, minTail: number): string => {
		const tail = tailLength(value, keep, minTail);
		return `${value.slice(0, Math.max(0, keep - tail))}${ELLIPSIS}${value.slice(value.length - tail)}`;
	};

	const fitText = (
		value: string,
		available: number,
		measure: (candidateText: string) => number,
		minTail: number
	): string => {
		if (isProtected(value) || measure(value) <= available) {
			return value;
		}
		let low = 0;
		let high = value.length - 1;
		let best = ELLIPSIS;
		while (low <= high) {
			const middle = Math.floor((low + high) / 2);
			const attempt = candidate(value, middle, minTail);
			if (measure(attempt) <= available) {
				best = attempt;
				low = middle + 1;
			} else {
				high = middle - 1;
			}
		}
		return best;
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'children'> {
		text: string;
		mono?: boolean;
		minTail?: number;
	}

	let { text, mono = false, minTail = 8, class: className, ...rest }: Props = $props();

	let root: HTMLSpanElement | undefined = $state();
	let sizer: HTMLSpanElement | undefined = $state();
	let shown = $state('');

	$effect(() => {
		const element = root;
		const measurer = sizer;
		const source = text;
		const tail = minTail;
		if (undefined === element || undefined === measurer) {
			return;
		}
		const measure = (candidateText: string): number => {
			measurer.textContent = candidateText;
			return measurer.scrollWidth;
		};
		const update = (): void => {
			shown = fitText(source, element.clientWidth, measure, tail);
		};
		const observer = new ResizeObserver(update);
		observer.observe(element);
		update();
		return () => observer.disconnect();
	});
</script>

<span
	class={['middle-truncate', className]}
	data-mono={mono ? '' : undefined}
	title={text}
	aria-label={text}
	bind:this={root}
	{...rest}
>
	{shown}
	<span class="sizer" aria-hidden="true" bind:this={sizer}></span>
</span>

<style>
	.middle-truncate {
		display: block;
		min-inline-size: 0;
		max-inline-size: 100%;
		overflow: hidden;
		white-space: nowrap;
		color: var(--middle-truncate-fg, inherit);
	}

	.middle-truncate[data-mono] {
		font-family: var(--font-mono);
	}

	.sizer {
		display: block;
		block-size: 0;
		overflow: hidden;
		visibility: hidden;
		white-space: pre;
	}
</style>
