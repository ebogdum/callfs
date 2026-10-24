import { job as fetchJob } from '$lib/api/session';
import type { Job } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';

const POLL_MS = 2_000;
const RUNNING_STATES: readonly Job['state'][] = ['queued', 'running', 'cancelling'];

export interface WatchedJob {
	id: string;
	job: Job | null;
	error: UiError | null;
}

export class JobWatch {
	entries = $state<WatchedJob[]>([]);
	running = $derived(
		this.entries.filter((entry) => null !== entry.job && RUNNING_STATES.includes(entry.job.state)).length
	);
	failed = $derived(this.entries.filter((entry) => 'failed' === entry.job?.state).length);

	private readonly timers = new Map<string, ReturnType<typeof setTimeout>>();

	watch(id: string): void {
		if (undefined !== this.find(id)) {
			return;
		}
		this.entries = [...this.entries, { id, job: null, error: null }];
		void this.poll(id);
	}

	unwatch(id: string): void {
		this.stop(id);
		this.entries = this.entries.filter((entry) => id !== entry.id);
	}

	receive(job: Job): void {
		if (undefined === this.find(job.id)) {
			return;
		}
		this.replace(job.id, { job, error: null });
		if (false === RUNNING_STATES.includes(job.state)) {
			this.stop(job.id);
		}
	}

	private async poll(id: string): Promise<void> {
		this.timers.delete(id);
		const result = await fetchJob(id);
		if (undefined === this.find(id)) {
			return;
		}
		if (result.ok) {
			this.replace(id, { job: result.value, error: null });
		} else {
			this.replace(id, { error: result.error });
		}
		const keepPolling = result.ok ? RUNNING_STATES.includes(result.value.state) : result.error.retryable;
		if (keepPolling) {
			this.timers.set(
				id,
				setTimeout(() => {
					void this.poll(id);
				}, POLL_MS)
			);
		}
	}

	private find(id: string): WatchedJob | undefined {
		return this.entries.find((entry) => id === entry.id);
	}

	private replace(id: string, changes: Partial<WatchedJob>): void {
		this.entries = this.entries.map((entry) => (id === entry.id ? { ...entry, ...changes } : entry));
	}

	private stop(id: string): void {
		const timer = this.timers.get(id);
		if (undefined !== timer) {
			clearTimeout(timer);
			this.timers.delete(id);
		}
	}
}

export const jobs = new JobWatch();
