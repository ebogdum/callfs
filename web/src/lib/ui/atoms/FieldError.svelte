<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends HTMLAttributes<HTMLParagraphElement> {
		id: string;
		message: string;
	}

	let { id, message, class: className, ...rest }: Props = $props();
</script>

<p {id} class={['error', className]} {...rest}>
	<StatusGlyph tone="danger" size="sm" />
	<span class="message">{message}</span>
</p>

<style>
	.error {
		display: flex;
		align-items: flex-start;
		gap: var(--space-1-5);
		color: var(--error-fg, var(--danger-text));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		letter-spacing: var(--text-sm-tracking);
		overflow-wrap: anywhere;
	}

	.message {
		min-inline-size: 0;
	}
</style>
