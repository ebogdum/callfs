<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Chip } from '../types';
	import Button from '../atoms/Button.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import FilterChip from './FilterChip.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLElement> {
		chips: Chip[];
		onremove: (chip: Chip) => void;
		onclear: () => void;
		label: string;
	}

	let { chips, onremove, onclear, label, class: className, ...rest }: Props = $props();
</script>

{#if 0 < chips.length}
	<Cluster gap="2" role="group" aria-label={label} class={className} {...rest}>
		{#each chips as chip (`${chip.facet}${chip.op}${chip.value}`)}
			<FilterChip {chip} {onremove} />
		{/each}
		<Button variant="ghost" size="sm" onclick={onclear}>{t('u076molecules.chip_bar_clear')}</Button>
	</Cluster>
{/if}
