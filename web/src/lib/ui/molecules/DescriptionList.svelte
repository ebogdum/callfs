<script lang="ts" module>
	import type { Snippet } from 'svelte';

	export interface DescriptionItem {
		term: string;
		value: Snippet | string;
		copy?: string;
		help?: string;
	}

	export const TOKENS = ['--description-list-row-gap', '--description-list-cell-gap'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import CopyButton from '../atoms/CopyButton.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDListElement>, 'class'> {
		items: DescriptionItem[];
		columns?: 1 | 2;
		class?: string;
	}

	let { items, columns = 1, class: className, ...rest }: Props = $props();

	const uid = $props.id();
</script>

<dl class={['description-list', className]} data-columns={columns} {...rest}>
	{#each items as item, index (index)}
		{@const helpId = `${uid}-help-${index}`}
		<div class="row">
			<dt>
				<Text tone="muted" size="sm">{item.term}</Text>
			</dt>
			<dd aria-describedby={undefined !== item.help && '' !== item.help ? helpId : undefined}>
				<Cluster gap="2">
					{#if 'string' === typeof item.value}
						<Text>{item.value}</Text>
					{:else}
						{@render item.value()}
					{/if}
					{#if undefined !== item.copy && '' !== item.copy}
						<CopyButton value={item.copy} />
					{/if}
				</Cluster>
				{#if undefined !== item.help && '' !== item.help}
					<HelpText id={helpId}>{item.help}</HelpText>
				{/if}
			</dd>
		</div>
	{/each}
</dl>

<style>
	.description-list {
		display: grid;
		gap: var(--description-list-row-gap, var(--space-3));
		margin: 0;
		min-inline-size: 0;
	}

	.row {
		display: grid;
		gap: var(--description-list-cell-gap, var(--space-1));
		min-inline-size: 0;
	}

	dt,
	dd {
		margin: 0;
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	dd {
		display: grid;
		gap: var(--space-1);
	}

	@media (min-width: 768px) {
		.description-list[data-columns='2'] {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
