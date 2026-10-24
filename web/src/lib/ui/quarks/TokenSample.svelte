<script lang="ts" module>
	export type TokenSampleKind = 'space' | 'radius' | 'elevation' | 'motion' | 'type';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		kind: TokenSampleKind;
		token: string;
		label: string;
	}

	let { kind, token, label, class: className, ...rest }: Props = $props();

	let specimenElement: HTMLSpanElement | undefined = $state();

	function durationMilliseconds(value: string): number {
		const match = /^(\d+(?:\.\d+)?)(ms|s)$/.exec(value.trim());
		if (null === match) {
			return 0;
		}
		const amount = Number(match[1]);
		return 'ms' === match[2] ? amount : amount * 1000;
	}

	function play(): void {
		const element = specimenElement;
		if (undefined === element || 'function' !== typeof element.animate) {
			return;
		}
		const computed = getComputedStyle(element);
		element.animate(
			[{ transform: 'translateX(0)' }, { transform: 'translateX(100%)' }, { transform: 'translateX(0)' }],
			{
				duration: durationMilliseconds(computed.getPropertyValue(token)),
				easing: computed.getPropertyValue('--ease-in-out').trim() || 'linear'
			}
		);
	}
</script>

{#snippet specimen()}
	<span
		class={['specimen', kind]}
		bind:this={specimenElement}
		data-kind={kind}
		style:inline-size={'space' === kind ? `var(${token})` : undefined}
		style:border-radius={'radius' === kind ? `var(${token})` : undefined}
		style:box-shadow={'elevation' === kind ? `var(${token})` : undefined}
		style:font-size={'type' === kind ? `var(${token}-size)` : undefined}
		style:line-height={'type' === kind ? `var(${token}-line)` : undefined}
		style:font-weight={'type' === kind ? `var(${token}-weight)` : undefined}
		style:letter-spacing={'type' === kind ? `var(${token}-tracking)` : undefined}
	>
		{#if 'type' === kind}{label}{/if}
	</span>
{/snippet}

<div class={['token-sample', className]} data-kind={kind} {...rest}>
	{#if 'motion' === kind}
		<button type="button" class="sample" onclick={play}>
			{@render specimen()}
			<span class="label">{label}</span>
		</button>
	{:else if 'type' === kind}
		{@render specimen()}
	{:else}
		{@render specimen()}
		<span class="label">{label}</span>
	{/if}
</div>

<style>
	.token-sample {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-2);
		padding: var(--space-2);
		color: var(--text);
	}

	.sample {
		appearance: none;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-2);
		min-block-size: var(--target-min);
		min-inline-size: var(--target-min);
		padding: var(--space-1);
		margin: 0;
		border: 0;
		border-radius: var(--radius-sm);
		background: none;
		color: inherit;
		font: inherit;
		text-align: start;
		cursor: pointer;
	}

	.sample:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.specimen {
		display: block;
		max-inline-size: 100%;
	}

	.space {
		block-size: var(--space-3);
		min-inline-size: var(--space-1);
		background: var(--accent);
		border-radius: var(--radius-xs);
	}

	.radius {
		inline-size: var(--space-12);
		block-size: var(--space-12);
		background: var(--accent-soft);
		border: var(--hairline) solid var(--accent-border);
	}

	.elevation {
		inline-size: var(--space-12);
		block-size: var(--space-12);
		background: var(--surface-raised);
		border-radius: var(--radius-lg);
	}

	.motion {
		inline-size: var(--space-4);
		block-size: var(--space-4);
		border-radius: var(--radius-full);
		background: var(--accent);
	}

	.type {
		font-family: var(--font-sans);
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.label {
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		color: var(--text-muted);
	}
</style>
