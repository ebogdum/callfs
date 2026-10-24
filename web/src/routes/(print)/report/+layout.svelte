<script lang="ts">
	import type { Snippet } from 'svelte';
	import PrintUniverse from '$lib/ui/universes/PrintUniverse.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<PrintUniverse>
	{@render children()}
</PrintUniverse>
