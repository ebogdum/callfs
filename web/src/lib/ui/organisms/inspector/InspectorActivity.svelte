<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ComponentProps } from 'svelte';
	import type { QueryStatus } from '../../types';
	import Timeline from '../Timeline.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import Text from '../../atoms/Text.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		events: ComponentProps<typeof Timeline>['items'];
		status: QueryStatus;
		class?: string;
	}

	let { events, status, class: className, ...rest }: Props = $props();
</script>

<div class={['inspector-activity', className]} {...rest}>
	{#if 'loading' === status}
		<Skeleton shape="text" lines={3} />
	{:else if 'error' === status}
		<InlineAlert tone="danger" title={t('inspector.activity.error.title')} body={t('inspector.activity.error.body')} />
	{:else}
		<Stack gap="3">
			{#if 'partial' === status}
				<InlineAlert tone="warn" title={t('inspector.activity.partial')} />
			{/if}
			{#if 0 === events.length}
				<Text tone="muted">{t('inspector.activity.empty')}</Text>
			{:else}
				<Timeline items={events} />
			{/if}
		</Stack>
	{/if}
</div>
