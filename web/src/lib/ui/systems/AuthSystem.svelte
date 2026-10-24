<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { session } from '$lib/stores/session.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	const FALLBACK_TARGET = '/';

	const safeRedirect = (value: string | null): string => {
		if (null === value || !value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) {
			return FALLBACK_TARGET;
		}
		return value;
	};

	let { children, class: className, ...rest }: Props = $props();

	const target = $derived(safeRedirect(page.url.searchParams.get('redirect')));

	$effect(() => {
		if (null === session.me) {
			return;
		}
		void goto(`${base}${target}`, { replaceState: true });
	});
</script>

<div class={['auth-system', className]} {...rest}>
	{@render children()}
</div>

<style>
	.auth-system {
		display: contents;
	}
</style>
