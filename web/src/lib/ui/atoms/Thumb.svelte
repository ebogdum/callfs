<script lang="ts">
	import type { HTMLImgAttributes } from 'svelte/elements';
	import Frame from '../quarks/Frame.svelte';

	interface Props extends Omit<HTMLImgAttributes, 'src' | 'alt'> {
		src: string;
		alt: string;
		ratio?: '1/1' | '4/3' | '16/9' | '3/2';
	}

	let { src, alt, ratio = '4/3', class: className, ...rest }: Props = $props();
</script>

<Frame {ratio} class={['thumb', className]}>
	<img {...rest} {src} {alt} loading="lazy" decoding="async" />
</Frame>

<style>
	.thumb {
		border-radius: var(--radius-sm);
		background: var(--surface-sunken);
	}

	:global(:root[data-theme='dark']) .thumb img {
		filter: brightness(0.9);
	}

	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .thumb img {
			filter: brightness(0.9);
		}
	}
</style>
