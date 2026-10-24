import type { IconName } from '$lib/ui/types';
import { fuzzyMatch, type FuzzyMatch } from '../format/fuzzy';

export type CommandGroup =
	| 'Navigate'
	| 'Files'
	| 'Sharing'
	| 'Cluster'
	| 'Insights'
	| 'Activity'
	| 'Settings'
	| 'Appearance'
	| 'Help'
	| 'Account';

export type CommandMode = 'all' | 'actions' | 'paths' | 'settings' | 'cluster' | 'help';

export interface CommandContext {
	role: 'admin' | 'member';
	route: string;
	selectionCount: number;
	capabilities: readonly string[];
}

export interface CommandSecondary {
	title: string;
	run: (ctx: CommandContext) => void | Promise<void>;
}

export interface Command {
	id: string;
	title: string;
	group: CommandGroup;
	keywords: string[];
	shortcut?: string[];
	icon?: IconName;
	when?: (ctx: CommandContext) => boolean;
	disabledReason?: (ctx: CommandContext) => string | null;
	run: (ctx: CommandContext) => void | Promise<void>;
	secondary?: CommandSecondary;
}

export interface CommandEntry {
	command: Command;
	disabledReason: string | null;
}

export interface SearchHit extends CommandEntry {
	score: number;
	indices: number[];
}

export interface CommandError {
	code: 'missing' | 'hidden' | 'disabled' | 'failed';
	reason: string | null;
}

export type CommandResult = { ok: true; value: null } | { ok: false; error: CommandError };

export interface CommandRegistryOptions {
	context: () => CommandContext;
	recent?: readonly string[];
	persistRecent?: (ids: string[]) => void;
}

const RECENT_LIMIT = 10;
const KEYWORD_WEIGHT = 0.5;

const MODE_GROUPS: Record<CommandMode, readonly CommandGroup[] | null> = {
	all: null,
	actions: ['Files', 'Sharing', 'Cluster', 'Insights', 'Activity', 'Appearance', 'Account'],
	paths: ['Navigate'],
	settings: ['Settings'],
	cluster: ['Cluster'],
	help: ['Help']
};

const PREFIX_MODES: ReadonlyMap<string, CommandMode> = new Map([
	['>', 'actions'],
	['/', 'paths'],
	['@', 'settings'],
	['#', 'cluster'],
	['?', 'help']
]);

export function parseCommandQuery(raw: string): { mode: CommandMode; query: string } {
	const mode = PREFIX_MODES.get(raw.charAt(0));
	if (undefined === mode) {
		return { mode: 'all', query: raw.trim() };
	}
	return { mode, query: raw.slice(1).trim() };
}

function matchCommand(query: string, command: Command): FuzzyMatch | null {
	const title = fuzzyMatch(query, command.title);
	if (null !== title) {
		return title;
	}
	let best: FuzzyMatch | null = null;
	for (const keyword of command.keywords) {
		const match = fuzzyMatch(query, keyword);
		if (null === match) {
			continue;
		}
		const score = match.score * KEYWORD_WEIGHT;
		if (null === best || score > best.score) {
			best = { score, indices: [] };
		}
	}
	return best;
}

function fail(code: CommandError['code'], reason: string | null): CommandResult {
	return { ok: false, error: { code, reason } };
}

export class CommandRegistry {
	#entries = $state.raw<Command[]>([]);
	#recent = $state.raw<string[]>([]);
	readonly #context: () => CommandContext;
	readonly #persistRecent: ((ids: string[]) => void) | null;

	constructor(options: CommandRegistryOptions) {
		this.#context = options.context;
		this.#persistRecent = options.persistRecent ?? null;
		this.#recent = [...(options.recent ?? [])].slice(0, RECENT_LIMIT);
	}

	get recent(): readonly string[] {
		return this.#recent;
	}

	register(commands: readonly Command[]): () => void {
		const ids = new Set(commands.map((command) => command.id));
		this.#entries = [...this.#entries.filter((entry) => !ids.has(entry.id)), ...commands];
		return (): void => {
			this.#entries = this.#entries.filter((entry) => !commands.includes(entry));
		};
	}

	list(ctx: CommandContext): CommandEntry[] {
		return this.#entries
			.filter((command) => false !== command.when?.(ctx))
			.map((command) => ({ command, disabledReason: command.disabledReason?.(ctx) ?? null }));
	}

	search(query: string, mode: CommandMode): SearchHit[] {
		const needle = query.trim();
		const groups = MODE_GROUPS[mode];
		const hits: SearchHit[] = [];
		for (const entry of this.list(this.#context())) {
			if (null !== groups && !groups.includes(entry.command.group)) {
				continue;
			}
			if ('' === needle) {
				hits.push({ ...entry, score: 0, indices: [] });
				continue;
			}
			const match = matchCommand(needle, entry.command);
			if (null !== match) {
				hits.push({ ...entry, score: match.score, indices: match.indices });
			}
		}
		return hits.sort((a, b) =>
			'' === needle ? this.#rank(a.command.id) - this.#rank(b.command.id) : b.score - a.score
		);
	}

	async execute(id: string, variant: 'primary' | 'secondary' = 'primary'): Promise<CommandResult> {
		const command = this.#entries.find((entry) => entry.id === id);
		if (undefined === command) {
			return fail('missing', null);
		}
		const ctx = this.#context();
		if (false === command.when?.(ctx)) {
			return fail('hidden', null);
		}
		const reason = command.disabledReason?.(ctx) ?? null;
		if (null !== reason) {
			return fail('disabled', reason);
		}
		const action = 'secondary' === variant ? command.secondary?.run : command.run;
		if (undefined === action) {
			return fail('missing', null);
		}
		try {
			await action(ctx);
		} catch (error) {
			return fail('failed', error instanceof Error ? error.message : null);
		}
		this.#recordRecent(id);
		return { ok: true, value: null };
	}

	#recordRecent(id: string): void {
		this.#recent = [id, ...this.#recent.filter((recent) => id !== recent)].slice(0, RECENT_LIMIT);
		this.#persistRecent?.([...this.#recent]);
	}

	#rank(id: string): number {
		const index = this.#recent.indexOf(id);
		return -1 === index ? RECENT_LIMIT : index;
	}
}

const EMPTY_CONTEXT: CommandContext = { role: 'member', route: '/', selectionCount: 0, capabilities: [] };

const commandScope = $state<{ context: CommandContext }>({ context: EMPTY_CONTEXT });

export function setCommandContext(context: CommandContext): void {
	commandScope.context = context;
}

const RECENT_STORAGE_KEY = 'callfs.recent-commands';

function readRecent(): string[] {
	try {
		const parsed: unknown = JSON.parse(globalThis.localStorage?.getItem(RECENT_STORAGE_KEY) ?? '[]');
		return Array.isArray(parsed) ? parsed.filter((id): id is string => 'string' === typeof id) : [];
	} catch {
		return [];
	}
}

function writeRecent(ids: string[]): void {
	try {
		globalThis.localStorage?.setItem(RECENT_STORAGE_KEY, JSON.stringify(ids));
	} catch {
		return;
	}
}

export const commands = new CommandRegistry({
	context: () => commandScope.context,
	recent: readRecent(),
	persistRecent: writeRecent
});

export function register(entries: readonly Command[]): () => void {
	return commands.register(entries);
}
