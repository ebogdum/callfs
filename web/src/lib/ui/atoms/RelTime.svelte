<script lang="ts" module>
	export type RelTimeMode = 'auto' | 'relative' | 'absolute';

	const TICK_MS = 30 * 1000;
	const listeners = new Set<() => void>();
	let timer: ReturnType<typeof setInterval> | undefined;

	const subscribe = (listener: () => void): (() => void) => {
		listeners.add(listener);
		if (undefined === timer) {
			timer = setInterval(() => {
				for (const notify of listeners) {
					notify();
				}
			}, TICK_MS);
		}
		return () => {
			listeners.delete(listener);
			if (0 === listeners.size && undefined !== timer) {
				clearInterval(timer);
				timer = undefined;
			}
		};
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatAbsolute, formatRelative, formatTimestamp } from '$lib/format/time';

	interface Props extends HTMLAttributes<HTMLTimeElement> {
		at: string | number | Date;
		mode?: RelTimeMode;
	}

	let { at, mode = 'auto', class: className, ...rest }: Props = $props();

	let now = $state(Date.now());
	$effect(() => subscribe(() => {
		now = Date.now();
	}));

	const date = $derived(at instanceof Date ? at : new Date(at));
	const iso = $derived(Number.isNaN(date.getTime()) ? undefined : date.toISOString());
	const text = $derived('absolute' === mode ? formatAbsolute(date) : formatRelative(date, now));
</script>

<time class={['reltime', className]} datetime={iso} title={formatTimestamp(date)} {...rest}>{text}</time>

<style>
	.reltime {
		color: var(--reltime-color, inherit);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
