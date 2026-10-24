<script lang="ts">
	import { page } from '$app/state';
	import LinkDetailPlanet from '$lib/ui/planets/sharing/LinkDetailPlanet.svelte';
</script>

<LinkDetailPlanet id={page.params.id ?? ''} />
