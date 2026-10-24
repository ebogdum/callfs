<script lang="ts">
	import MemberDetailPlanet from '$lib/ui/planets/cluster/MemberDetailPlanet.svelte';

	const { params }: { params: { id: string } } = $props();
</script>

<MemberDetailPlanet id={params.id} />
