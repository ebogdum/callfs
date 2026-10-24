<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLLabelAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLLabelAttributes {
		for: string;
		optional?: boolean;
		children: Snippet;
	}

	let { for: htmlFor, optional = false, class: className, children, ...rest }: Props = $props();
</script>

<label class={['label', className]} for={htmlFor} {...rest}>
	{@render children()}{#if optional}<span class="optional">({t('common.optional')})</span>{/if}
</label>

<style>
	.label {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1);
		max-inline-size: 100%;
		color: var(--label-fg, var(--text));
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-label-weight);
		letter-spacing: var(--text-body-tracking);
		overflow-wrap: anywhere;
	}

	.optional {
		color: var(--label-optional-fg, var(--text-muted));
		font-weight: 400;
	}
</style>
