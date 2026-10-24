<script lang="ts">
	import type { Snippet } from 'svelte';
	import ActivitySystem from '$lib/ui/systems/ActivitySystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<ActivitySystem>{@render children()}</ActivitySystem>
