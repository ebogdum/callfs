<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLElement> {
		ratio: '1/1' | '4/3' | '16/9' | '3/2';
		children: Snippet;
	}

	let { ratio, class: className, children, ...rest }: Props = $props();
</script>

<div
	class={['frame', className]}
	style:--frame-ratio={ratio.replace('/', ' / ')}
	{...rest}
>
	{@render children()}
</div>

<style>
	.frame {
		display: block;
		inline-size: 100%;
		min-inline-size: 0;
		aspect-ratio: var(--frame-ratio);
		overflow: hidden;
	}

	.frame > :global(*) {
		inline-size: 100%;
		block-size: 100%;
		object-fit: cover;
	}
</style>
