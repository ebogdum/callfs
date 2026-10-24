<script lang="ts" module>
	export type ResizeOrientation = 'horizontal' | 'vertical';

	const KEY_STEP = 16;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		orientation: ResizeOrientation;
		value: number;
		min: number;
		max: number;
		label: string;
		class?: string;
	}

	let { orientation, value = $bindable(), min, max, label, class: className, ...rest }: Props = $props();

	let dragging = $state(false);
	let origin: { pointer: number; value: number } | null = null;

	function clamp(next: number): number {
		return Math.min(max, Math.max(min, next));
	}

	function pointerAt(event: MouseEvent): number {
		return 'vertical' === orientation ? event.clientX : event.clientY;
	}

	function moveDrag(event: MouseEvent): void {
		if (null === origin) {
			return;
		}
		value = clamp(origin.value + pointerAt(event) - origin.pointer);
	}

	function endDrag(): void {
		origin = null;
		dragging = false;
		window.removeEventListener('pointermove', moveDrag);
		window.removeEventListener('pointerup', endDrag);
		window.removeEventListener('pointercancel', endDrag);
	}

	function beginDrag(event: PointerEvent): void {
		event.preventDefault();
		origin = { pointer: pointerAt(event), value };
		dragging = true;
		window.addEventListener('pointermove', moveDrag);
		window.addEventListener('pointerup', endDrag);
		window.addEventListener('pointercancel', endDrag);
	}

	function nextValueFor(key: string): number | null {
		if ('Home' === key) {
			return min;
		}
		if ('End' === key) {
			return max;
		}
		if ('ArrowRight' === key || 'ArrowDown' === key) {
			return value + KEY_STEP;
		}
		if ('ArrowLeft' === key || 'ArrowUp' === key) {
			return value - KEY_STEP;
		}
		return null;
	}

	function handleKey(event: KeyboardEvent): void {
		const next = nextValueFor(event.key);
		if (null === next) {
			return;
		}
		event.preventDefault();
		value = clamp(next);
	}

	$effect(() => endDrag);
</script>

<div
	class={['resize-handle', orientation, className]}
	role="separator"
	tabindex="0"
	aria-label={label}
	aria-orientation={orientation}
	aria-valuemin={min}
	aria-valuemax={max}
	aria-valuenow={value}
	data-state={dragging ? 'dragging' : 'idle'}
	onpointerdown={beginDrag}
	onkeydown={handleKey}
	{...rest}
>
	<span class="grip"></span>
</div>

<style>
	.resize-handle {
		display: flex;
		flex: 0 0 auto;
		align-items: center;
		justify-content: center;
		background: transparent;
		touch-action: none;
		user-select: none;
	}

	.resize-handle.vertical {
		inline-size: var(--resize-handle-size, var(--target-min));
		block-size: 100%;
		cursor: col-resize;
	}

	.resize-handle.horizontal {
		block-size: var(--resize-handle-size, var(--target-min));
		inline-size: 100%;
		cursor: row-resize;
	}

	.grip {
		display: block;
		background: var(--resize-handle-color, var(--border-strong));
	}

	.resize-handle.vertical .grip {
		inline-size: var(--hairline);
		block-size: 100%;
	}

	.resize-handle.horizontal .grip {
		block-size: var(--hairline);
		inline-size: 100%;
	}

	.resize-handle[data-state='dragging'] .grip {
		background: var(--resize-handle-active-color, var(--accent));
	}

	.resize-handle:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.resize-handle:hover .grip {
			background: var(--resize-handle-active-color, var(--accent));
		}
	}

	@media (pointer: coarse) {
		.resize-handle.vertical {
			inline-size: var(--touch-target);
		}

		.resize-handle.horizontal {
			block-size: var(--touch-target);
		}
	}
</style>
