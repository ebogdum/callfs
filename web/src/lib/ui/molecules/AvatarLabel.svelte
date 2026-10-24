<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Avatar from '../atoms/Avatar.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		name: string;
		secondary?: string;
		class?: string;
	}

	let { name, secondary, class: className, ...rest }: Props = $props();
</script>

<Cluster gap="2" class="avatar-label {className}" {...rest}>
	<Avatar {name} />
	<div class="names">
		<Text truncate>{name}</Text>
		{#if undefined !== secondary && '' !== secondary}
			<Text size="sm" tone="muted" truncate>{secondary}</Text>
		{/if}
	</div>
</Cluster>

<style>
	.names {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}
</style>
