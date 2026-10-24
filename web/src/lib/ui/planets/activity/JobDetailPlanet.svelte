<script lang="ts">
	import { getContext, type ComponentProps } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { cancel as cancelAdminJob, job as adminJob, retryFailed as retryAdminJob } from '$lib/api/admin/jobs';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { cancelJob, job as ownJob, retryJobFailed } from '$lib/api/session';
	import type { Job, Result } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatDuration } from '$lib/format/duration';
	import { formatNumber } from '$lib/format/number';
	import { session } from '$lib/stores/session.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { ColumnDef, QueryStatus, StatusKey, UiError } from '$lib/ui/types';
	import Button from '$lib/ui/atoms/Button.svelte';
	import CopyButton from '$lib/ui/atoms/CopyButton.svelte';
	import Duration from '$lib/ui/atoms/Duration.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import ProgressBar from '$lib/ui/atoms/ProgressBar.svelte';
	import Spinner from '$lib/ui/quarks/Spinner.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import PathCell from '$lib/ui/cells/PathCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ProgressSteps from '$lib/ui/molecules/ProgressSteps.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	type FailedRow = Job['failed_items'][number] & { key: string };
	type Step = ComponentProps<typeof ProgressSteps>['steps'][number];
	type Phase = 'copy' | 'verify' | 'delete';

	const COMMANDS_CONTEXT = 'callfs.commands';
	const ACTIVE_STATES: readonly Job['state'][] = ['queued', 'running', 'cancelling'];
	const POLL_MS = 2000;
	const PHASES: readonly Phase[] = ['copy', 'verify', 'delete'];

	const ACTION_OPEN = 'jobs.action.open';
	const ACTION_CANCEL = 'jobs.action.cancel';
	const ACTION_RETRY = 'jobs.action.retry';

	const JOB_STATUS: Record<Job['state'], StatusKey> = {
		queued: 'job.queued',
		running: 'job.running',
		succeeded: 'job.succeeded',
		partial: 'job.partial',
		failed: 'job.failed',
		cancelled: 'job.cancelled',
		cancelling: 'job.cancelling'
	};

	const KIND_KEY: Record<Job['kind'], string> = {
		delete_prefix: 'jobs.kind.delete_prefix',
		delete_recursive: 'jobs.kind.delete_recursive',
		batch: 'jobs.kind.batch',
		erasure_repair: 'jobs.kind.erasure_repair',
		erasure_verify_all: 'jobs.kind.erasure_verify_all',
		replication_retry: 'jobs.kind.replication_retry',
		owner_reassign: 'jobs.kind.owner_reassign',
		staging_cleanup: 'jobs.kind.staging_cleanup',
		usage_scan: 'jobs.kind.usage_scan',
		trash_empty: 'jobs.kind.trash_empty',
		link_cleanup: 'jobs.kind.link_cleanup',
		retag: 'jobs.kind.retag'
	};

	const PHASE_KEY: Record<Phase, string> = {
		copy: 'jobs.phase.copy',
		verify: 'jobs.phase.verify',
		delete: 'jobs.phase.delete'
	};

	interface Props {
		id: string;
		class?: string;
	}

	let { id, class: className }: Props = $props();

	function fromAdmin<T>(result: Result<T, ApiFailure>, action: string): Result<T, UiError> {
		return true === result.ok ? result : { ok: false, error: toUiError(result.error, { action }) };
	}

	async function loadJob(jobId: string): Promise<Result<Job, UiError>> {
		if (session.isAdmin) {
			return fromAdmin(await adminJob(jobId), ACTION_OPEN);
		}
		return ownJob(jobId);
	}

	async function cancelById(jobId: string): Promise<Result<Job, UiError>> {
		if (session.isAdmin) {
			return fromAdmin(await cancelAdminJob(jobId), ACTION_CANCEL);
		}
		return cancelJob(jobId);
	}

	async function retryById(jobId: string): Promise<Result<Job, UiError>> {
		if (session.isAdmin) {
			return fromAdmin(await retryAdminJob(jobId), ACTION_RETRY);
		}
		return retryJobFailed(jobId);
	}

	const jobQuery = createQuery(() => ({
		key: ['job', id],
		fetch: () => loadJob(id),
		staleMs: POLL_MS,
		refetchMs: (data) => (true === data?.ok && ACTIVE_STATES.includes(data.value.state) ? POLL_MS : false),
		revalidateOnFocus: true
	}));

	$effect(() => jobQuery.attach());

	const jobResult = $derived(jobQuery.data);
	const job = $derived(true === jobResult?.ok ? jobResult.value : null);
	const loadError = $derived(false === jobResult?.ok ? jobResult.error : null);

	let retryPending = $state(false);
	let cancelOpen = $state(false);
	let actionError = $state<UiError | null>(null);

	function isActive(target: Job): boolean {
		return ACTIVE_STATES.includes(target.state);
	}

	function isDeleteJob(target: Job | null): boolean {
		return null !== target && ('delete_prefix' === target.kind || 'delete_recursive' === target.kind);
	}

	function targetOf(target: Job): string | null {
		const path = target.result.path ?? target.result.prefix;
		return 'string' === typeof path ? path : null;
	}

	function percentOf(target: Job): number {
		const { done, total } = target.progress;
		if (null === total || 0 === total) {
			return 0;
		}
		return Math.round((done * 100) / total);
	}

	function countsOf(target: Job): string {
		const done = formatNumber(target.progress.done);
		if (null === target.progress.total) {
			return t('jobs.counts.unknown', { done });
		}
		const total = formatNumber(target.progress.total);
		if (0 === target.failed_count) {
			return t('jobs.counts.clean', { done, total });
		}
		return t('jobs.counts', { done, total, failed: formatNumber(target.failed_count) });
	}

	function etaOf(target: Job): string {
		const { done, total, per_second } = target.progress;
		if (null === total || 0 >= per_second || 'running' !== target.state) {
			return t('jobs.eta.unknown');
		}
		return t('jobs.eta', { time: formatDuration(((total - done) / per_second) * 1000, 'long') });
	}

	function stepStatus(target: Job, index: number, current: number): StatusKey {
		if ('succeeded' === target.state || index < current) {
			return 'job.succeeded';
		}
		if (index > current) {
			return 'job.queued';
		}
		return JOB_STATUS[target.state];
	}

	function stepsOf(target: Job): Step[] {
		const current = PHASES.findIndex((phase) => phase === target.phase);
		return PHASES.map((phase, index) => ({
			id: phase,
			label: t(PHASE_KEY[phase]),
			state: stepStatus(target, index, current)
		}));
	}

	function reportOf(target: Job): string {
		return JSON.stringify(target, null, 2);
	}

	const pageTitle = $derived(null === job ? t('activity.jobs.detailTitle') : t(KIND_KEY[job.kind]));
	const steps = $derived(null === job ? [] : stepsOf(job));
	const failedRows = $derived(
		null === job ? [] : job.failed_items.map((item, index): FailedRow => ({ ...item, key: `${index}` }))
	);
	const isTruncated = $derived(null !== job && job.failed_count > job.failed_items.length);
	const failedStatus = $derived.by((): QueryStatus => {
		if (null === job || 0 === job.failed_items.length) {
			return 'empty';
		}
		return isTruncated ? 'partial' : 'success';
	});
	const showCancel = $derived(null !== job && isActive(job));
	const cancelReason = $derived(null !== job && 'cancelling' === job.state ? t('jobs.cancel.stopping') : undefined);
	const canRetry = $derived(null !== job && 0 < job.failed_count && !isActive(job));
	const reportText = $derived(null === job ? '' : reportOf(job));
	const cancelTitle = $derived(t(isDeleteJob(job) ? 'jobs.cancel.delete.title' : 'jobs.cancel.generic.title'));
	const cancelBody = $derived(t(isDeleteJob(job) ? 'jobs.cancel.delete.body' : 'jobs.cancel.generic.body'));

	const failedColumns: ColumnDef<FailedRow>[] = [
		{ id: 'path', header: t('jobs.failed.col.path'), cell: failedPathCell },
		{ id: 'reason', header: t('jobs.failed.col.reason'), cell: failedReasonCell },
		{ id: 'code', header: t('jobs.failed.col.code'), cell: failedCodeCell }
	];

	const failedKey = (row: FailedRow): string => row.key;

	async function retry(): Promise<void> {
		const current = job;
		if (null === current) {
			return;
		}
		retryPending = true;
		const result = await retryById(current.id);
		retryPending = false;
		if (false === result.ok) {
			actionError = result.error;
			return;
		}
		actionError = null;
		invalidate(['job', current.id]);
		invalidate(['jobs']);
		if (result.value.id !== current.id) {
			goto(`${base}/activity/jobs/${encodeURIComponent(result.value.id)}`).catch(() =>
				toasts.push({ tone: 'danger', title: t('jobs.open_failed') })
			);
		}
	}

	async function confirmCancel(): Promise<void> {
		cancelOpen = false;
		if (null === job) {
			return;
		}
		const result = await cancelById(job.id);
		if (false === result.ok) {
			actionError = result.error;
			return;
		}
		actionError = null;
		invalidate(['job', job.id]);
		invalidate(['jobs']);
	}

	function copyRequestId(): void {
		navigator.clipboard
			.writeText(id)
			.catch(() => toasts.push({ tone: 'danger', title: t('jobs.command.copy_failed') }));
	}

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'activity.job.cancel',
				title: t('jobs.cancel'),
				group: 'Activity',
				keywords: [id, 'stop'],
				icon: 'stop',
				when: () => showCancel,
				run: () => {
					cancelOpen = true;
				}
			},
			{
				id: 'activity.job.retry',
				title: t('jobs.retry'),
				group: 'Activity',
				keywords: [id, 'fix', 'failed'],
				icon: 'refresh',
				when: () => canRetry,
				run: () => retry()
			},
			{
				id: 'activity.job.copy-request-id',
				title: t('jobs.command.copy_id'),
				group: 'Activity',
				keywords: [id],
				icon: 'copy',
				run: () => copyRequestId()
			}
		]);
	});
</script>

{#snippet failedPathCell(row: FailedRow)}
	<PathCell path={row.path} />
{/snippet}

{#snippet failedReasonCell(row: FailedRow)}
	<TextCell value={row.message} />
{/snippet}

{#snippet failedCodeCell(row: FailedRow)}
	<TextCell value={row.code} mono />
{/snippet}

<PageFrame title={pageTitle} class={className}>
	{#snippet breadcrumbs()}
		<Link href={`${base}/activity/jobs`}>{t('activity.jobs.back')}</Link>
	{/snippet}

	{#snippet scope()}
		{#if null !== job}
			<StatusPill status={JOB_STATUS[job.state]} />
		{/if}
	{/snippet}

	{#snippet primary()}
		{#if canRetry}
			<Button variant="primary" iconStart="refresh" loading={retryPending} onclick={() => void retry()}>
				{t('jobs.retry')}
			</Button>
		{/if}
	{/snippet}

	{#snippet secondary()}
		{#if null !== job}
			{#if showCancel}
				<Button
					variant="secondary"
					iconStart="stop"
					disabledReason={cancelReason}
					onclick={() => (cancelOpen = true)}
				>
					{t('jobs.cancel')}
				</Button>
			{/if}
			<CopyButton variant="button" value={reportText} label={t('jobs.copy_report')} />
		{/if}
	{/snippet}

	{#if null !== loadError && 'not_found' === loadError.layer}
		<EmptyState
			variant="not-available"
			icon="jobs"
			title={t('activity.jobs.gone.title')}
			body={t('activity.jobs.gone.body')}
			primary={{ label: t('activity.jobs.back'), href: `${base}/activity/jobs` }}
		/>
	{:else if null !== loadError}
		<ErrorState error={loadError} onretry={() => void jobQuery.refetch()} />
	{:else if null === job}
		<Spinner label={t('jobs.loading')} />
	{:else}
		<Stack gap="8">
			{#if null !== job.error}
				<InlineAlert tone="danger" title={t('jobs.error.title')} body={job.error} />
			{/if}

			{#if null !== actionError}
				<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
			{/if}

			<section class="card" aria-label={t('jobs.facts.progress')}>
				<div class="progress-head">
					<Text weight={600}>{countsOf(job)}</Text>
					<Text tone="muted" size="sm">{etaOf(job)}</Text>
				</div>
				<ProgressBar
					value={percentOf(job)}
					label={t('jobs.facts.progress')}
					indeterminate={null === job.progress.total}
				/>
				<ProgressSteps {steps} />
				<dl class="facts">
					<div class="fact">
						<dt>{t('jobs.target')}</dt>
						<dd>
							{#if null === targetOf(job)}
								<Text tone="muted">{t('jobs.target_unknown')}</Text>
							{:else}
								<PathCell path={targetOf(job) ?? ''} />
							{/if}
						</dd>
					</div>
					<div class="fact">
						<dt>{t('jobs.facts.started')}</dt>
						<dd>
							{#if null === job.started_at}
								<Text>{t('jobs.facts.not_started')}</Text>
							{:else}
								<TimeCell value={job.started_at} />
							{/if}
						</dd>
					</div>
					<div class="fact">
						<dt>{t('jobs.facts.duration')}</dt>
						<dd>
							{#if null === job.started_at}
								<Text>{t('jobs.facts.not_started')}</Text>
							{:else if null === job.finished_at}
								<Text>{t('jobs.facts.still_running')}</Text>
							{:else}
								<Duration value={Date.parse(job.finished_at) - Date.parse(job.started_at)} />
							{/if}
						</dd>
					</div>
					<div class="fact">
						<dt>{t('jobs.facts.speed')}</dt>
						<dd><Text>{t('jobs.rate', { rate: formatNumber(job.progress.per_second, 'decimal') })}</Text></dd>
					</div>
					<div class="fact">
						<dt>{t('jobs.facts.actor')}</dt>
						<dd><TextCell value={job.created_by} /></dd>
					</div>
					<div class="fact">
						<dt>{t('jobs.facts.kind')}</dt>
						<dd><Text>{t(KIND_KEY[job.kind])}</Text></dd>
					</div>
				</dl>
			</section>

			<Stack gap="4">
				<Heading level={2} size="title-sm">{t('jobs.failed.title')}</Heading>
				{#if isTruncated}
					<InlineAlert
						tone="warn"
						title={t('jobs.partial.title')}
						body={t('jobs.partial.body', { count: formatNumber(job.failed_items.length) })}
					/>
				{/if}
				<DataTable
					rows={failedRows}
					columns={failedColumns}
					rowKey={failedKey}
					label={t('jobs.failed.title')}
					status={failedStatus}
				>
					{#snippet empty()}
						<EmptyState
							variant="cleared"
							icon="ok"
							title={t('jobs.failed.empty.title')}
							body={t('jobs.failed.empty.body')}
						/>
					{/snippet}
				</DataTable>
			</Stack>
		</Stack>
	{/if}
</PageFrame>

<AlertDialog
	bind:open={cancelOpen}
	title={cancelTitle}
	body={cancelBody}
	confirmLabel={t('jobs.cancel.confirm')}
	tone="danger"
	onconfirm={() => void confirmCancel()}
/>

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--card-pad);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		min-inline-size: 0;
	}

	.progress-head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-2);
	}

	.facts {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(calc(var(--space-4) * 12), 1fr));
		gap: var(--space-4) var(--space-6);
		margin: 0;
		padding-block-start: var(--space-4);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.fact {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	dt {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
	}

	dd {
		margin: 0;
		min-inline-size: 0;
	}
</style>
