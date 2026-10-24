<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Hairline from '../quarks/Hairline.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'role' | 'class'> {
		orientation?: 'horizontal' | 'vertical';
		class?: string;
	}

	let { orientation = 'horizontal', class: className, ...rest }: Props = $props();
</script>

<Hairline {orientation} decorative={false} class={className} {...rest} />
