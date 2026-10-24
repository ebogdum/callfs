<script lang="ts">
	import { page } from '$app/state';
	import JobDetailPlanet from '$lib/ui/planets/activity/JobDetailPlanet.svelte';
</script>

<JobDetailPlanet id={page.params.id ?? ''} />
