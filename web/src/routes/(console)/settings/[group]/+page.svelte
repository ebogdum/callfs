<script lang="ts">
	import { page } from '$app/state';
	import SettingsGroupPlanet from '$lib/ui/planets/settings/SettingsGroupPlanet.svelte';
</script>

<SettingsGroupPlanet group={page.params.group ?? ''} />
