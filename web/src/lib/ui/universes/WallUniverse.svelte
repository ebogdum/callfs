<script lang="ts">
	import type { Snippet } from 'svelte';
	import '$lib/ui/quarks/tokens/index.css';
	import '@fontsource-variable/inter';
	import '@fontsource-variable/jetbrains-mono';
	import WallLayout from '$lib/ui/environments/WallLayout.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<WallLayout>
	{@render children()}
</WallLayout>
