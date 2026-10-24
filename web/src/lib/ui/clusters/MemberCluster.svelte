<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import ConsoleGalaxy from '../galaxies/ConsoleGalaxy.svelte';
	import DegradedCluster, { type DegradedStatus } from './DegradedCluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		content: Snippet<[DegradedStatus]>;
		bulkBar?: Snippet;
		class?: string;
	}

	let { content, bulkBar, class: className, ...rest }: Props = $props();
</script>

<ConsoleGalaxy {bulkBar} class={className} {...rest}>
	<DegradedCluster role="member">
		{#snippet children(status)}
			{@render content(status)}
		{/snippet}
	</DegradedCluster>
</ConsoleGalaxy>
