<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Percent from '../atoms/Percent.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class' | 'style'> {
		num: number;
		den: number;
		class?: string;
	}

	let { num, den, class: className, ...rest }: Props = $props();
</script>

<Percent {num} {den} class={className} {...rest} />
