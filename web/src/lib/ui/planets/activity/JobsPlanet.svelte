<script lang="ts">
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { cancel as cancelAdminJob, jobs, retryFailed as retryAdminJob, runWorker, workers } from '$lib/api/admin/jobs';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { cancelJob, myJobs, retryJobFailed } from '$lib/api/session';
	import type { Job, Result, Worker } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { ColumnDef, QueryStatus, UiError } from '$lib/ui/types';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Duration from '$lib/ui/atoms/Duration.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import JobList from '$lib/ui/organisms/JobList.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import SegmentedControl from '$lib/ui/molecules/SegmentedControl.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	type ListTab = 'running' | 'failed' | 'done';
	type JobTab = ListTab | 'background';

	const COMMANDS_CONTEXT = 'callfs.commands';
	const TABS: readonly JobTab[] = ['running', 'failed', 'done', 'background'];
	const ACTIVE_STATES: readonly Job['state'][] = ['queued', 'running', 'cancelling'];
	const JOBS_INTERVAL_MS = 2000;
	const WORKERS_INTERVAL_MS = 5000;

	const ACTION_LOAD = 'jobs.action.load';
	const ACTION_CANCEL = 'jobs.action.cancel';
	const ACTION_RETRY = 'jobs.action.retry';
	const ACTION_WORKERS = 'jobs.action.workers';
	const ACTION_RUN = 'jobs.action.run';

	const LIST_STATES: Record<ListTab, readonly Job['state'][]> = {
		running: ACTIVE_STATES,
		failed: ['failed', 'partial'],
		done: ['succeeded', 'cancelled']
	};

	const TAB_LABEL: Record<JobTab, string> = {
		running: 'jobs.tabs.running',
		failed: 'jobs.tabs.failed',
		done: 'jobs.tabs.done',
		background: 'jobs.tabs.background'
	};

	const EMPTY_COPY: Record<ListTab, { title: string; body: string }> = {
		running: { title: 'jobs.empty.running.title', body: 'jobs.empty.running.body' },
		failed: { title: 'jobs.empty.failed.title', body: 'jobs.empty.failed.body' },
		done: { title: 'jobs.empty.done.title', body: 'jobs.empty.done.body' }
	};

	const WORKER_NAME_KEY: Record<Worker['name'], string> = {
		link_cleanup: 'jobs.worker.link_cleanup',
		trash_cleanup: 'jobs.worker.trash_cleanup',
		upload_cleanup: 'jobs.worker.upload_cleanup',
		instance_prober: 'jobs.worker.instance_prober',
		metrics_sampler: 'jobs.worker.metrics_sampler',
		usage_refresh: 'jobs.worker.usage_refresh'
	};

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	function parseTab(value: string | null): JobTab {
		if ('failed' === value || 'done' === value || 'background' === value) {
			return value;
		}
		return 'running';
	}

	function isListTab(value: JobTab): value is ListTab {
		return 'background' !== value;
	}

	function fromAdmin<T>(result: Result<T, ApiFailure>, action: string): Result<T, UiError> {
		return true === result.ok ? result : { ok: false, error: toUiError(result.error, { action }) };
	}

	async function listJobs(): Promise<Result<Job[], UiError>> {
		if (session.isAdmin) {
			const result = await jobs();
			return true === result.ok ? { ok: true, value: result.value.items } : fromAdmin(result, ACTION_LOAD);
		}
		const result = await myJobs();
		return true === result.ok ? { ok: true, value: result.value.items } : result;
	}

	function inTab(job: Job, target: ListTab): boolean {
		return LIST_STATES[target].includes(job.state);
	}

	function tabLabel(id: JobTab): string {
		const label = t(TAB_LABEL[id]);
		if (false === isListTab(id)) {
			return label;
		}
		const count = allJobs.filter((job) => inTab(job, id)).length;
		return 0 === count ? label : t('activity.jobs.tabCount', { label, count });
	}

	function selectTab(next: string): void {
		const target = parseTab(next);
		goto(`${base}/activity/jobs?tab=${target}`, { replaceState: true, noScroll: true, keepFocus: true }).catch(() =>
			toasts.push({ tone: 'danger', title: t('jobs.open_failed') })
		);
	}

	async function listWorkers(): Promise<Result<Worker[], UiError>> {
		const result = await workers();
		return true === result.ok ? { ok: true, value: result.value.items } : fromAdmin(result, ACTION_WORKERS);
	}

	async function cancelById(id: string): Promise<Result<Job, UiError>> {
		if (session.isAdmin) {
			return fromAdmin(await cancelAdminJob(id), ACTION_CANCEL);
		}
		return cancelJob(id);
	}

	async function retryById(id: string): Promise<Result<Job, UiError>> {
		if (session.isAdmin) {
			return fromAdmin(await retryAdminJob(id), ACTION_RETRY);
		}
		return retryJobFailed(id);
	}

	function isDeleteJob(job: Job | null): boolean {
		return null !== job && ('delete_prefix' === job.kind || 'delete_recursive' === job.kind);
	}

	const requestedTab = $derived(parseTab(page.url.searchParams.get('tab')));
	const tab = $derived(!session.isAdmin && 'background' === requestedTab ? 'running' : requestedTab);
	const listTab = $derived(isListTab(tab) ? tab : 'running');
	const tabOptions = $derived(
		(session.isAdmin ? TABS : TABS.filter((id) => 'background' !== id)).map((id) => ({
			value: id,
			label: tabLabel(id)
		}))
	);

	let online = $state(navigator.onLine);
	let cancelOpen = $state(false);
	let cancelTarget = $state<Job | null>(null);
	let actionError = $state<UiError | null>(null);
	let runningWorker = $state<string | null>(null);

	const cancelTitle = $derived(t(isDeleteJob(cancelTarget) ? 'jobs.cancel.delete.title' : 'jobs.cancel.generic.title'));
	const cancelBody = $derived(t(isDeleteJob(cancelTarget) ? 'jobs.cancel.delete.body' : 'jobs.cancel.generic.body'));

	const jobsQuery = createQuery(() => ({
		key: ['jobs', session.isAdmin ? 'admin' : 'own'],
		fetch: () => listJobs(),
		staleMs: JOBS_INTERVAL_MS,
		refetchMs: (data) =>
			true === data?.ok && data.value.some((item) => ACTIVE_STATES.includes(item.state)) ? JOBS_INTERVAL_MS : false,
		revalidateOnFocus: true
	}));

	const workersQuery = createQuery(() => ({
		key: ['workers'],
		fetch: () => listWorkers(),
		enabled: () => session.isAdmin && 'background' === tab,
		staleMs: WORKERS_INTERVAL_MS,
		revalidateOnFocus: true
	}));

	$effect(() => jobsQuery.attach());
	$effect(() => workersQuery.attach());

	$effect(() => {
		const update = (): void => {
			online = navigator.onLine;
		};
		window.addEventListener('online', update);
		window.addEventListener('offline', update);
		return () => {
			window.removeEventListener('online', update);
			window.removeEventListener('offline', update);
		};
	});

	const jobsResult = $derived(jobsQuery.data);
	const allJobs = $derived(true === jobsResult?.ok ? jobsResult.value : []);
	const jobRows = $derived(allJobs.filter((job) => inTab(job, listTab)));
	const jobsError = $derived(false === jobsResult?.ok ? jobsResult.error : null);
	const jobsState = $derived.by((): QueryStatus => {
		if (undefined === jobsResult) {
			return 'loading';
		}
		if (false === jobsResult.ok) {
			return 'error';
		}
		return 0 === jobRows.length ? 'empty' : 'success';
	});

	const workersResult = $derived(workersQuery.data);
	const workerRows = $derived(true === workersResult?.ok ? workersResult.value : []);
	const workersError = $derived(false === workersResult?.ok ? workersResult.error : null);
	const workersState = $derived.by((): QueryStatus => {
		if (undefined === workersResult) {
			return 'loading';
		}
		if (false === workersResult.ok) {
			return 'error';
		}
		if (0 === workersResult.value.length) {
			return 'empty';
		}
		return workersResult.value.some((worker) => null !== worker.last_error) ? 'partial' : 'success';
	});

	const activeUpdatedAt = $derived('background' === tab ? workersQuery.updatedAt : jobsQuery.updatedAt);
	const activeIntervalMs = $derived('background' === tab ? WORKERS_INTERVAL_MS : JOBS_INTERVAL_MS);

	function refresh(): void {
		void jobsQuery.refetch();
		void workersQuery.refetch();
	}

	function openJob(job: Job): void {
		goto(`${base}/activity/jobs/${encodeURIComponent(job.id)}`).catch(() =>
			toasts.push({ tone: 'danger', title: t('jobs.open_failed') })
		);
	}

	function askCancel(job: Job): void {
		cancelTarget = job;
		cancelOpen = true;
	}

	async function confirmCancel(): Promise<void> {
		const target = cancelTarget;
		cancelOpen = false;
		if (null === target) {
			return;
		}
		const result = await cancelById(target.id);
		if (false === result.ok) {
			actionError = result.error;
			return;
		}
		actionError = null;
		invalidate(['jobs']);
		invalidate(['job', target.id]);
	}

	async function retryJob(job: Job): Promise<void> {
		const result = await retryById(job.id);
		if (false === result.ok) {
			actionError = result.error;
			return;
		}
		actionError = null;
		invalidate(['jobs']);
		invalidate(['job', job.id]);
	}

	async function runNow(worker: Worker): Promise<void> {
		runningWorker = worker.name;
		const result = await runWorker(worker.name);
		runningWorker = null;
		if (false === result.ok) {
			actionError = toUiError(result.error, { action: ACTION_RUN });
			return;
		}
		actionError = null;
		invalidate(['workers']);
		invalidate(['jobs']);
		toasts.push({ tone: 'ok', title: t('jobs.workers.started', { name: t(WORKER_NAME_KEY[worker.name]) }) });
	}

	function workerResult(worker: Worker): string {
		if (null === worker.last_run_at) {
			return t('jobs.workers.result.never');
		}
		return null === worker.last_error ? t('jobs.workers.result.ok') : t('jobs.workers.result.failed');
	}

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'activity.jobs.open',
				title: t('jobs.command.open'),
				group: 'Navigate',
				keywords: ['job', 'jobs', 'open'],
				icon: 'jobs',
				run: () => {
					const [first] = jobRows;
					if (undefined !== first) {
						openJob(first);
					}
				}
			}
		]);
	});

	const workerColumns: ColumnDef<Worker>[] = [
		{ id: 'name', header: t('jobs.workers.col.name'), cell: workerNameCell },
		{ id: 'every', header: t('jobs.workers.col.every'), cell: workerEveryCell },
		{ id: 'last', header: t('jobs.workers.col.last'), cell: workerLastCell },
		{ id: 'result', header: t('jobs.workers.col.result'), cell: workerResultCell },
		{ id: 'next', header: t('jobs.workers.col.next'), cell: workerNextCell },
		{ id: 'action', header: t('jobs.workers.col.action'), cell: workerActionCell, align: 'end' }
	];

	const workerKey = (worker: Worker): string => worker.name;
</script>

{#snippet workerNameCell(worker: Worker)}
	<TextCell value={t(WORKER_NAME_KEY[worker.name])} />
{/snippet}

{#snippet workerEveryCell(worker: Worker)}
	<Duration value={worker.interval_s * 1000} />
{/snippet}

{#snippet workerLastCell(worker: Worker)}
	<TimeCell value={worker.last_run_at} />
{/snippet}

{#snippet workerResultCell(worker: Worker)}
	<TextCell value={workerResult(worker)} />
{/snippet}

{#snippet workerNextCell(worker: Worker)}
	<TimeCell value={worker.next_run_at} />
{/snippet}

{#snippet workerActionCell(worker: Worker)}
	<Button
		size="sm"
		variant="secondary"
		iconStart="play"
		loading={runningWorker === worker.name}
		disabledReason={worker.running ? t('jobs.workers.running') : undefined}
		onclick={() => void runNow(worker)}
	>
		{t('jobs.workers.run')}
	</Button>
{/snippet}

<PageFrame title={t('jobs.title')} job={t('jobs.job')} class={className}>
	{#snippet status()}
		<StaleIndicator
			updatedAt={activeUpdatedAt}
			intervalMs={activeIntervalMs}
			state={online ? 'live' : 'offline'}
			onrefresh={refresh}
		/>
	{/snippet}

	{#snippet toolbar()}
		<SegmentedControl label={t('jobs.tabs.label')} options={tabOptions} bind:value={() => tab, selectTab} />
	{/snippet}

	<Stack gap="8">
		{#if null !== actionError}
			<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
		{/if}

		{#if 'background' === tab}
			{#if null !== workersError}
				<ErrorState error={workersError} onretry={() => void workersQuery.refetch()} />
			{:else}
				{#if 'partial' === workersState}
					<InlineAlert tone="warn" title={t('jobs.workers.partial.title')} body={t('jobs.workers.partial.body')} />
				{/if}
				<DataTable
					rows={workerRows}
					columns={workerColumns}
					rowKey={workerKey}
					label={t('jobs.workers.label')}
					status={workersState}
				>
					{#snippet empty()}
						<EmptyState
							variant="not-available"
							icon="server"
							title={t('jobs.workers.empty.title')}
							body={t('jobs.workers.empty.body')}
						/>
					{/snippet}
				</DataTable>
			{/if}
		{:else if null !== jobsError}
			<ErrorState error={jobsError} onretry={() => void jobsQuery.refetch()} />
		{:else if 'loading' === jobsState}
			<Skeleton shape="rect" height="12rem" />
		{:else if 'empty' === jobsState}
			<EmptyState
				variant="cleared"
				icon="jobs"
				title={t(EMPTY_COPY[listTab].title)}
				body={t(EMPTY_COPY[listTab].body)}
			/>
		{:else}
			<JobList
				jobs={jobRows}
				onopen={openJob}
				oncancel={askCancel}
				onretryfailed={(job) => void retryJob(job)}
			/>
		{/if}
	</Stack>
</PageFrame>

<AlertDialog
	bind:open={cancelOpen}
	title={cancelTitle}
	body={cancelBody}
	confirmLabel={t('jobs.cancel.confirm')}
	tone="danger"
	onconfirm={() => void confirmCancel()}
/>
