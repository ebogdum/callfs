<script lang="ts" module>
	import type { Job } from '$lib/api/types';
	import type { StatusKey } from '$lib/status/catalog';

	export const TOKENS = ['--job-list-gap', '--job-row-pad', '--job-row-border', '--job-row-radius'] as const;

	const JOB_STATUS: Record<Job['state'], StatusKey> = {
		queued: 'job.queued',
		running: 'job.running',
		succeeded: 'job.succeeded',
		partial: 'job.partial',
		failed: 'job.failed',
		cancelled: 'job.cancelled',
		cancelling: 'job.cancelling'
	};

	const CANCELLABLE_STATES: readonly Job['state'][] = ['queued', 'running', 'cancelling'];
	const RETRYABLE_STATES: readonly Job['state'][] = ['partial', 'failed'];

	function jobTarget(job: Job): string | null {
		const target = job.result.path ?? job.result.prefix;
		return 'string' === typeof target ? target : null;
	}

	function jobDurationMs(job: Job): number | null {
		if (null === job.started_at || null === job.finished_at) {
			return null;
		}
		const duration = Date.parse(job.finished_at) - Date.parse(job.started_at);
		return Number.isFinite(duration) ? duration : null;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Duration from '../atoms/Duration.svelte';
	import RelTime from '../atoms/RelTime.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Text from '../atoms/Text.svelte';
	import ProgressCell from '../cells/ProgressCell.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'oncancel'> {
		jobs: Job[];
		onopen: (job: Job) => void;
		oncancel: (job: Job) => void;
		onretryfailed: (job: Job) => void;
		class?: string;
	}

	let { jobs, onopen, oncancel, onretryfailed, class: className, ...rest }: Props = $props();
</script>

<div class={['job-list', className]} {...rest}>
	{#if 0 === jobs.length}
		<Text tone="muted">{t('support.jobs.empty')}</Text>
	{:else}
		<ul class="job-items">
			{#each jobs as job (job.id)}
				{@const status = JOB_STATUS[job.state]}
				{@const target = jobTarget(job)}
				{@const duration = jobDurationMs(job)}
				<li class="job-row" data-state={job.state}>
					<Stack gap="2">
						<Cluster justify="space-between" align="center" wrap gap="2">
							<Text weight={600}>{t(`support.jobs.kind.${job.kind}`)}</Text>
							<StatusPill {status} />
						</Cluster>
						<Text size="sm" tone="muted">
							{t('support.jobs.target')}: {target ?? t('support.jobs.target_unknown')}
						</Text>
						<ProgressCell done={job.progress.done} total={job.progress.total} state={status} />
						<dl class="job-facts">
							<div class="job-fact">
								<dt><Text size="sm" tone="muted">{t('support.jobs.started')}</Text></dt>
								<dd>
									{#if null === job.started_at}
										<Text size="sm">{t('support.jobs.not_started')}</Text>
									{:else}
										<RelTime at={job.started_at} />
									{/if}
								</dd>
							</div>
							<div class="job-fact">
								<dt><Text size="sm" tone="muted">{t('support.jobs.duration')}</Text></dt>
								<dd>
									{#if null !== duration}
										<Duration value={duration} style="short" />
									{:else if null === job.started_at}
										<Text size="sm">{t('support.jobs.not_started')}</Text>
									{:else}
										<Text size="sm">{t('support.jobs.still_running')}</Text>
									{/if}
								</dd>
							</div>
							<div class="job-fact">
								<dt><Text size="sm" tone="muted">{t('support.jobs.actor')}</Text></dt>
								<dd><Text size="sm">{job.created_by}</Text></dd>
							</div>
						</dl>
						{#if null !== job.phase}
							<Text size="sm" tone="muted">{t(`support.jobs.phase.${job.phase}`)}</Text>
						{/if}
						<Cluster gap="2" align="center" wrap>
							<Button size="sm" variant="ghost" onclick={() => onopen(job)}>{t('support.jobs.open')}</Button>
							{#if CANCELLABLE_STATES.includes(job.state)}
								<Button
									size="sm"
									variant="secondary"
									disabledReason={'cancelling' === job.state ? t('support.jobs.stopping') : undefined}
									onclick={() => oncancel(job)}>{t('support.jobs.cancel')}</Button
								>
							{/if}
							{#if 0 < job.failed_count && RETRYABLE_STATES.includes(job.state)}
								<Button size="sm" variant="secondary" onclick={() => onretryfailed(job)}>
									{t('support.jobs.retry_failed')}
								</Button>
								<Text size="sm" tone="danger">
									{t('support.jobs.failed_count', { count: job.failed_count })}
								</Text>
							{/if}
						</Cluster>
					</Stack>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.job-list {
		display: grid;
		gap: var(--job-list-gap, var(--space-4));
		min-inline-size: 0;
	}

	.job-items {
		display: grid;
		gap: var(--job-list-gap, var(--space-4));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.job-row {
		padding: var(--job-row-pad, var(--space-4));
		border: var(--hairline) solid var(--job-row-border, var(--border-subtle));
		border-radius: var(--job-row-radius, var(--radius-lg));
		min-inline-size: 0;
	}

	.job-facts {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		margin: 0;
	}

	.job-fact {
		display: grid;
		gap: var(--space-1);
	}

	.job-fact dt,
	.job-fact dd {
		margin: 0;
	}
</style>
