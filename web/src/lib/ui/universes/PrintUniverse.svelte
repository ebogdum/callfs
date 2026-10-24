<script lang="ts">
	import type { Snippet } from 'svelte';
	import '$lib/ui/quarks/tokens/index.css';
	import '@fontsource-variable/inter';
	import '@fontsource-variable/jetbrains-mono';
	import PrintLayout from '$lib/ui/environments/PrintLayout.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

<PrintLayout>
	{@render children()}
</PrintLayout>
