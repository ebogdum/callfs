<script lang="ts">
	import type { Snippet } from 'svelte';
	import SettingsSystem from '$lib/ui/systems/SettingsSystem.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<SettingsSystem>
	{@render children()}
</SettingsSystem>
