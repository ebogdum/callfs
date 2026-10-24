<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { StatusKey } from '../types';
	import StatusPill from '../atoms/StatusPill.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		status: StatusKey;
		detail?: string;
	}

	let { status, detail, class: className, ...rest }: Props = $props();
</script>

<span class={['status-cell', className]} {...rest}>
	<StatusPill {status} size="sm" {detail} />
</span>

<style>
	.status-cell {
		display: inline-flex;
		max-inline-size: 100%;
		font-size: var(--table-text);
	}
</style>
