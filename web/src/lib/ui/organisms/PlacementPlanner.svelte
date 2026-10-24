<script lang="ts" module>
	export interface PlacementPlan {
		total_shards: number;
		assignments: { index: number; instance_id: string; kind: 'data' | 'parity' }[];
		per_instance: Record<string, number>;
		max_on_one_instance: number;
		tolerated_instance_failures: number;
		tolerated_piece_losses: number;
		overhead_ratio: number;
		fault_tolerant: boolean;
		warnings: string[];
	}

	export const TOKENS = ['--planner-list-gap'] as const;

	const presets = [
		{ id: '2+1', data: 2, spare: 1, recommended: false },
		{ id: '4+2', data: 4, spare: 2, recommended: true },
		{ id: '6+3', data: 6, spare: 3, recommended: false },
		{ id: '8+4', data: 8, spare: 4, recommended: false }
	];
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { defaultLocale, formatFixed } from '$lib/format/number';
	import Badge from '../atoms/Badge.svelte';
	import Button from '../atoms/Button.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import NumberInput from '../atoms/NumberInput.svelte';
	import ProgressBar from '../atoms/ProgressBar.svelte';
	import Skeleton from '../atoms/Skeleton.svelte';
	import Text from '../atoms/Text.svelte';
	import FormField from '../molecules/FormField.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Grid from '../quarks/Grid.svelte';
	import Stack from '../quarks/Stack.svelte';
	import type { QueryStatus } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		data: number;
		spare: number;
		servers: string[];
		plan: PlacementPlan | null;
		status: QueryStatus;
		class?: string;
	}

	let {
		data = $bindable(2),
		spare = $bindable(1),
		servers,
		plan,
		status,
		class: className,
		...rest
	}: Props = $props();

	const barMax = $derived(null === plan ? 1 : Math.max(1, plan.total_shards));
	const concentrated = $derived(null !== plan && spare < plan.max_on_one_instance);
	const perServer = $derived(
		servers.map((id) => ({ id, count: null === plan ? 0 : (plan.per_instance[id] ?? 0) }))
	);

	const isActive = (preset: (typeof presets)[number]) => preset.data === data && preset.spare === spare;

	const applyPreset = (preset: (typeof presets)[number]) => {
		data = preset.data;
		spare = preset.spare;
	};
</script>

<section {...rest} class={['placement-planner', className]} aria-label={t('findings.planner.label')}>
	<Stack gap="4">
		<Grid min="12" gap="4">
			<FormField label={t('findings.planner.data')} for="placement-data" help={t('findings.planner.data_help')}>
				{#snippet control({ id, describedBy, invalid })}
					<NumberInput
						{id}
						aria-describedby={describedBy}
						aria-invalid={invalid}
						min={2}
						max={32}
						step={1}
						bind:value={data}
					/>
				{/snippet}
			</FormField>
			<FormField label={t('findings.planner.spare')} for="placement-spare" help={t('findings.planner.spare_help')}>
				{#snippet control({ id, describedBy, invalid })}
					<NumberInput
						{id}
						aria-describedby={describedBy}
						aria-invalid={invalid}
						min={1}
						max={32}
						step={1}
						bind:value={spare}
					/>
				{/snippet}
			</FormField>
		</Grid>

		<div role="group" aria-label={t('findings.planner.presets')}>
			<Cluster gap="2" align="center">
				{#each presets as preset (preset.id)}
					<Button variant="secondary" size="sm" aria-pressed={isActive(preset)} onclick={() => applyPreset(preset)}>
						{t('findings.planner.preset', { data: preset.data, spare: preset.spare })}
					</Button>
					{#if preset.recommended}
						<Badge tone="accent">{t('findings.planner.recommended')}</Badge>
					{/if}
				{/each}
			</Cluster>
		</div>

		{#if 'loading' === status}
			<Skeleton shape="rect" lines={3} height="var(--space-16)" />
		{:else if 'error' === status}
			<InlineAlert tone="danger" title={t('findings.planner.error_title')} body={t('findings.planner.error_body')} />
		{:else if null === plan}
			<HelpText id="placement-empty">{t('findings.planner.empty')}</HelpText>
		{:else}
			<Stack gap="3">
				{#if 'partial' === status}
					<InlineAlert tone="warn" title={t('findings.planner.partial')} />
				{/if}
				<Cluster gap="4" align="center">
					<Text size="title-sm">{t('findings.planner.can_lose', { count: plan.tolerated_instance_failures })}</Text>
					<Text size="title-sm">
						{t('findings.planner.overhead', { ratio: formatFixed(plan.overhead_ratio, 1, defaultLocale()) })}
					</Text>
				</Cluster>
				{#if concentrated}
					<InlineAlert
						tone="warn"
						title={t('findings.planner.concentrated_title', { pieces: plan.max_on_one_instance, spare })}
						body={t('findings.planner.concentrated_body')}
					/>
				{/if}
				<Text size="sm" tone="muted">{t('findings.planner.per_server')}</Text>
				<ul class="servers">
					{#each perServer as server (server.id)}
						<li>
							<ProgressBar value={server.count} max={barMax} label={server.id} showValue />
						</li>
					{/each}
				</ul>
				{#if 0 < plan.warnings.length}
					<ul class="notes">
						{#each plan.warnings as warning}
							<li>{warning}</li>
						{/each}
					</ul>
				{/if}
			</Stack>
		{/if}
	</Stack>
</section>

<style>
	.placement-planner {
		display: grid;
		min-inline-size: 0;
	}

	.servers,
	.notes {
		display: grid;
		gap: var(--planner-list-gap, var(--space-2));
		margin: 0;
		padding: 0;
		list-style: none;
	}
</style>
