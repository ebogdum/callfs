<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Kbd from '../atoms/Kbd.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		keys: string[];
		label?: string;
		class?: string;
	}

	let { keys, label, class: className, ...rest }: Props = $props();
</script>

<Cluster as="span" gap="2" class={['shortcut-hint', className]} {...rest}>
	<Kbd {keys} />
	{#if undefined !== label && '' !== label}
		<Text size="sm" tone="muted">{label}</Text>
	{/if}
</Cluster>
