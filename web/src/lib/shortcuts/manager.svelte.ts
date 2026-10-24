import {
	shortcutDefinitions,
	type KeyAlternative,
	type KeymapOverrides,
	type ShortcutDefinition,
	type ShortcutScope
} from './keymap';

export interface ActiveShortcut {
	id: string;
	labelKey: string;
	scope: ShortcutScope;
	keys: KeyAlternative[];
}

interface Candidate {
	signature: string;
	run: () => void;
}

const MODIFIERS: readonly string[] = ['mod', 'alt', 'shift'];
const SEQUENCE_WINDOW_MS = 1000;
const KEY_NAMES: ReadonlyMap<string, string> = new Map([
	['Escape', 'esc'],
	[' ', 'space'],
	['ArrowUp', 'arrowup'],
	['ArrowDown', 'arrowdown'],
	['ArrowLeft', 'arrowleft'],
	['ArrowRight', 'arrowright']
]);

const isModifier = (token: string): boolean => MODIFIERS.includes(token);

const isSequenceAlternative = (keys: KeyAlternative): boolean =>
	1 < keys.length && keys.every((token) => !isModifier(token));

const isSingleKeyAlternative = (keys: KeyAlternative): boolean => {
	const [first = ''] = keys;
	return keys.every((token) => !isModifier(token)) && (isSequenceAlternative(keys) || 1 === first.length);
};

const signatureOfAlternative = (keys: KeyAlternative): string => {
	if (isSequenceAlternative(keys)) {
		return keys.join(' ');
	}
	const modifiers = MODIFIERS.filter((modifier) => keys.includes(modifier));
	const key = keys.filter((token) => !isModifier(token)).join('+');
	return [...modifiers, key].join('+');
};

const keyName = (raw: string): string => KEY_NAMES.get(raw) ?? raw.toLowerCase();

const eventSignature = (event: KeyboardEvent): string => {
	const key = keyName(event.key);
	const shiftApplies = /^[a-z]$/.test(key) || 1 < key.length;
	const tokens: string[] = [];
	if (event.metaKey || event.ctrlKey) {
		tokens.push('mod');
	}
	if (event.altKey) {
		tokens.push('alt');
	}
	if (shiftApplies && event.shiftKey) {
		tokens.push('shift');
	}
	tokens.push(key);
	return tokens.join('+');
};

const isEditable = (target: EventTarget | null): boolean => {
	if (!(target instanceof HTMLElement)) {
		return false;
	}
	return (
		['INPUT', 'TEXTAREA'].includes(target.tagName) ||
		null !== target.closest('[contenteditable]:not([contenteditable="false"])')
	);
};

const overlaps = (first: ShortcutScope, second: ShortcutScope): boolean =>
	'global' === first || 'global' === second || first === second;

export class ShortcutManager {
	readonly #definitions: readonly ShortcutDefinition[];
	readonly #handlers = new Map<string, () => void>();
	#overrides = $state.raw<KeymapOverrides>({});
	#singleKeyEnabled = $state(true);
	#pending: string[] = [];
	#pendingTimer: ReturnType<typeof setTimeout> | null = null;

	constructor(definitions: readonly ShortcutDefinition[] = shortcutDefinitions) {
		this.#definitions = definitions;
	}

	get singleKeyEnabled(): boolean {
		return this.#singleKeyEnabled;
	}

	setSingleKeyEnabled(enabled: boolean): void {
		this.#singleKeyEnabled = enabled;
	}

	get overrides(): KeymapOverrides {
		return this.#overrides;
	}

	setOverrides(overrides: KeymapOverrides): void {
		this.#overrides = { ...overrides };
	}

	setOverride(id: string, keys: KeyAlternative[]): void {
		this.#overrides = { ...this.#overrides, [id]: keys };
	}

	resetOverrides(): void {
		this.#overrides = {};
	}

	effective(): ActiveShortcut[] {
		return this.#definitions.map((definition) => ({
			id: definition.id,
			labelKey: definition.labelKey,
			scope: definition.scope,
			keys: Object.hasOwn(this.#overrides, definition.id) ? this.#overrides[definition.id] : definition.keys
		}));
	}

	bind(id: string, handler: () => void): () => void {
		this.#handlers.set(id, handler);
		return (): void => {
			if (handler === this.#handlers.get(id)) {
				this.#handlers.delete(id);
			}
		};
	}

	conflictsFor(id: string, keys: KeyAlternative[]): string[] {
		const scope = this.#definitions.find((definition) => definition.id === id)?.scope ?? 'global';
		const wanted = new Set(keys.map(signatureOfAlternative));
		return this.effective()
			.filter(
				(binding) =>
					binding.id !== id &&
					overlaps(binding.scope, scope) &&
					binding.keys.some((alternative) => wanted.has(signatureOfAlternative(alternative)))
			)
			.map((binding) => binding.id);
	}

	start(target: EventTarget = globalThis): () => void {
		target.addEventListener('keydown', this.#onKeydown);
		return (): void => {
			target.removeEventListener('keydown', this.#onKeydown);
			this.#clearPending();
		};
	}

	#onKeydown = (event: Event): void => {
		if (!(event instanceof KeyboardEvent) || event.isComposing) {
			return;
		}
		if (isEditable(event.target) && !(event.metaKey || event.ctrlKey)) {
			this.#clearPending();
			return;
		}
		const step = eventSignature(event);
		const hadPending = 0 < this.#pending.length;
		if (this.#resolve([...this.#pending, step], event)) {
			return;
		}
		this.#clearPending();
		if (hadPending) {
			this.#resolve([step], event);
		}
	};

	#resolve(steps: string[], event: KeyboardEvent): boolean {
		const typed = steps.join(' ');
		const candidates = this.#candidates();
		const exact = candidates.find((candidate) => candidate.signature === typed);
		if (undefined !== exact) {
			this.#clearPending();
			event.preventDefault();
			exact.run();
			return true;
		}
		if (candidates.some((candidate) => candidate.signature.startsWith(`${typed} `))) {
			this.#setPending(steps);
			return true;
		}
		return false;
	}

	#candidates(): Candidate[] {
		const candidates: Candidate[] = [];
		for (const binding of this.effective()) {
			const handler = this.#handlers.get(binding.id);
			if (undefined === handler) {
				continue;
			}
			for (const keys of binding.keys) {
				if (!this.#singleKeyEnabled && isSingleKeyAlternative(keys)) {
					continue;
				}
				candidates.push({ signature: signatureOfAlternative(keys), run: handler });
			}
		}
		return candidates;
	}

	#setPending(steps: string[]): void {
		this.#clearPending();
		this.#pending = steps;
		this.#pendingTimer = setTimeout(() => {
			this.#pending = [];
			this.#pendingTimer = null;
		}, SEQUENCE_WINDOW_MS);
	}

	#clearPending(): void {
		if (null !== this.#pendingTimer) {
			clearTimeout(this.#pendingTimer);
		}
		this.#pendingTimer = null;
		this.#pending = [];
	}
}
