<script lang="ts">
	import ErrorState from '../molecules/ErrorState.svelte';
	import type { UiError } from '../types';

	interface Props {
		error: UiError;
	}

	const { error }: Props = $props();
</script>

<main class="error-page">
	<ErrorState variant="page" {error} onretry={() => window.location.reload()} />
</main>

<style>
	.error-page {
		display: grid;
		padding: var(--space-10) var(--gutter);
	}
</style>
