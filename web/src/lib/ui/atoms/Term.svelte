<script lang="ts">
	import { Tooltip } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { GLOSSARY, type GlossaryKey } from '$lib/copy/glossary';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		term: GlossaryKey;
		children?: Snippet;
	}

	let { term, children, class: className, ...rest }: Props = $props();

	const entry = $derived(GLOSSARY[term]);
	const descriptionId = $props.id();
</script>

<Tooltip.Provider>
	<Tooltip.Root delayDuration={500}>
		<Tooltip.Trigger>
			{#snippet child({ props })}
				<span
					class={['term', className]}
					{...rest}
					{...props}
					tabindex="0"
					aria-describedby={descriptionId}
				>
					{#if children}
						{@render children()}
					{:else}
						{entry.word}
					{/if}
				</span>
			{/snippet}
		</Tooltip.Trigger>
		<Tooltip.Portal>
			<Tooltip.Content class="term-tooltip" side="top" sideOffset={4}>
				{entry.means}
			</Tooltip.Content>
		</Tooltip.Portal>
	</Tooltip.Root>
</Tooltip.Provider>
<VisuallyHidden id={descriptionId}>{entry.means}</VisuallyHidden>

<style>
	.term {
		border-radius: var(--radius-xs);
		text-decoration: underline dotted;
		text-decoration-color: var(--text-faint);
		text-underline-offset: var(--space-0-5);
		cursor: help;
	}

	.term:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	:global(.term-tooltip) {
		z-index: var(--z-tooltip);
		max-inline-size: var(--dialog-sm);
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-md);
		background: var(--surface-overlay);
		color: var(--text);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		box-shadow: var(--shadow-md);
	}
</style>
