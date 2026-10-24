<script lang="ts">
	import { base } from '$app/paths';
	import { goto } from '$app/navigation';
	import { rateLimits } from '$lib/api/admin/runtime';
	import type { ApiFailure } from '$lib/api/errors';
	import type { RateLimits, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Num from '$lib/ui/atoms/Num.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import { formatNumber } from '$lib/format/number';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import BarChart from '$lib/ui/organisms/chart/BarChart.svelte';
	import type { ColumnDef } from '$lib/ui/types';

	type TopClient = RateLimits['top_clients'][number];

	async function loadLimits(): Promise<RateLimits> {
		const outcome: Result<RateLimits, ApiFailure> = await rateLimits();
		return outcome.ok ? outcome.value : Promise.reject(outcome.error);
	}

	const limitsQuery = createQuery({
		key: ['ratelimits'],
		fetch: loadLimits,
		isEmpty: (data: RateLimits) => 0 === data.limits.length,
		isPartial: (data: RateLimits) => data.top_clients.length < data.tracked_clients
	});

	const endpointNames: Record<string, string> = {
		download: t('runtime.limits.endpoint.download'),
		link_generate: t('runtime.limits.endpoint.link_generate'),
		session: t('runtime.limits.endpoint.session')
	};

	function endpointName(endpoint: string): string {
		return endpointNames[endpoint] ?? endpoint;
	}

	function rejectedFor(data: RateLimits, endpoint: string): number {
		return (data.rejections_total as Record<string, number>)[endpoint] ?? 0;
	}

	function clientBars(data: RateLimits, endpoint: string): { label: string; value: number }[] {
		return data.top_clients
			.filter((client: TopClient) => endpoint === client.endpoint)
			.map((client: TopClient) => ({ label: client.client, value: client.rejections }));
	}

	function retry(): void {
		void limitsQuery.refetch();
	}

	function openSettings(): void {
		void goto(`${base}/settings/limits`);
	}

	function totalRejected(data: RateLimits): number {
		return Object.values(data.rejections_total as Record<string, number>).reduce((sum, count) => sum + count, 0);
	}

	const endpointIcons: Record<string, 'download' | 'link' | 'key'> = {
		download: 'download',
		link_generate: 'link',
		session: 'key'
	};

	const columns: ColumnDef<TopClient>[] = [
		{ id: 'client', header: t('runtime.limits.col.client'), cell: clientCell, sortKey: 'client' },
		{ id: 'endpoint', header: t('runtime.limits.col.endpoint'), cell: endpointCell },
		{ id: 'rejected', header: t('runtime.limits.col.rejected'), cell: rejectedCell, align: 'end', sortKey: 'rejections' }
	];
</script>

{#snippet clientCell(client: TopClient)}
	<Text mono truncate>{client.client}</Text>
{/snippet}

{#snippet endpointCell(client: TopClient)}
	<Text>{endpointName(client.endpoint)}</Text>
{/snippet}

{#snippet rejectedCell(client: TopClient)}
	<Num value={client.rejections} />
{/snippet}

{#snippet primary()}
	<Button variant="primary" iconStart="sliders" onclick={openSettings}>{t('runtime.limits.change')}</Button>
{/snippet}

<PageFrame title={t('runtime.limits.title')} job={t('runtime.limits.job')} {primary}>
	<Stack gap="8">
		{#if undefined !== limitsQuery.data}
			{#if 'empty' === limitsQuery.status}
				<EmptyState
					variant="not-available"
					icon="limit"
					title={t('runtime.limits.empty.title')}
					body={t('runtime.limits.empty.body')}
					primary={{ label: t('runtime.limits.change'), onclick: openSettings }}
				/>
			{:else}
				{@const limitData = limitsQuery.data}
				<Stack gap="4">
					{#if limitsQuery.error}
						<InlineAlert tone="danger" title={limitsQuery.error.title} body={limitsQuery.error.body} />
					{/if}
					<KpiStrip
						tiles={[
							{ label: t('runtime.limits.tracked'), value: limitData.tracked_clients, unit: 'count', base: { num: limitData.tracked_clients, den: limitData.max_entries } },
							{ label: t('cluster.limits.kpi.rejected'), value: totalRejected(limitData), unit: 'count' },
							{ label: t('cluster.limits.kpi.limits'), value: limitData.limits.length, unit: 'count' }
						]}
					/>
					<InlineAlert tone="info" title={t('runtime.limits.proxy')} />
				</Stack>
				<div class="cards">
					{#each limitData.limits as limit (limit.endpoint)}
						{@const rejected = rejectedFor(limitData, limit.endpoint)}
						{@const bars = clientBars(limitData, limit.endpoint)}
						<ClusterCard
							title={endpointName(limit.endpoint)}
							description={t('runtime.limits.rate', { rps: limit.rps, burst: limit.burst })}
							icon={endpointIcons[limit.endpoint] ?? 'limit'}
							tone={0 < rejected ? 'warn' : 'neutral'}
						>
							<p class="figure">
								<span class="figure-value">{formatNumber(rejected, 'int')}</span>
								<span class="figure-label">{t('cluster.limits.rejected_unit')}</span>
							</p>
							{#if 0 < bars.length}
								<BarChart data={bars} unit="count" label={t('runtime.limits.by_client')} />
							{:else}
								<Text size="sm" tone="muted">{t('runtime.limits.by_client.none')}</Text>
							{/if}
						</ClusterCard>
					{/each}
				</div>
				<section class="region" aria-labelledby="limits-top">
					<div class="region-head">
						<Heading level={2} size="title-sm" id="limits-top">{t('runtime.limits.top')}</Heading>
						{#if 'partial' === limitsQuery.status}
							<Text size="sm" tone="muted">{t('runtime.limits.partial.title')} {t('runtime.limits.partial.body')}</Text>
						{/if}
					</div>
					<DataTable
						rows={limitData.top_clients}
						{columns}
						rowKey={(client: TopClient) => `${client.endpoint}|${client.client}`}
						label={t('runtime.limits.top.table')}
						status={0 < limitData.top_clients.length ? 'success' : 'empty'}
					>
						{#snippet empty()}
							<EmptyState
								variant="cleared"
								icon="limit"
								title={t('cluster.limits.top.empty')}
								body={t('runtime.limits.by_client.none')}
							/>
						{/snippet}
					</DataTable>
				</section>
			{/if}
		{:else if limitsQuery.error}
			<ErrorState variant="page" error={limitsQuery.error} onretry={retry} />
		{:else}
			<Skeleton shape="rect" height="12rem" />
		{/if}
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 16)), 1fr));
		gap: var(--space-4);
	}

	.region,
	.region-head {
		display: grid;
		gap: var(--space-3);
	}

	.region-head {
		gap: var(--space-1);
	}

	.figure {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
		margin: 0;
	}

	.figure-value {
		font-size: var(--text-heading-size);
		line-height: var(--text-heading-line);
		font-weight: var(--text-heading-weight);
		letter-spacing: var(--text-heading-tracking);
		font-variant-numeric: tabular-nums;
		color: var(--text);
	}

	.figure-label {
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}
</style>
