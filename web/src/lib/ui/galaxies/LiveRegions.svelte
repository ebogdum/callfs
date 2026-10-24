<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		class?: string;
	}

	let { class: className, ...rest }: Props = $props();
</script>

<div class={['live-regions', className]} {...rest}>
	<div id="live-polite" class="region" role="status" aria-live="polite" aria-atomic="true"></div>
	<div id="live-assertive" class="region" role="alert" aria-live="assertive" aria-atomic="true"></div>
</div>

<style>
	.live-regions {
		display: contents;
	}

	.region {
		position: absolute;
		inline-size: var(--hairline);
		block-size: var(--hairline);
		overflow: hidden;
		clip-path: inset(100%);
		white-space: nowrap;
	}
</style>
