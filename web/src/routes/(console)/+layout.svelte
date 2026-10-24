<script lang="ts">
	import type { Snippet } from 'svelte';
	import { session } from '$lib/stores/session.svelte';
	import AdminCluster from '$lib/ui/clusters/AdminCluster.svelte';
	import MemberCluster from '$lib/ui/clusters/MemberCluster.svelte';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
</script>

{#if session.isAdmin}
	<AdminCluster content={children} />
{:else}
	<MemberCluster content={children} />
{/if}
