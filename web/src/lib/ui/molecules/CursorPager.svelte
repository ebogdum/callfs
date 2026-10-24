<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'children'> {
		hasPrev: boolean;
		hasNext: boolean;
		onprev: () => void;
		onnext: () => void;
		countLabel: string;
		class?: string;
	}

	let { hasPrev, hasNext, onprev, onnext, countLabel, class: className, ...rest }: Props = $props();
</script>

<Cluster
	as="nav"
	gap="3"
	justify="space-between"
	class={['cursor-pager', className]}
	aria-label={t('tablecontrols.pagination.label')}
	{...rest}
>
	<Text tone="muted" tabular>{countLabel}</Text>
	<Cluster gap="2">
		<Button size="sm" variant="secondary" iconStart="chevron-left" disabled={!hasPrev} onclick={onprev}>
			{t('tablecontrols.pagination.previous')}
		</Button>
		<Button size="sm" variant="secondary" iconEnd="chevron-right" disabled={!hasNext} onclick={onnext}>
			{t('tablecontrols.pagination.next')}
		</Button>
	</Cluster>
</Cluster>

<style>
	:global(.cursor-pager) {
		flex: 1 1 auto;
		min-inline-size: 0;
	}
</style>
