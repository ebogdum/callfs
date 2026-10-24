<script lang="ts">
	import { base } from '$app/paths';
	import { replication, retryReplication, type JobAccepted } from '$lib/api/admin/topology';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery, type Query } from '$lib/data/query.svelte';
	import type { Replication, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import type { StatusKey } from '$lib/status/catalog';
	import type { ColumnDef, UiError } from '$lib/ui/types';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList from './FactList.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import PathCell from '$lib/ui/cells/PathCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';

	type Failure = Replication['failures'][number];
	type Shown<T> = { value: T | null; error: UiError | null };
	type Feedback = { tone: 'ok' | 'danger'; title: string; body?: string };
	type BackupStatus = Extract<StatusKey, 'backup.on_strict' | 'backup.on_best_effort' | 'backup.off'>;

	const LOAD_ACTION = 'backup.action.load';
	const RETRY_ACTION = 'backup.action.retry';
	const EXPLANATION_KEYS = {
		'backup.on_strict': 'backup.mode.strict',
		'backup.on_best_effort': 'backup.mode.best_effort',
		'backup.off': 'backup.mode.off'
	} as const;

	const NOTHING_SHOWN: Shown<Replication> = { value: null, error: null };

	const replicationQuery: Query<Shown<Replication>> = createQuery<Shown<Replication>>({
		key: ['replication'],
		fetch: async () => settle(replicationQuery.data ?? NOTHING_SHOWN, await replication(), LOAD_ACTION)
	});

	const backup = $derived(replicationQuery.data ?? NOTHING_SHOWN);
	let feedback = $state<Feedback | null>(null);

	const report = $derived(backup.value);
	const shownError = $derived(backup.error);
	const failures = $derived(report?.failures ?? []);
	const SETTINGS_HREF = `${base}/settings/storage`;
	const modeLabel = $derived(
		null === report || false === report.enabled
			? t('cluster.backup.mode.off')
			: report.require_success
				? t('cluster.backup.mode.strict')
				: t('cluster.backup.mode.best_effort')
	);
	const tiles = $derived(
		null === report
			? []
			: [
					{
						label: t('cluster.backup.kpi.made'),
						value: report.counts_24h.ok,
						unit: 'count' as const,
						base: { num: report.counts_24h.ok, den: report.counts_24h.ok + report.counts_24h.failed }
					},
					{
						label: t('cluster.backup.kpi.failed'),
						value: report.counts_24h.failed,
						unit: 'count' as const,
						status: 0 < report.counts_24h.failed ? ('health.warn' as const) : ('health.ok' as const)
					},
					{ label: t('cluster.backup.kpi.waiting'), value: failures.length, unit: 'count' as const }
				]
	);

	const retry = createMutation<string[] | null, Result<JobAccepted, ApiFailure>>({
		run: (paths) => retryReplication(paths),
		invalidate: () => [['replication']],
		context: { action: RETRY_ACTION }
	});

	function settle<T>(previous: Shown<T>, outcome: Result<T, ApiFailure>, action: string): Shown<T> {
		if (outcome.ok) {
			return { value: outcome.value, error: null };
		}
		return { value: previous.value, error: toUiError(outcome.error, { action, read: true }) };
	}

	function statusOf(replicaReport: Replication): BackupStatus {
		if (false === replicaReport.enabled) {
			return 'backup.off';
		}
		return replicaReport.require_success ? 'backup.on_strict' : 'backup.on_best_effort';
	}

	function storageLabel(storage: string): string {
		if ('localfs' === storage) {
			return t('backup.storage.local');
		}
		if ('s3' === storage) {
			return t('backup.storage.s3');
		}
		return t('backup.storage.other');
	}

	async function startRetry(paths: string[] | null): Promise<void> {
		const outcome = await retry.run(paths);
		if (null === outcome) {
			return;
		}
		if (outcome.ok) {
			feedback = { tone: 'ok', title: t('backup.retry.started') };
			return;
		}
		const error = toUiError(outcome.error, { action: RETRY_ACTION });
		feedback = { tone: 'danger', title: error.title, body: error.body };
	}

	const columns: ColumnDef<Failure>[] = [
		{ id: 'path', header: t('backup.failures.path'), cell: pathCell, minWidth: 200 },
		{ id: 'error', header: t('backup.failures.error'), cell: errorCell, minWidth: 160 },
		{ id: 'when', header: t('backup.failures.when'), cell: whenCell },
		{ id: 'retry', header: t('backup.failures.retry'), cell: retryCell, align: 'end' }
	];
</script>

{#snippet pathCell(row: Failure)}
	<PathCell path={row.path} />
{/snippet}

{#snippet errorCell(row: Failure)}
	<TextCell value={row.error} />
{/snippet}

{#snippet whenCell(row: Failure)}
	<TimeCell value={row.at} />
{/snippet}

{#snippet retryCell(row: Failure)}
	<Button variant="ghost" size="sm" loading={retry.pending} onclick={() => startRetry([row.path])}>
		{t('backup.failures.retry')}
	</Button>
{/snippet}

{#snippet loadingBody()}
	<Stack gap="4">
		<Skeleton shape="rect" height="var(--space-12)" />
		<Skeleton shape="text" lines={3} />
	</Stack>
{/snippet}

<PageFrame title={t('backup.title')} job={t('backup.job')}>
	{#snippet primary()}
		{#if report?.enabled && 0 < failures.length}
			<Button variant="primary" iconStart="refresh" loading={retry.pending} onclick={() => startRetry(null)}>
				{t('backup.retry.label')}
			</Button>
		{/if}
	{/snippet}
	<Stack gap="8">
		{#if feedback}
			<InlineAlert tone={feedback.tone} title={feedback.title} body={feedback.body} />
		{/if}
		{#if null === report}
			{#if shownError}
				<ErrorState error={shownError} onretry={() => replicationQuery.refetch()} variant="region" />
			{:else}
				{@render loadingBody()}
			{/if}
		{:else}
			{#if shownError}
				<InlineAlert tone="danger" title={shownError.title} body={shownError.body} />
			{/if}
			{@const status = statusOf(report)}
			{#if false === report.enabled}
				<EmptyState
					variant="not-available"
					icon="backup"
					title={t('backup.off.title')}
					body={t('backup.off.body')}
					primary={{ label: t('cluster.backup.open_settings'), href: SETTINGS_HREF }}
				/>
			{:else}
				<KpiStrip {tiles} />
				<ClusterCard
					title={t('cluster.backup.how.title')}
					description={t(EXPLANATION_KEYS[status])}
					icon="backup"
					tone={0 < report.counts_24h.failed ? 'warn' : 'ok'}
				>
					{#snippet meta()}
						<StatusPill {status} size="sm" />
					{/snippet}
					<FactList
						items={[
							{ term: t('cluster.backup.fact.mode'), value: modeLabel },
							{ term: t('cluster.backup.fact.target'), value: storageLabel(report.replica_storage) }
						]}
					/>
					{#snippet footer()}
						<Link href={SETTINGS_HREF}>{t('cluster.backup.open_settings')}</Link>
					{/snippet}
				</ClusterCard>
				<section class="region" aria-labelledby="backup-failures">
					<Heading level={2} size="title-sm" id="backup-failures">{t('backup.failures.title')}</Heading>
					<DataTable
						rows={report.failures}
						{columns}
						rowKey={(row: Failure) => `${row.path}|${row.at}`}
						label={t('backup.failures.label')}
						status={0 < report.failures.length ? 'success' : 'empty'}
					>
						{#snippet empty()}
							<EmptyState
								variant="cleared"
								icon="backup"
								title={t('backup.failures.empty.title')}
								body={t('backup.failures.empty.body')}
							/>
						{/snippet}
					</DataTable>
				</section>
			{/if}
		{/if}
	</Stack>
</PageFrame>

<style>
	.region {
		display: grid;
		gap: var(--space-3);
	}
</style>
