import * as sessionApi from '$lib/api/session';
import type { Capabilities, Result } from '$lib/api/types';
import type { QueryStatus, UiError } from '$lib/ui/types';

const RETRY_DELAYS_MS = [500, 1000, 2000, 4000, 8000] as const;

const wait = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

async function fetchWithBackoff(): Promise<Result<Capabilities, UiError>> {
	let result = await sessionApi.capabilities();
	for (const delay of RETRY_DELAYS_MS) {
		if (result.ok || 429 !== result.error.status) {
			break;
		}
		await wait(delay);
		result = await sessionApi.capabilities();
	}
	return result;
}

export type CapabilityFeature = keyof Capabilities['features'];

export class CapabilitiesStore {
	data = $state<Capabilities | null>(null);
	error = $state<UiError | null>(null);
	status = $derived<QueryStatus>(null !== this.data ? 'success' : null !== this.error ? 'error' : 'loading');
	#inFlight: Promise<Result<Capabilities, UiError>> | null = null;

	load(): Promise<Result<Capabilities, UiError>> {
		if (null !== this.data) {
			return Promise.resolve({ ok: true, value: this.data });
		}
		if (null !== this.#inFlight) {
			return this.#inFlight;
		}
		this.error = null;
		this.#inFlight = fetchWithBackoff().then((result) => {
			this.#inFlight = null;
			if (false === result.ok) {
				this.error = result.error;
				return result;
			}
			this.data = result.value;
			this.error = null;
			return result;
		});
		return this.#inFlight;
	}

	reload(): Promise<Result<Capabilities, UiError>> {
		const pending: Promise<Result<Capabilities, UiError>> = fetchWithBackoff().then((result) => {
			if (pending !== this.#inFlight) {
				return result;
			}
			this.#inFlight = null;
			if (false === result.ok) {
				this.error = result.error;
				return result;
			}
			this.data = result.value;
			this.error = null;
			return result;
		});
		this.#inFlight = pending;
		return pending;
	}

	has(feature: CapabilityFeature): boolean {
		return null !== this.data && true === this.data.features[feature];
	}

	reset(): void {
		this.data = null;
		this.error = null;
		this.#inFlight = null;
	}
}

export const capabilities = new CapabilitiesStore();
