<script lang="ts" module>
	export interface SegmentOption {
		value: string;
		label: string;
		icon?: IconName;
	}
</script>

<script lang="ts">
	import { ToggleGroup } from 'bits-ui';
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children' | 'id'> {
		id?: string;
		value?: string;
		options: SegmentOption[];
		label: string;
	}

	let { value = $bindable(''), options, label, class: className = '', ...rest }: Props = $props();

	const select = (next: string) => {
		if ('' !== next) {
			value = next;
		}
	};
</script>

<ToggleGroup.Root type="single" bind:value={() => value, select} aria-label={label} class="segmented {className}" {...rest}>
	{#each options as option (option.value)}
		<ToggleGroup.Item value={option.value} class="segment">
			{#if option.icon}<Icon name={option.icon} size="sm" />{/if}
			<span class="text">{option.label}</span>
		</ToggleGroup.Item>
	{/each}
</ToggleGroup.Root>

<style>
	.text {
		white-space: nowrap;
	}
</style>
