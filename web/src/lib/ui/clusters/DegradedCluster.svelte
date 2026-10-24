<script lang="ts" module>
	export type ClusterRole = 'admin' | 'member';
	export type DegradedCause = 'unreachable' | 'no-leader' | 'health-down';

	export interface DegradedStatus {
		degraded: boolean;
		disabledReason: string | null;
	}
</script>

<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cluster as loadCluster } from '$lib/api/admin/cluster';
	import { health as loadHealth } from '$lib/api/admin/health';
	import { me as loadMe } from '$lib/api/session';
	import type { Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { formatTimestamp } from '$lib/format/time';
	import { LiveConnection } from '$lib/live/connection.svelte';
	import type { UiError } from '$lib/ui/types';
	import Banner from '../molecules/Banner.svelte';
	import StaleIndicator from '../molecules/StaleIndicator.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class' | 'children'> {
		role: ClusterRole;
		children: Snippet<[DegradedStatus]>;
		class?: string;
	}

	const HEALTHY_CHECK_MS = 10_000;
	const MAX_RETRY_MS = 30_000;
	const RETRY_STEPS_MS: readonly number[] = [1_000, 2_000, 5_000, 10_000, MAX_RETRY_MS];
	const COPY_KEYS: Record<DegradedCause, { title: string; body: string; write: string }> = {
		unreachable: {
			title: 'clusters.degraded.unreachable.title',
			body: 'clusters.degraded.unreachable.body',
			write: 'clusters.degraded.unreachable.write'
		},
		'no-leader': {
			title: 'clusters.degraded.no_leader.title',
			body: 'clusters.degraded.no_leader.body',
			write: 'clusters.degraded.no_leader.write'
		},
		'health-down': {
			title: 'clusters.degraded.health.title',
			body: 'clusters.degraded.health.body',
			write: 'clusters.degraded.health.write'
		}
	};

	let { role, children, class: className, ...rest }: Props = $props();

	let probedCause = $state<DegradedCause | null>(null);
	let since = $state<number | null>(null);
	let checkedAt = $state<number | null>(null);
	let intervalMs = $state(HEALTHY_CHECK_MS);
	let retryStep = 0;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let disposed = false;

	const connection = new LiveConnection({ onPause: stopTimer, onResume: retryNow });

	const cause = $derived<DegradedCause | null>('offline' === connection.state ? 'unreachable' : probedCause);
	const failure = $derived(null === cause ? null : COPY_KEYS[cause]);
	const sinceText = $derived(null === since ? '' : formatTimestamp(since));
	const status = $derived<DegradedStatus>({
		degraded: null !== cause,
		disabledReason: null === cause ? null : t(COPY_KEYS[cause].write, { since: sinceText })
	});

	function isNetworkFailure(result: Result<unknown, UiError>): boolean {
		return false === result.ok && 'network' === result.error.layer;
	}

	async function probeAdmin(): Promise<DegradedCause | null> {
		const [healthResult, clusterResult] = await Promise.all([loadHealth(true), loadCluster()]);
		if (isNetworkFailure(healthResult) || isNetworkFailure(clusterResult)) {
			return 'unreachable';
		}
		if (true === clusterResult.ok && 'nats' === clusterResult.value.mode && null === clusterResult.value.leader_id) {
			return 'no-leader';
		}
		if (true === healthResult.ok && 'down' === healthResult.value.status) {
			return 'health-down';
		}
		return null;
	}

	async function probeMember(): Promise<DegradedCause | null> {
		return isNetworkFailure(await loadMe()) ? 'unreachable' : null;
	}

	async function check(): Promise<void> {
		const next = 'admin' === role ? await probeAdmin() : await probeMember();
		if (disposed) {
			return;
		}
		checkedAt = Date.now();
		if (null === next) {
			probedCause = null;
			since = null;
			retryStep = 0;
			schedule(HEALTHY_CHECK_MS);
			return;
		}
		if (null === probedCause) {
			since = Date.now();
		}
		probedCause = next;
		schedule(nextRetryDelay());
	}

	function nextRetryDelay(): number {
		const delay = RETRY_STEPS_MS[retryStep] ?? MAX_RETRY_MS;
		retryStep = Math.min(retryStep + 1, RETRY_STEPS_MS.length);
		return delay;
	}

	function schedule(delayMs: number): void {
		stopTimer();
		intervalMs = delayMs;
		timer = setTimeout(() => {
			timer = null;
			void check();
		}, delayMs);
	}

	function stopTimer(): void {
		if (null !== timer) {
			clearTimeout(timer);
			timer = null;
		}
	}

	function retryNow(): void {
		stopTimer();
		retryStep = 0;
		void check();
	}

	$effect(() => {
		disposed = false;
		const stopWatching = connection.watch();
		void untrack(check);
		return (): void => {
			disposed = true;
			stopWatching();
			stopTimer();
		};
	});
</script>

<Stack gap="4" class={className} {...rest}>
	{#if null !== failure}
		<Banner
			tone="danger"
			title={t(failure.title)}
			body={t(failure.body, { since: sinceText })}
			action={{ label: t('clusters.degraded.retry'), run: retryNow }}
		/>
		<StaleIndicator updatedAt={checkedAt} {intervalMs} state="offline" onrefresh={retryNow} />
	{/if}
	{@render children(status)}
</Stack>
