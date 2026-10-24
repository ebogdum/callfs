<script lang="ts">
	import type { Snippet } from 'svelte';
	import InsightsSystem from '$lib/ui/systems/InsightsSystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<InsightsSystem>{@render children()}</InsightsSystem>
