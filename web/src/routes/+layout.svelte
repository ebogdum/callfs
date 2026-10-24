<script lang="ts">
	import type { Snippet } from 'svelte';
	import ConsoleUniverse from '$lib/ui/universes/ConsoleUniverse.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<ConsoleUniverse>
	{@render children()}
</ConsoleUniverse>
