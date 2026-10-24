<script lang="ts">
	import { Slider } from 'bits-ui';

	interface Props {
		value: number;
		min: number;
		max: number;
		step: number;
		label: string;
		format?: (n: number) => string;
		class?: string;
		id?: string;
	}

	let {
		value = $bindable(),
		min,
		max,
		step,
		label,
		format = (n: number) => String(n),
		class: className,
		id: rootId
	}: Props = $props();

	const id = $props.id();
	const labelId = `${id}-label`;
	const shownValue = $derived(format(value));
</script>

<div class={['slider', className]}>
	<div class="head">
		<span class="label" id={labelId}>{label}</span>
		<span class="value">{shownValue}</span>
	</div>
	<Slider.Root type="single" bind:value {min} {max} {step} class="track" id={rootId}>
		{#snippet children()}
			<Slider.Range class="range" />
			<Slider.Thumb index={0} class="thumb" aria-labelledby={labelId} aria-valuetext={shownValue} />
		{/snippet}
	</Slider.Root>
</div>

<style>
	.slider {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-2);
	}

	.label {
		color: var(--text);
		font-size: var(--text-body-size);
	}

	.value {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		font-variant-numeric: tabular-nums;
	}

	.slider :global(.track) {
		position: relative;
		display: flex;
		align-items: center;
		inline-size: 100%;
		block-size: var(--target-min);
		background: linear-gradient(var(--slider-track, var(--border-control)), var(--slider-track, var(--border-control)))
			center / 100% var(--space-1-5) no-repeat;
		cursor: pointer;
		touch-action: none;
	}

	.slider :global(.track[data-disabled]) {
		cursor: not-allowed;
		background-image: linear-gradient(var(--text-faint), var(--text-faint));
	}

	.slider :global(.range) {
		block-size: var(--space-1-5);
		border-radius: var(--radius-full);
		background: var(--slider-range, var(--accent));
	}

	.slider :global(.thumb) {
		position: absolute;
		inset-block-start: 50%;
		translate: 0 -50%;
		inline-size: var(--slider-thumb-size, var(--space-6));
		block-size: var(--slider-thumb-size, var(--space-6));
		border: var(--hairline) solid var(--slider-range, var(--accent));
		border-radius: var(--radius-full);
		background: var(--slider-thumb, var(--surface));
		transition: background-color var(--dur-micro) var(--ease-standard);
	}

	.slider :global(.thumb:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.slider :global(.track:not([data-disabled]) .thumb:hover) {
			background: var(--surface-hover);
		}
	}

	.slider :global(.track:not([data-disabled]) .thumb:active) {
		background: var(--surface-pressed);
	}
</style>
