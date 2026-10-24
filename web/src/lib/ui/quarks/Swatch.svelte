<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		token: string;
		label: string;
		showValue?: boolean;
	}

	let { token, label, showValue = false, class: className, ...rest }: Props = $props();

	let chip: HTMLSpanElement | undefined = $state();
	let value = $state('');
	let resolved = $state(false);

	function toHex(computed: string): string {
		const match = /^rgba?\(\s*(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)/.exec(computed);
		if (null === match) {
			return computed;
		}
		return `#${match
			.slice(1, 4)
			.map((channel) => Math.round(Number(channel)).toString(16).padStart(2, '0'))
			.join('')}`;
	}

	$effect(() => {
		const element = chip;
		const current = token;
		if (undefined === element || '' === current) {
			return;
		}
		value = toHex(getComputedStyle(element).backgroundColor);
		resolved = true;
	});
</script>

<span class={['swatch', className]} data-state={resolved ? 'success' : 'loading'} {...rest}>
	<span class="chip" bind:this={chip} style:background-color={`var(${token})`} aria-hidden="true"></span>
	<span class="text">
		<span class="label">{label}</span>
		{#if showValue}
			<span class="value">{value}</span>
		{/if}
	</span>
</span>

<style>
	.swatch {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text);
	}

	.chip {
		display: block;
		flex: none;
		inline-size: var(--space-6);
		block-size: var(--space-6);
		border-radius: var(--radius-sm);
		box-shadow: inset 0 0 0 var(--hairline) var(--border);
	}

	.text {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.label {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-sm-weight);
		letter-spacing: var(--text-sm-tracking);
	}

	.value {
		font-family: var(--font-mono);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		color: var(--text-muted);
		font-variant-numeric: tabular-nums;
		overflow-wrap: anywhere;
	}
</style>
