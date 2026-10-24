<script lang="ts">
	import DegradedCluster, { type ClusterRole } from './DegradedCluster.svelte';

	let { role }: { role: ClusterRole } = $props();
</script>

<DegradedCluster {role}>
	{#snippet children(status)}
		<p data-testid="status" data-degraded={String(status.degraded)}>{status.disabledReason ?? ''}</p>
	{/snippet}
</DegradedCluster>
