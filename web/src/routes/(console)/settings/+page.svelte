<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { session } from '$lib/stores/session.svelte';

	$effect(() => {
		if (null === session.me) {
			return;
		}
		void goto(`${base}/settings/${true === session.isAdmin ? 'network' : 'appearance'}`, { replaceState: true });
	});
</script>
