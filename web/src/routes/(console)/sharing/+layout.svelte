<script lang="ts">
	import type { Snippet } from 'svelte';
	import SharingSystem from '$lib/ui/systems/SharingSystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<SharingSystem>{@render children()}</SharingSystem>
