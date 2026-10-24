<script lang="ts">
	import type { Snippet } from 'svelte';
	import WallUniverse from '$lib/ui/universes/WallUniverse.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<WallUniverse>
	{@render children()}
</WallUniverse>
