<script lang="ts">
	import { base } from '$app/paths';
	import { series as metricSeries } from '$lib/api/admin/telemetry';
	import { backends as listBackends, probeBackend, type BackendProbe } from '$lib/api/admin/topology';
	import type { ApiFailure } from '$lib/api/errors';
	import type { Backend, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatNumber } from '$lib/format/number';
	import { formatRelative } from '$lib/format/time';
	import type { StatusKey } from '$lib/status/catalog';
	import Badge from '$lib/ui/atoms/Badge.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Num from '$lib/ui/atoms/Num.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import ThresholdBar from '$lib/ui/molecules/ThresholdBar.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList, { type Fact } from './FactList.svelte';

	const BACKEND_REFRESH_MS = 10000;
	const DAY_SECONDS = 86400;
	const HOUR_SECONDS = 3600;
	const USAGE_WINDOW_SECONDS = 7 * DAY_SECONDS;
	const WARN_RATIO = 0.8;

	const cardTitleKeys: Record<string, string> = {
		localfs: 'servers-storage.storage.card.localfs',
		s3: 'servers-storage.storage.card.s3',
		internal_proxy: 'servers-storage.storage.card.internal_proxy'
	};

	const valueOrReject = <T,>(outcome: Result<T, ApiFailure>): Promise<T> =>
		outcome.ok ? Promise.resolve(outcome.value) : Promise.reject(outcome.error);

	const backendsQuery = createQuery({
		key: ['backends'],
		fetch: async () => valueOrReject(await listBackends()),
		refetchMs: BACKEND_REFRESH_MS,
		isEmpty: (page) => 0 === page.items.length,
		isPartial: (page) => page.items.some((backend) => 'down' === backend.status)
	});

	const usageQuery = createQuery({
		key: ['series', 'callfs_usage_bytes'],
		fetch: () => {
			const nowSeconds = Math.floor(Date.now() / 1000);
			return metricSeries({
				name: 'callfs_usage_bytes',
				measure: 'value',
				from: nowSeconds - USAGE_WINDOW_SECONDS,
				to: nowSeconds,
				stepS: HOUR_SECONDS
			}).then(valueOrReject);
		},
		staleMs: 60000
	});

	const backends = $derived(backendsQuery.data?.items ?? []);
	const defaultName = $derived(backendsQuery.data?.default ?? null);
	const cardBackends = $derived(backends.filter((backend) => Object.hasOwn(cardTitleKeys, backend.name)));
	const localDisk = $derived(backends.find((backend) => 'localfs' === backend.name));
	const usagePoints = $derived(usageQuery.data?.series[0]?.points ?? []);
	const staleState = $derived('error' === backendsQuery.status ? 'offline' : 'live');
	const queryError = $derived(backendsQuery.error);

	const probeMutation = createMutation<void, BackendProbe[]>({
		run: () =>
			Promise.all(
				backends
					.filter((backend) => true === backend.enabled)
					.map(async (backend) => valueOrReject(await probeBackend(backend.name)))
			),
		invalidate: () => [['backends']],
		success: () => t('servers-storage.storage.checked'),
		context: { action: 'servers-storage.storage.check' }
	});

	const projectDaysToFull = (points: [number, number | null][], totalBytes: number | null): number | null => {
		const samples = points.filter((point): point is [number, number] => null !== point[1]);
		if (null === totalBytes || 2 > samples.length) {
			return null;
		}
		const [firstAt, firstBytes] = samples[0];
		const [lastAt, lastBytes] = samples[samples.length - 1];
		const spanSeconds = lastAt - firstAt;
		if (0 >= spanSeconds) {
			return null;
		}
		const bytesPerSecond = (lastBytes - firstBytes) / spanSeconds;
		if (0 >= bytesPerSecond) {
			return null;
		}
		return Math.max(0, (totalBytes - lastBytes) / bytesPerSecond / DAY_SECONDS);
	};

	const fullInText = $derived.by(() => {
		const days = projectDaysToFull(usagePoints, localDisk?.total_bytes ?? null);
		if (null === days) {
			return t('servers-storage.storage.no_projection');
		}
		const roundedDays = Math.round(days);
		if (1 > roundedDays) {
			return t('servers-storage.storage.full_soon');
		}
		return t('servers-storage.storage.full_in', { days: formatNumber(roundedDays, 'int') });
	});

	const diskThresholds = (totalBytes: number) => [
		{ value: Math.round(totalBytes * WARN_RATIO), tone: 'warn' as const, label: t('servers-storage.storage.disk.warn') }
	];

	const backendStatus = (backend: Backend): StatusKey => {
		if ('ok' === backend.status) {
			return 'storage.on';
		}
		if ('disabled' === backend.status) {
			return 'storage.off';
		}
		if ('warn' === backend.status) {
			return 'health.warn';
		}
		return 'storage.error';
	};

	const probeStatus = (backend: Backend): StatusKey => (true === backend.last_probe?.ok ? 'health.ok' : 'health.down');

	const isPublicAcl = (backend: Backend) => 'string' === typeof backend.acl && backend.acl.startsWith('public');

	const SETTINGS_HREF = `${base}/settings/storage`;

	const cardIcons: Record<string, 'hard-drive' | 'cloud' | 'network'> = {
		localfs: 'hard-drive',
		s3: 'cloud',
		internal_proxy: 'network'
	};

	const cardDescriptionKeys: Record<string, string> = {
		localfs: 'cluster.storage.localfs.description',
		s3: 'cluster.storage.s3.description',
		internal_proxy: 'cluster.storage.internal_proxy.description'
	};

	const isOff = (backend: Backend) => 'disabled' === backend.status || false === backend.enabled;

	const orderedBackends = $derived(
		[...cardBackends].sort((left, right) => {
			const rank = (backend: Backend) => (defaultName === backend.name ? 0 : isOff(backend) ? 2 : 1);
			return rank(left) - rank(right);
		})
	);

	const fact = (term: string, value: string | null, unset: string, mono = false): Fact =>
		null === value ? { term, value: unset, tone: 'muted' } : { term, value, mono };

	const yesOrNo = (value: boolean) =>
		true === value ? t('servers-storage.storage.summary.yes') : t('servers-storage.storage.summary.no');

	const summaryItems = (backend: Backend): Fact[] => {
		if ('localfs' === backend.name) {
			return [
				fact(t('servers-storage.storage.summary.root_folder'), backend.root_path, t('servers-storage.storage.summary.root_unset'), true)
			];
		}
		if ('s3' === backend.name) {
			return [
				fact(t('servers-storage.storage.summary.bucket'), backend.bucket, t('servers-storage.storage.summary.bucket_unset'), true),
				fact(t('servers-storage.storage.summary.region'), backend.region, t('servers-storage.storage.summary.region_unset')),
				fact(t('servers-storage.storage.summary.endpoint'), backend.endpoint, t('servers-storage.storage.summary.endpoint_unset'), true),
				fact(t('servers-storage.storage.summary.encryption'), backend.encryption, t('servers-storage.storage.summary.encryption_unset')),
				fact(t('servers-storage.storage.summary.access'), backend.acl, t('servers-storage.storage.summary.access_unset')),
				{ term: t('servers-storage.storage.summary.kms'), value: yesOrNo(backend.kms_key_set) }
			];
		}
		return [{ term: t('servers-storage.storage.summary.servers'), value: formatNumber(backend.peers, 'int') }];
	};
</script>

{#snippet probeLine(backend: Backend)}
	<span class="probe">
		<Text size="sm" tone="muted">{t('servers-storage.storage.probe.label')}</Text>
		{#if null === backend.last_probe}
			<Text size="sm">{t('servers-storage.storage.probe.never')}</Text>
		{:else}
			<StatusPill status={probeStatus(backend)} size="sm" />
			{#if null !== backend.last_probe.latency_ms}
				<Text size="sm" tabular><Num value={backend.last_probe.latency_ms} unit={t('servers-storage.unit.ms')} /></Text>
			{/if}
			<Text size="sm" tone="muted">
				{t('servers-storage.storage.probe.checked', {
					when: formatRelative(new Date(backend.last_probe.at), new Date())
				})}
			</Text>
		{/if}
	</span>
{/snippet}

{#snippet backendCard(backend: Backend)}
	<ClusterCard
		class={defaultName === backend.name ? 'wide' : undefined}
		title={t(cardTitleKeys[backend.name])}
		description={t(cardDescriptionKeys[backend.name])}
		icon={cardIcons[backend.name]}
		tone={'down' === backend.status ? 'danger' : 'warn' === backend.status ? 'warn' : 'neutral'}
	>
		{#snippet meta()}
			{#if defaultName === backend.name}
				<Badge tone="accent">{t('servers-storage.storage.default_badge')}</Badge>
			{/if}
			<StatusPill status={backendStatus(backend)} size="sm" />
		{/snippet}
		{#if isOff(backend)}
			<Text size="sm" tone="muted">{t('cluster.storage.off_body')}</Text>
		{:else}
			{#if 'localfs' === backend.name}
				{#if null === backend.total_bytes || null === backend.used_bytes}
					<Text size="sm" tone="muted">{t('servers-storage.storage.disk.unknown')}</Text>
				{:else}
					<div class="usage">
						<ThresholdBar
							value={backend.used_bytes}
							max={backend.total_bytes}
							free={backend.free_bytes}
							thresholds={diskThresholds(backend.total_bytes)}
							label={t('servers-storage.storage.disk.label')}
							unit="bytes"
						/>
						<Text size="sm" tone="muted">{fullInText}</Text>
					</div>
				{/if}
			{/if}
			<FactList items={summaryItems(backend)} />
			{#if isPublicAcl(backend)}
				<InlineAlert tone="warn" title={t('servers-storage.storage.acl.public')} />
			{/if}
		{/if}
		{#snippet footer()}
			{#if isOff(backend)}
				<Link href={SETTINGS_HREF}>{t('cluster.storage.set_up')}</Link>
			{:else}
				{@render probeLine(backend)}
				<span class="push"></span>
				<Link href={SETTINGS_HREF}>{t('cluster.storage.open_settings')}</Link>
			{/if}
		{/snippet}
	</ClusterCard>
{/snippet}

<PageFrame title={t('servers-storage.storage.title')} job={t('servers-storage.storage.job')}>
	{#snippet primary()}
		<Button
			variant="primary"
			iconStart="activity"
			loading={probeMutation.pending}
			onclick={() => {
				void probeMutation.run();
			}}
		>
			{t('servers-storage.storage.check')}
		</Button>
	{/snippet}
	{#snippet status()}
		<StaleIndicator
			updatedAt={backendsQuery.updatedAt}
			intervalMs={BACKEND_REFRESH_MS}
			state={staleState}
			onrefresh={() => backendsQuery.refetch()}
		/>
	{/snippet}

	<Stack gap="6">
		{#if 'partial' === backendsQuery.status}
			<InlineAlert tone="warn" title={t('servers-storage.storage.partial')} />
		{/if}
		{#if null !== probeMutation.error}
			<InlineAlert tone="danger" title={probeMutation.error.title} body={probeMutation.error.body} />
		{/if}
		{#if 'loading' === backendsQuery.status}
			<div class="cards">
				<Skeleton shape="rect" lines={4} />
				<Skeleton shape="rect" lines={4} />
			</div>
		{:else if null !== queryError && undefined === backendsQuery.data}
			<ErrorState error={queryError} variant="region" onretry={() => backendsQuery.refetch()} />
		{:else if 'empty' === backendsQuery.status}
			<EmptyState
				variant="first-use"
				icon="hard-drive"
				title={t('servers-storage.storage.empty.title')}
				body={t('servers-storage.storage.empty.body')}
				primary={{ label: t('servers-storage.storage.settings_link'), href: SETTINGS_HREF }}
			/>
		{:else}
			<div class="cards">
				{#each orderedBackends as backend (backend.name)}
					{@render backendCard(backend)}
				{/each}
			</div>
		{/if}
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 20)), 1fr));
		gap: var(--space-4);
	}

	.cards > :global(.wide) {
		grid-column: 1 / -1;
	}

	.usage {
		display: grid;
		gap: var(--space-2);
	}

	.probe {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.push {
		flex: 1 1 auto;
	}
</style>
