<script lang="ts">
	import { page } from '$app/state';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import ErrorPlanet from '$lib/ui/planets/ErrorPlanet.svelte';
	import NotFoundPlanet from '$lib/ui/planets/NotFoundPlanet.svelte';

	const failure: ApiFailure = $derived(null === page.error ? { kind: 'network' } : { kind: 'malformed' });
	const pageError = $derived(toUiError(failure, { action: 'errors.open_page' }));
</script>

{#if 404 === page.status}
	<NotFoundPlanet />
{:else}
	<ErrorPlanet error={pageError} />
{/if}
