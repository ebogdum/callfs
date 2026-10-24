<script lang="ts" module>
	import type { Snippet } from 'svelte';

	export interface Fact {
		term: string;
		value: string | Snippet;
		mono?: boolean;
		tone?: 'muted' | 'warn' | 'danger';
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLDListElement> {
		items: Fact[];
	}

	let { items, ...rest }: Props = $props();
</script>

<dl {...rest} class="facts">
	{#each items as item, index (index)}
		<div class="row">
			<dt>{item.term}</dt>
			<dd class:mono={true === item.mono} data-tone={item.tone}>
				{#if 'string' === typeof item.value}
					{item.value}
				{:else}
					{@render item.value()}
				{/if}
			</dd>
		</div>
	{/each}
</dl>

<style>
	.facts {
		display: grid;
		margin: 0;
		min-inline-size: 0;
		container-type: inline-size;
	}

	.row {
		display: grid;
		grid-template-columns: minmax(var(--space-16), calc(var(--space-4) * 9)) minmax(0, 1fr);
		align-items: baseline;
		gap: var(--space-4);
		padding-block: var(--space-2);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.row:first-child {
		border-block-start: 0;
		padding-block-start: 0;
	}

	.row:last-child {
		padding-block-end: 0;
	}

	dt {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
	}

	dd {
		margin: 0;
		min-inline-size: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-label-weight);
		color: var(--text);
		overflow-wrap: anywhere;
	}

	dd.mono {
		font-family: var(--font-mono);
		font-weight: 400;
	}

	dd[data-tone='muted'] {
		color: var(--text-muted);
		font-weight: 400;
	}

	dd[data-tone='warn'] {
		color: var(--warn-text);
	}

	dd[data-tone='danger'] {
		color: var(--danger-text);
	}

	@container (max-width: 22rem) {
		.row {
			grid-template-columns: minmax(0, 1fr);
			gap: var(--space-0-5);
		}
	}
</style>
