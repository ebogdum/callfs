import { listNodes, type NodeInfo, type NodeList } from '$lib/api/nodes';
import type { Capabilities, Result } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { clearCache } from '$lib/data/cache';
import { liveHub } from '$lib/live/hub.svelte';
import type { UiError } from '$lib/ui/types';
import { capabilities } from './capabilities.svelte';
import { nodeScope, type NodeScopeStore } from './node-scope.svelte';
import { toasts } from './toasts.svelte';

export interface ScopeEffects {
	clear: () => void;
	reload: () => Promise<Result<Capabilities, UiError>>;
	rescope: () => void;
}

const browserEffects: ScopeEffects = {
	clear: () => clearCache(),
	reload: () => capabilities.reload(),
	rescope: () => liveHub.rescope()
};

export class NodeDirectory {
	items = $state<NodeInfo[]>([]);
	selfId = $state<string | null>(null);
	error = $state<UiError | null>(null);
	updatedAt = $state<number | null>(null);
	loading = $state(false);
	multi = $derived(1 < this.items.length);
	upCount = $derived(this.items.filter((node) => 'up' === node.status).length);
	readonly #scope: NodeScopeStore;
	readonly #fetch: () => Promise<Result<NodeList, UiError>>;
	readonly #effects: ScopeEffects;
	#inFlight: Promise<Result<NodeList, UiError>> | null = null;

	constructor(
		scope: NodeScopeStore = nodeScope,
		fetch: () => Promise<Result<NodeList, UiError>> = listNodes,
		effects: ScopeEffects = browserEffects
	) {
		this.#scope = scope;
		this.#fetch = fetch;
		this.#effects = effects;
	}

	get selected(): string | null {
		return this.#scope.selected;
	}

	get activeId(): string | null {
		return this.#scope.selected ?? this.selfId;
	}

	get active(): NodeInfo | null {
		const id = this.activeId;
		return this.items.find((node) => id === node.id) ?? null;
	}

	isActive(node: NodeInfo): boolean {
		return null === this.#scope.selected ? true === node.self || this.selfId === node.id : this.#scope.selected === node.id;
	}

	load(): Promise<Result<NodeList, UiError>> {
		if (null !== this.#inFlight) {
			return this.#inFlight;
		}
		this.loading = true;
		this.#inFlight = this.#fetch().then((result) => {
			this.#inFlight = null;
			this.loading = false;
			if (false === result.ok) {
				this.error = result.error;
				return result;
			}
			this.items = result.value.nodes;
			this.selfId = '' === result.value.self ? (result.value.nodes.find((node) => node.self)?.id ?? null) : result.value.self;
			this.error = null;
			this.updatedAt = Date.now();
			const known = result.value.nodes.map((node) => node.id);
			if (this.#scope.retain(known, this.selfId)) {
				void this.#afterSwitch();
			}
			return result;
		});
		return this.#inFlight;
	}

	switchTo(id: string | null): Promise<Result<Capabilities, UiError>> | null {
		const target = null !== id && id === this.selfId ? null : id;
		if (false === this.#scope.select(target)) {
			return null;
		}
		return this.#afterSwitch();
	}

	#afterSwitch(): Promise<Result<Capabilities, UiError>> {
		this.#effects.clear();
		this.#effects.rescope();
		return this.#effects.reload();
	}
}

export const nodes = new NodeDirectory();

export function controlNode(id: string | null): void {
	const home = nodes.selfId ?? '';
	const target = id ?? home;
	const pending = nodes.switchTo(id);
	if (null === pending) {
		return;
	}
	void pending.then((result) => {
		if (false === result.ok) {
			toasts.push({ tone: 'danger', title: result.error.title, body: result.error.body });
			return;
		}
		const back = null === nodes.selected;
		toasts.push({
			tone: 'ok',
			title: back ? t('nodes.switch.home', { node: home }) : t('nodes.switch.done', { node: target })
		});
	});
}
