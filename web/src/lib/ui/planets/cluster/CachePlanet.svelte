<script lang="ts">
	import { untrack } from 'svelte';
	import { base } from '$app/paths';
	import { cache, invalidateCache } from '$lib/api/admin/runtime';
	import { toUiError } from '$lib/api/errors';
	import { register } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatNumber } from '$lib/format/number';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import PathInput from '$lib/ui/molecules/PathInput.svelte';
	import ConfirmInline from '$lib/ui/molecules/ConfirmInline.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import BarChart from '$lib/ui/organisms/chart/BarChart.svelte';

	const cacheQuery = createQuery({
		key: ['cache'],
		fetch: () => cache()
	});

	const cacheMutation = createMutation({
		run: (vars: { all: true } | { prefix: string }) => invalidateCache(vars),
		invalidate: () => [['cache']],
		context: { action: 'runtime.cache.clear' }
	});

	let prefix = $state('');
	let confirmAll = $state(false);
	let confirmPath = $state(false);
	let clearedCount = $state<number | null>(null);

	const scopedPrefix = $derived(prefix.trim());
	const cacheOutcome = $derived(cacheQuery.data);
	const stats = $derived(undefined !== cacheOutcome && true === cacheOutcome.ok ? cacheOutcome.value : null);
	const loadError = $derived(undefined !== cacheOutcome && false === cacheOutcome.ok ? toUiError(cacheOutcome.error, { action: 'runtime.cache.job', read: true }) : cacheQuery.error);

	async function clearAll(): Promise<void> {
		const result = await cacheMutation.run({ all: true });
		if (null !== result && true === result.ok) {
			confirmAll = false;
			clearedCount = result.value.invalidated;
		}
	}

	async function clearPath(): Promise<void> {
		const result = await cacheMutation.run({ prefix: scopedPrefix });
		if (null !== result && true === result.ok) {
			confirmPath = false;
			clearedCount = result.value.invalidated;
		}
	}

	function retry(): void {
		void cacheQuery.refetch();
	}

	$effect(() =>
		untrack(() => register([
			{
				id: 'cluster.cache.clear',
				title: t('runtime.cache.command.clear'),
				group: 'Cluster',
				keywords: [t('runtime.cache.command.keywords')],
				run: () => {
					confirmAll = true;
				}
			},
			{
				id: 'cluster.cache.clear_path',
				title: t('runtime.cache.command.clear_path'),
				group: 'Cluster',
				keywords: [t('runtime.cache.command.keywords')],
				run: () => {
					confirmPath = true;
				},
				disabledReason: () => (0 === scopedPrefix.length ? t('runtime.cache.path.required') : null)
			}
		]))
	);
</script>

{#snippet primary()}
	{#if null !== stats && false !== stats.enabled}
		<Button variant="primary" iconStart="trash" onclick={() => (confirmAll = true)}>{t('runtime.cache.clear')}</Button>
	{/if}
{/snippet}

<PageFrame title={t('runtime.cache.title')} job={t('runtime.cache.job')} {primary}>
	<Stack gap="6">
		{#if null !== stats && false === stats.enabled}
			<EmptyState
				variant="not-available"
				icon="cache"
				title={t('runtime.cache.off.title')}
				body={t('runtime.cache.off.body')}
				primary={{ label: t('runtime.cache.off.link'), href: `${base}/settings/index` }}
			/>
		{:else if null !== stats}
			{#if loadError}
				<InlineAlert tone="danger" title={loadError.title} body={loadError.body} />
			{/if}
			{#if confirmAll}
				<ConfirmInline
					question={t('runtime.cache.confirm_all.question')}
					consequence={t('runtime.cache.confirm_all.consequence')}
					confirmLabel={t('runtime.cache.confirm_all.action')}
					cancelLabel={t('runtime.cache.cancel')}
					tone="warn"
					busy={cacheMutation.pending}
					onconfirm={() => void clearAll()}
					oncancel={() => (confirmAll = false)}
				/>
			{/if}
			{#if null !== clearedCount}
				<InlineAlert
					tone="ok"
					role="status"
					title={t('cluster.cache.cleared', { count: formatNumber(clearedCount, 'int') })}
				/>
			{/if}
			{#if cacheMutation.error}
				<InlineAlert tone="danger" title={cacheMutation.error.title} body={cacheMutation.error.body} />
			{/if}
			<KpiStrip
				tiles={[
					{ label: t('runtime.cache.kpi.hits'), value: stats.hits, unit: 'count', base: { num: stats.hits, den: stats.hits + stats.misses } },
					{ label: t('runtime.cache.kpi.entries'), value: stats.entries, unit: 'count', base: { num: stats.entries, den: stats.max_entries } },
					{ label: t('runtime.cache.kpi.ttl'), value: stats.ttl_ms, unit: 'ms' },
					{ label: t('runtime.cache.kpi.evictions'), value: stats.evictions, unit: 'count' }
				]}
			/>
			<div class="cards">
				<ClusterCard title={t('runtime.cache.chart')} description={t('cluster.cache.chart.body')} icon="bar-chart">
					<BarChart
						data={[
							{ label: t('runtime.cache.chart.hits'), value: stats.hits, colorIndex: 1 },
							{ label: t('runtime.cache.chart.misses'), value: stats.misses, colorIndex: 4 }
						]}
						unit="count"
						label={t('runtime.cache.chart')}
					/>
				</ClusterCard>
				<ClusterCard title={t('cluster.cache.path.title')} description={t('cluster.cache.path.body')} icon="folder">
					<PathInput bind:value={prefix} label={t('runtime.cache.path.label')} onsubmit={() => (confirmPath = 0 < scopedPrefix.length)} />
					{#if confirmPath}
						<ConfirmInline
							question={t('runtime.cache.path.question', { path: scopedPrefix })}
							consequence={t('runtime.cache.path.consequence')}
							confirmLabel={t('runtime.cache.path.action')}
							cancelLabel={t('runtime.cache.cancel')}
							tone="warn"
							busy={cacheMutation.pending}
							onconfirm={() => void clearPath()}
							oncancel={() => (confirmPath = false)}
						/>
					{/if}
					{#snippet footer()}
						<Button
							variant="secondary"
							size="sm"
							disabledReason={0 === scopedPrefix.length ? t('runtime.cache.path.required') : undefined}
							onclick={() => (confirmPath = true)}
						>
							{t('runtime.cache.path.button')}
						</Button>
					{/snippet}
				</ClusterCard>
			</div>
		{:else if loadError}
			<ErrorState variant="page" error={loadError} onretry={retry} />
		{:else}
			<Skeleton shape="rect" height="12rem" />
		{/if}
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 20)), 1fr));
		gap: var(--space-4);
	}
</style>
