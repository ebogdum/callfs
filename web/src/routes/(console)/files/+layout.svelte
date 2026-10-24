<script lang="ts">
	import type { Snippet } from 'svelte';
	import FilesSystem from '$lib/ui/systems/FilesSystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<FilesSystem>{@render children()}</FilesSystem>
