<script lang="ts">
	import type { Snippet } from 'svelte';
	import ClusterSystem from '$lib/ui/systems/ClusterSystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<ClusterSystem>{@render children()}</ClusterSystem>
