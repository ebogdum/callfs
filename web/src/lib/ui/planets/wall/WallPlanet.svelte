<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { createQuery } from '$lib/data/query.svelte';
	import { health } from '$lib/api/admin/health';
	import { findings } from '$lib/api/admin/findings';
	import { cluster } from '$lib/api/admin/cluster';
	import { dataOf, series } from '$lib/api/admin/telemetry';
	import { usage } from '$lib/api/search';
	import type { Result, SeriesResult } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import type { StatusKey } from '$lib/status/catalog';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import IconButton from '$lib/ui/atoms/IconButton.svelte';
	import RelTime from '$lib/ui/atoms/RelTime.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import ToggleButton from '$lib/ui/atoms/ToggleButton.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import QuorumMeter from '$lib/ui/molecules/QuorumMeter.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		class?: string;
	}

	let { class: className, ...rest }: Props = $props();

	const ROTATE_MS = 20000;
	const TICK_MS = 1000;
	const STALE_MS = 30000;
	const HOUR_S = 3600;
	const DAY_S = 86400;
	const SLOW_MS = 500;
	const ERROR_SHARE = 0.01;
	const boards = ['health', 'traffic', 'storage'] as const;

	const nowSeconds = (): number => Math.floor(Date.now() / 1000);
	const windowStart = (span: number): number => nowSeconds() - span;

	function valueOrReject<T, E>(outcome: Result<T, E>): Promise<T> {
		return outcome.ok ? Promise.resolve(outcome.value) : Promise.reject(outcome.error);
	}

	const healthQuery = createQuery({
		key: ['wall', 'health'],
		fetch: async () => valueOrReject(await health(false)),
		refetchMs: 10000
	});
	const findingsQuery = createQuery({
		key: ['wall', 'findings'],
		fetch: async () => valueOrReject(await findings(undefined, false)),
		refetchMs: 30000
	});
	const clusterQuery = createQuery({
		key: ['wall', 'cluster'],
		fetch: async () => valueOrReject(await cluster()),
		refetchMs: 5000
	});
	const requestsQuery = createQuery({
		key: ['wall', 'requests'],
		fetch: async () =>
			valueOrReject(
				await series({
					name: 'callfs_http_requests_total',
					measure: 'rate',
					by: ['status_code'],
					from: windowStart(HOUR_S),
					to: nowSeconds(),
					stepS: 60
				})
			),
		refetchMs: 10000
	});
	const latencyQuery = createQuery({
		key: ['wall', 'latency'],
		fetch: async () =>
			valueOrReject(
				await series({
					name: 'callfs_http_request_duration_seconds',
					measure: 'p95',
					from: windowStart(HOUR_S),
					to: nowSeconds(),
					stepS: 60
				})
			),
		refetchMs: 10000
	});
	const usageQuery = createQuery({
		key: ['wall', 'usage'],
		fetch: async () => valueOrReject(dataOf(await usage('/', { depth: 0 }))),
		refetchMs: 60000
	});
	const growthQuery = createQuery({
		key: ['wall', 'growth'],
		fetch: async () =>
			valueOrReject(
				await series({
					name: 'usage_bytes',
					measure: 'value',
					from: windowStart(DAY_S),
					to: nowSeconds(),
					stepS: 3600
				})
			),
		refetchMs: 60000
	});

	let now = $state(Date.now());
	let boardIndex = $state(0);
	let paused = $state(false);
	const board = $derived(boards[boardIndex] ?? 'health');
	const position = $derived(t('wall-report.wall.position', { current: boardIndex + 1, total: boards.length }));
	const boardTitle = $derived(t(`wall-report.board.${board}`));

	$effect(() => {
		const ticker = setInterval(() => {
			now = Date.now();
		}, TICK_MS);
		return () => {
			clearInterval(ticker);
		};
	});

	$effect(() => {
		if (paused) return;
		const rotation = setInterval(() => {
			boardIndex = (boardIndex + 1) % boards.length;
		}, ROTATE_MS);
		return () => {
			clearInterval(rotation);
		};
	});

	function step(direction: 1 | -1): void {
		boardIndex = (boardIndex + direction + boards.length) % boards.length;
	}

	function onKey(event: KeyboardEvent): void {
		if ('ArrowRight' === event.key) step(1);
		if ('ArrowLeft' === event.key) step(-1);
	}

	const primary = $derived.by(() => {
		if ('health' === board) return healthQuery;
		if ('traffic' === board) return requestsQuery;
		return usageQuery;
	});
	const boardReady = $derived(undefined !== primary.data);
	const updatedAt = $derived(primary.updatedAt);
	const staleSeconds = $derived(
		null === updatedAt || STALE_MS >= now - updatedAt ? null : Math.round((now - updatedAt) / 1000)
	);

	function retry(): void {
		void primary.refetch();
	}

	function statusOf(breached: boolean): 'health.ok' | 'health.warn' {
		return breached ? 'health.warn' : 'health.ok';
	}

	function toneOf(key: StatusKey): 'ok' | 'warn' | 'danger' {
		if ('health.down' === key) return 'danger';
		if ('health.warn' === key) return 'warn';
		return 'ok';
	}

	function verdictOf(healthStatus: string | undefined, hasQuorum: boolean | undefined, attentionCount: number): StatusKey {
		if ('down' === healthStatus || false === hasQuorum) return 'health.down';
		if (0 < attentionCount) return 'health.warn';
		return 'health.ok';
	}

	function verdictText(key: StatusKey, attentionCount: number): string {
		if ('health.down' === key) return t('wall-report.verdict.down');
		if ('health.warn' === key) return t('wall-report.verdict.warn', { count: attentionCount });
		return t('wall-report.verdict.ok');
	}

	const openFindings = $derived((findingsQuery.data?.items ?? []).filter((finding) => null === finding.muted));
	const attention = $derived(
		openFindings.filter((finding) => 'critical' === finding.severity || 'warning' === finding.severity).length
	);
	const healthyVoters = $derived(
		(clusterQuery.data?.members ?? []).filter(
			(member) => 'voter' === member.suffrage && 'unreachable' !== member.state && 'shutdown' !== member.state
		).length
	);
	const verdict = $derived(verdictOf(healthQuery.data?.status, clusterQuery.data?.has_quorum, attention));
	const verdictLine = $derived(verdictText(verdict, attention));
	const downComponent = $derived(
		healthQuery.data?.components.find((component) => 'down' === component.status)?.name
	);

	function latest(points: [number, number | null][]): number | null {
		for (let index = points.length - 1; 0 <= index; index -= 1) {
			const point = points[index];
			if (point && 'number' === typeof point[1]) return point[1];
		}
		return null;
	}

	function earliest(points: [number, number | null][]): number | null {
		for (const point of points) {
			if ('number' === typeof point[1]) return point[1];
		}
		return null;
	}

	function totalOf(result: SeriesResult | undefined, keep: (code: string) => boolean): number | null {
		if (undefined === result) return null;
		let total: number | null = null;
		for (const entry of result.series) {
			const value = latest(entry.points);
			if (null !== value && keep(entry.labels.status_code ?? '')) total = (total ?? 0) + value;
		}
		return total;
	}

	function isServerError(code: string): boolean {
		return '5' === code.charAt(0);
	}

	function growthOf(result: SeriesResult | undefined): number | null {
		const points = result?.series.at(0)?.points;
		if (undefined === points) return null;
		const start = earliest(points);
		const end = latest(points);
		return null === start || null === end ? null : end - start;
	}

	const requestsPerSecond = $derived(totalOf(requestsQuery.data, () => true));
	const serverErrorsPerSecond = $derived(totalOf(requestsQuery.data, isServerError));
	const p95Ms = $derived.by(() => {
		const value = latest(latencyQuery.data?.series.at(0)?.points ?? []);
		return null === value ? null : value * 1000;
	});
	const errorsBreached = $derived(
		null !== serverErrorsPerSecond &&
			null !== requestsPerSecond &&
			0 < requestsPerSecond &&
			ERROR_SHARE < serverErrorsPerSecond / requestsPerSecond
	);
	const slowBreached = $derived(null !== p95Ms && SLOW_MS < p95Ms);
	const growthBytes = $derived(growthOf(growthQuery.data));

	const healthTiles = $derived([
		{
			label: t('wall-report.kpi.findings'),
			value: openFindings.length,
			unit: 'count' as const,
			status: statusOf(0 < attention)
		},
		{
			label: t('wall-report.kpi.members'),
			value: undefined !== clusterQuery.data ? healthyVoters : null,
			unit: 'count' as const,
			status: statusOf(false === clusterQuery.data?.has_quorum)
		}
	]);
	const trafficTiles = $derived([
		{ label: t('wall-report.kpi.requests'), value: requestsPerSecond, unit: 'per_s' as const },
		{
			label: t('wall-report.kpi.errors'),
			value: serverErrorsPerSecond,
			unit: 'per_s' as const,
			status: statusOf(errorsBreached)
		},
		{
			label: t('wall-report.kpi.p95'),
			value: p95Ms,
			unit: 'ms' as const,
			status: statusOf(slowBreached)
		}
	]);
	const storageTiles = $derived([
		{ label: t('wall-report.kpi.bytes'), value: usageQuery.data?.bytes ?? null, unit: 'bytes' as const },
		{ label: t('wall-report.kpi.files'), value: usageQuery.data?.files ?? null, unit: 'count' as const },
		{ label: t('wall-report.kpi.split'), value: usageQuery.data?.erasure_files ?? null, unit: 'count' as const },
		{ label: t('wall-report.kpi.growth'), value: growthBytes, unit: 'bytes' as const }
	]);
</script>

<svelte:window onkeydown={onKey} />

<section
	class={['wall', className]}
	aria-label={t('wall-report.wall.label')}
	aria-busy={!boardReady}
	data-state={null === staleSeconds ? 'live' : 'stale'}
	{...rest}
>
	<Cluster gap="4" align="center">
		<IconButton icon="chevron-left" label={t('wall-report.wall.previous')} onclick={() => step(-1)} />
		<Text tone="muted">{position}</Text>
		<IconButton icon="chevron-right" label={t('wall-report.wall.next')} onclick={() => step(1)} />
		<ToggleButton bind:pressed={paused} label={t('wall-report.wall.pause')} />
	</Cluster>

	<Cluster gap="4" align="center">
		<Heading level={1}>{boardTitle}</Heading>
		{#if 'health' === board}
			<StatusPill status={verdict} />
		{/if}
		{#if null !== updatedAt}
			<Text tone="muted">{t('wall-report.wall.updated')}</Text>
			<RelTime at={updatedAt} />
		{/if}
		{#if null !== staleSeconds}
			<Text tone="warn" weight={600}>{t('wall-report.wall.stale', { seconds: staleSeconds })}</Text>
		{/if}
	</Cluster>

	{#if !boardReady && null !== primary.error}
		<ErrorState error={primary.error} variant="region" onretry={retry} />
	{:else if !boardReady}
		<Skeleton shape="text" lines={4} />
	{:else if 'health' === board}
		<Stack gap="6">
			<Text size="heading" weight={600} tone={toneOf(verdict)}>{verdictLine}</Text>
			{#if downComponent}
				<Text tone="danger">{t('wall-report.cause.down', { component: downComponent })}</Text>
			{/if}
			<KpiStrip tiles={healthTiles} />
			{#if undefined !== clusterQuery.data}
				<QuorumMeter voters={clusterQuery.data.voters} healthy={healthyVoters} />
			{/if}
		</Stack>
	{:else if 'traffic' === board}
		<KpiStrip tiles={trafficTiles} />
	{:else}
		<KpiStrip tiles={storageTiles} />
	{/if}
</section>

<style>
	.wall {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-block-size: 100%;
		padding: var(--space-6);
	}

	.wall[data-state='stale'] {
		border-block-start: var(--hairline) solid var(--warn);
	}
</style>
