<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { createQuery } from '$lib/data/query.svelte';
	import { health } from '$lib/api/admin/health';
	import { findings } from '$lib/api/admin/findings';
	import { cluster } from '$lib/api/admin/cluster';
	import { status } from '$lib/api/admin/status';
	import { getConfig } from '$lib/api/admin/config';
	import { dataOf, series } from '$lib/api/admin/telemetry';
	import { usage } from '$lib/api/search';
	import type { Result, SeriesResult } from '$lib/api/types';
	import { formatTimestamp } from '$lib/format/time';
	import { hasCopy, t } from '$lib/copy/t';
	import type { StatusKey } from '$lib/status/catalog';
	import type { Series, UiError } from '$lib/ui/types';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import TimeSeriesChart from '$lib/ui/organisms/chart/TimeSeriesChart.svelte';
	import BytesCell from '$lib/ui/cells/BytesCell.svelte';
	import CodeCell from '$lib/ui/cells/CodeCell.svelte';
	import NumberCell from '$lib/ui/cells/NumberCell.svelte';
	import StorageCell from '$lib/ui/cells/StorageCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		class?: string;
	}

	let { class: className, ...rest }: Props = $props();

	const DAY_S = 86400;
	const SLOW_MS = 500;
	const generatedAt = new Date();

	const nowSeconds = (): number => Math.floor(Date.now() / 1000);
	const windowStart = (span: number): number => nowSeconds() - span;
	const chartWindow: readonly [number, number] = [windowStart(DAY_S), nowSeconds()];

	const valueOrReject = <T, E>(outcome: Result<T, E>): Promise<T> =>
		outcome.ok ? Promise.resolve(outcome.value) : Promise.reject(outcome.error);

	const healthQuery = createQuery({
		key: ['report', 'health'],
		fetch: async () => valueOrReject(await health(false)),
		refetchMs: 10000
	});
	const findingsQuery = createQuery({
		key: ['report', 'findings'],
		fetch: async () => valueOrReject(await findings(undefined, false)),
		refetchMs: 30000
	});
	const clusterQuery = createQuery({
		key: ['report', 'cluster'],
		fetch: async () => valueOrReject(await cluster()),
		refetchMs: 5000
	});
	const statusQuery = createQuery({
		key: ['report', 'status'],
		fetch: async () => valueOrReject(await status())
	});
	const usageQuery = createQuery({
		key: ['report', 'usage'],
		fetch: async () => valueOrReject(dataOf(await usage('/', { depth: 0 })))
	});
	const configQuery = createQuery({
		key: ['report', 'config'],
		fetch: async () => valueOrReject(await getConfig())
	});
	const requestsQuery = createQuery({
		key: ['report', 'requests'],
		fetch: async () =>
			valueOrReject(
				await series({
					name: 'callfs_http_requests_total',
					measure: 'rate',
					by: ['status_code'],
					from: chartWindow[0],
					to: chartWindow[1],
					stepS: 300
				})
			)
	});
	const latencyQuery = createQuery({
		key: ['report', 'latency'],
		fetch: async () =>
			valueOrReject(
				await series({
					name: 'callfs_http_request_duration_seconds',
					measure: 'p95',
					from: chartWindow[0],
					to: chartWindow[1],
					stepS: 300
				})
			)
	});

	function print(): void {
		window.print();
	}

	function statusOf(breached: boolean): 'health.ok' | 'health.warn' {
		return breached ? 'health.warn' : 'health.ok';
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

	function findingStatus(severity: string): StatusKey {
		if ('critical' === severity) return 'finding.critical';
		if ('warning' === severity) return 'finding.warning';
		return 'finding.info';
	}

	function memberStatus(state: string): StatusKey {
		if ('leader' === state) return 'member.leader';
		if ('follower' === state) return 'member.follower';
		return 'member.unreachable';
	}

	function valueText(value: unknown, secret: boolean, isSet: boolean): string {
		if (secret) return isSet ? t('u065cells.set') : t('u065cells.not_set');
		if (null === value || undefined === value || '' === value) return t('u065cells.not_set');
		if ('boolean' === typeof value) return value ? t('u065cells.yes') : t('u065cells.no');
		if ('object' === typeof value) return JSON.stringify(value);
		return String(value);
	}

	function latest(points: [number, number | null][]): number | null {
		for (let index = points.length - 1; 0 <= index; index -= 1) {
			const point = points[index];
			if (point && 'number' === typeof point[1]) return point[1];
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

	function toSeries(
		result: SeriesResult | undefined,
		keep: (code: string) => boolean,
		fallbackLabel: string,
		unit: Series['unit'],
		scale: number
	): Series[] {
		if (undefined === result) return [];
		return result.series
			.filter((entry) => keep(entry.labels.status_code ?? ''))
			.map((entry, index) => ({
				id: `${entry.labels.status_code ?? fallbackLabel}-${index}`,
				label: entry.labels.status_code ?? fallbackLabel,
				unit,
				colorIndex: index,
				points: entry.points.map(([timestamp, value]): [number, number | null] => [
					timestamp,
					null === value ? null : value * scale
				])
			}));
	}

	const openFindings = $derived((findingsQuery.data?.items ?? []).filter((finding) => null === finding.muted));
	const attention = $derived(
		openFindings.filter((finding) => 'critical' === finding.severity || 'warning' === finding.severity).length
	);
	const problems = $derived(
		openFindings.filter((finding) => 'critical' === finding.severity || 'warning' === finding.severity)
	);
	const notes = $derived(openFindings.filter((finding) => 'critical' !== finding.severity && 'warning' !== finding.severity));
	const healthyVoters = $derived((clusterQuery.data?.members ?? []).filter((member) => member.online).length);
	const verdict = $derived(verdictOf(healthQuery.data?.status, clusterQuery.data?.has_quorum, attention));
	const verdictLine = $derived(verdictText(verdict, attention));
	const downComponent = $derived(
		healthQuery.data?.components.find((component) => 'down' === component.status)?.name
	);
	const serverName = $derived(statusQuery.data?.instance_id ?? t('wall-report.report.unknown_server'));
	const changedSettings = $derived((configQuery.data?.keys ?? []).filter((setting) => 'default' !== setting.source));
	const storageRows = $derived(Object.entries(usageQuery.data?.by_storage ?? {}));
	const requestsPerSecond = $derived(totalOf(requestsQuery.data, () => true));
	const p95Ms = $derived.by(() => {
		const value = latest(latencyQuery.data?.series.at(0)?.points ?? []);
		return null === value ? null : value * 1000;
	});
	const requestSeries = $derived(toSeries(requestsQuery.data, () => true, t('wall-report.report.chart.requests'), 'per_s', 1));
	const errorSeries = $derived(
		toSeries(requestsQuery.data, isServerError, t('wall-report.report.chart.errors'), 'per_s', 1)
	);
	const latencySeries = $derived(
		toSeries(latencyQuery.data, () => true, t('wall-report.report.chart.p95'), 'ms', 1000)
	);
	const slowThreshold = $derived([
		{ value: SLOW_MS, tone: 'warn' as const, label: t('wall-report.report.threshold.p95') }
	]);
	const memberRows = $derived(clusterQuery.data?.members ?? []);
	const kpiTiles = $derived([
		{ label: t('wall-report.kpi.requests'), value: requestsPerSecond, unit: 'per_s' as const },
		{ label: t('wall-report.kpi.p95'), value: p95Ms, unit: 'ms' as const, status: statusOf(null !== p95Ms && SLOW_MS < p95Ms) },
		{ label: t('wall-report.kpi.bytes'), value: usageQuery.data?.bytes ?? null, unit: 'bytes' as const },
		{
			label: t('wall-report.kpi.members'),
			value: undefined !== clusterQuery.data ? healthyVoters : null,
			unit: 'count' as const,
			status: statusOf(false === clusterQuery.data?.has_quorum)
		}
	]);
</script>

{#snippet fallback(error: UiError | null)}
	{#if error}
		<ErrorState {error} variant="region" />
	{:else}
		<Skeleton shape="text" lines={3} />
	{/if}
{/snippet}

{#snippet findingList(list: typeof openFindings)}
	<Stack as="ul" gap="4">
		{#each list as finding (finding.id)}
			<li>
				<Stack gap="1">
					<Cluster gap="3" align="center">
						<StatusPill status={findingStatus(finding.severity)} />
						<Text weight={600}>{finding.title}</Text>
					</Cluster>
					<Text tone="muted">{finding.body}</Text>
					<Text>{t('wall-report.report.fix', { label: finding.fix.label })}</Text>
				</Stack>
			</li>
		{/each}
	</Stack>
{/snippet}

<article class={['report', className]} aria-busy={undefined === healthQuery.data} {...rest}>
	<Cluster gap="4" align="center" class="actions">
		<Button variant="primary" onclick={print}>{t('wall-report.report.print')}</Button>
	</Cluster>

	<Stack gap="2">
		<Heading level={1}>{t('wall-report.report.title')}</Heading>
		<Cluster gap="3" align="center">
			<StatusPill status={verdict} />
			<Text size="title-sm" weight={600}>{verdictLine}</Text>
		</Cluster>
		{#if downComponent}
			<Text tone="danger">{t('wall-report.cause.down', { component: downComponent })}</Text>
		{/if}
		{#if 'health.ok' === verdict && 0 < notes.length}
			<Text tone="muted">{t('wall-report.verdict.notes', { count: notes.length })}</Text>
		{/if}
		<Text tone="muted">
			{t('wall-report.report.generated', { date: formatTimestamp(generatedAt), server: serverName })}
		</Text>
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.findings')}</Heading>
		{#if undefined === findingsQuery.data}
			{@render fallback(findingsQuery.error)}
		{:else if 0 === openFindings.length}
			<Text tone="muted">{t('wall-report.report.no_findings')}</Text>
		{:else}
			{#if 0 === problems.length}
				<Text tone="muted">{t('wall-report.report.no_problems')}</Text>
			{:else}
				{@render findingList(problems)}
			{/if}
			{#if 0 < notes.length}
				<Heading level={3}>{t('wall-report.report.notes')}</Heading>
				{@render findingList(notes)}
			{/if}
		{/if}
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.kpis')}</Heading>
		<KpiStrip tiles={kpiTiles} />
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.members')}</Heading>
		{#if undefined === clusterQuery.data}
			{@render fallback(clusterQuery.error)}
		{:else}
			<table aria-label={t('wall-report.report.members_table')}>
				<thead>
					<tr>
						<th scope="col">{t('wall-report.report.member_name')}</th>
						<th scope="col">{t('wall-report.report.member_state')}</th>
						<th scope="col">{t('wall-report.report.member_behind')}</th>
					</tr>
				</thead>
				<tbody>
					{#each memberRows as member (member.node_id)}
						<tr>
							<td><TextCell value={member.node_id} mono /></td>
							<td><StatusPill status={memberStatus(member.state)} /></td>
							<td><NumberCell value={member.behind_by} /></td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.storage')}</Heading>
		{#if undefined === usageQuery.data}
			{@render fallback(usageQuery.error)}
		{:else}
			<table aria-label={t('wall-report.report.storage_table')}>
				<thead>
					<tr>
						<th scope="col">{t('wall-report.report.storage_place')}</th>
						<th scope="col">{t('wall-report.report.storage_files')}</th>
						<th scope="col">{t('wall-report.report.storage_bytes')}</th>
					</tr>
				</thead>
				<tbody>
					{#each storageRows as [place, figures] (place)}
						<tr>
							<td><StorageCell value={place} /></td>
							<td><NumberCell value={figures.files} /></td>
							<td><BytesCell value={figures.bytes} /></td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.charts')}</Heading>
		<Heading level={3}>{t('wall-report.report.chart.requests_title')}</Heading>
		<TimeSeriesChart
			domain={chartWindow}
			series={requestSeries}
			unit="per_s"
			status={requestsQuery.status}
			error={requestsQuery.error ?? undefined}
			height="sm"
			stacked
		/>
		<Heading level={3}>{t('wall-report.report.chart.errors_title')}</Heading>
		<TimeSeriesChart
			domain={chartWindow}
			series={errorSeries}
			unit="per_s"
			status={requestsQuery.status}
			error={requestsQuery.error ?? undefined}
			height="sm"
		/>
		<Heading level={3}>{t('wall-report.report.chart.p95_title')}</Heading>
		<TimeSeriesChart
			domain={chartWindow}
			series={latencySeries}
			unit="ms"
			thresholds={slowThreshold}
			status={latencyQuery.status}
			error={latencyQuery.error ?? undefined}
			height="sm"
		/>
	</Stack>

	<Stack gap="4">
		<Heading level={2}>{t('wall-report.report.settings')}</Heading>
		{#if undefined === configQuery.data}
			{@render fallback(configQuery.error)}
		{:else if 0 === changedSettings.length}
			<Text tone="muted">{t('wall-report.report.settings_none')}</Text>
		{:else}
			<table aria-label={t('wall-report.report.settings_table')}>
				<thead>
					<tr>
						<th scope="col">{t('wall-report.report.setting_key')}</th>
						<th scope="col">{t('wall-report.report.setting_value')}</th>
					</tr>
				</thead>
				<tbody>
					{#each changedSettings as setting (setting.path)}
						{@const labelKey = `settings.label.${setting.path}`}
						<tr>
							<td>
								{#if !hasCopy(labelKey)}
									<CodeCell value={setting.path} />
								{:else}
									<span class="setting-label">{t(labelKey)}</span>
									<span class="setting-key">{setting.path}</span>
								{/if}
							</td>
							<td class="value">{valueText(setting.value, setting.secret, setting.set)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</Stack>
</article>

<style>
	.report {
		display: flex;
		flex-direction: column;
		gap: var(--space-8);
		padding: var(--space-6);
	}

	.report :global(.actions) {
		justify-content: flex-end;
	}

	table {
		border-collapse: collapse;
		inline-size: 100%;
	}

	th,
	td {
		padding-block: var(--space-2);
		padding-inline: var(--cell-px);
		text-align: start;
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.setting-label,
	.setting-key {
		display: block;
	}

	.setting-key {
		font-family: var(--font-mono);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		color: var(--text-muted);
		overflow-wrap: anywhere;
	}

	.value {
		font-family: var(--font-mono);
		overflow-wrap: anywhere;
		white-space: pre-wrap;
	}

	@media print {
		.report :global(.actions) {
			display: none;
		}
	}
</style>
