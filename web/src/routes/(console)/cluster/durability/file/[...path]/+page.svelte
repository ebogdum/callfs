<script lang="ts">
	import { page } from '$app/state';
	import SplitFilePlanet from '$lib/ui/planets/cluster/SplitFilePlanet.svelte';
</script>

<SplitFilePlanet path={page.params.path ?? ''} />
