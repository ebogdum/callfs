import { toUiError, type ApiFailure, type ErrorContext } from '$lib/api/errors';
import type { Result } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';
import { failureOf, invalidate } from './cache';

export interface UndoAction {
	label: string;
	run: () => Promise<Result<unknown, UiError>>;
}

export interface MutationDeps {
	registerUndo?: (undo: UndoAction) => void;
	announce?: (message: string) => void;
}

export interface MutationOptions<V, R> {
	run: (vars: V) => Promise<R>;
	optimistic?: (vars: V) => () => void;
	invalidate?: (vars: V, result: R) => readonly unknown[][];
	undo?: (vars: V, result: R) => { label: string; run: () => Promise<unknown> };
	success?: (vars: V, result: R) => string;
	context: { action: string; object?: (vars: V) => string };
}

export class Mutation<V, R> {
	pending = $state(false);
	error = $state<UiError | null>(null);

	readonly #options: MutationOptions<V, R>;
	readonly #deps: MutationDeps;

	constructor(options: MutationOptions<V, R>, deps: MutationDeps) {
		this.#options = options;
		this.#deps = deps;
	}

	async run(vars: V): Promise<R | null> {
		const rollback = this.#options.optimistic?.(vars);
		this.pending = true;
		this.error = null;
		const outcome = await this.#attempt(() => this.#options.run(vars));
		this.pending = false;
		if (outcome.ok) {
			this.#settle(vars, outcome.value);
			return outcome.value;
		}
		rollback?.();
		this.error = toUiError(outcome.error, this.#context(vars));
		return null;
	}

	#settle(vars: V, result: R): void {
		const keys = this.#options.invalidate?.(vars, result) ?? [];
		for (const key of keys) {
			invalidate(key);
		}
		const undo = this.#options.undo?.(vars, result);
		if (undefined !== undo) {
			this.#deps.registerUndo?.({
				label: undo.label,
				run: () => this.#reverse(undo.run, keys, vars)
			});
		}
		const message = this.#options.success?.(vars, result);
		if (undefined !== message) {
			this.#deps.announce?.(message);
		}
	}

	async #reverse(run: () => Promise<unknown>, keys: readonly unknown[][], vars: V): Promise<Result<unknown, UiError>> {
		const outcome = await this.#attempt(run);
		for (const key of keys) {
			invalidate(key);
		}
		if (false === outcome.ok) {
			return { ok: false, error: toUiError(outcome.error, this.#context(vars)) };
		}
		return { ok: true, value: outcome.value };
	}

	async #attempt<X>(work: () => Promise<X>): Promise<Result<X, ApiFailure>> {
		try {
			return { ok: true, value: await work() };
		} catch (error) {
			return { ok: false, error: failureOf(error) };
		}
	}

	#context(vars: V): ErrorContext {
		const { action, object } = this.#options.context;
		return undefined === object ? { action } : { action, object: object(vars) };
	}
}

export function createMutation<V, R>(opts: MutationOptions<V, R>, deps: MutationDeps = {}): Mutation<V, R> {
	return new Mutation(opts, deps);
}
