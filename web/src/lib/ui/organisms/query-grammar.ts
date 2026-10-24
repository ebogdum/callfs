import type { CopyKey } from '../../copy/t';

export type QueryField =
	| 'name'
	| 'type'
	| 'size'
	| 'modified'
	| 'owner'
	| 'storage'
	| 'server'
	| 'split'
	| 'locked'
	| 'path'
	| 'user'
	| 'op'
	| 'status'
	| 'level';

export type QueryTokenField = QueryField | 'text';
export type Comparator = ':' | '>' | '<' | '>=' | '<=';
export type QueryLevel = 'debug' | 'info' | 'warn' | 'error';
export type QueryErrorReason =
	| 'unknown_field'
	| 'empty_value'
	| 'bad_value'
	| 'bad_size'
	| 'bad_date'
	| 'bad_status'
	| 'bad_range'
	| 'unterminated_quote';

export interface QueryToken {
	field: QueryTokenField;
	comparator: Comparator;
	value: string;
	negated: boolean;
	raw: string;
	start: number;
	end: number;
}

export interface QueryError {
	reason: QueryErrorReason;
	raw: string;
	start: number;
	end: number;
}

export interface SearchQuery {
	name?: string;
	text?: string;
	path?: string;
	type?: 'file' | 'directory';
	minSize?: number;
	maxSize?: number;
	modifiedAfter?: string;
	modifiedBefore?: string;
	owner?: string;
	storage?: string;
	server?: string;
	split?: boolean;
	locked?: boolean;
	user?: string;
	action?: string;
	status?: { comparator: Comparator; code: number };
	level?: QueryLevel;
	exclude: QueryToken[];
}

export interface ParseResult {
	tokens: QueryToken[];
	errors: QueryError[];
	query: SearchQuery;
}

export interface QueryFieldDef {
	key: QueryField;
	label: CopyKey;
	hint: CopyKey;
	options?: readonly QueryValueOption[];
}

export interface QueryGrammar {
	fields: QueryFieldDef[];
}

export const grammar: QueryGrammar = {
	fields: [
		{ key: 'name', label: 'u095query.field_name', hint: 'u095query.hint_name' },
		{
			key: 'type',
			label: 'u095query.field_type',
			hint: 'u095query.hint_type',
			options: [
				{ value: 'file', label: 'u095query.value_type_file' },
				{ value: 'directory', label: 'u095query.value_type_directory' }
			]
		},
		{ key: 'size', label: 'u095query.field_size', hint: 'u095query.hint_size' },
		{ key: 'modified', label: 'u095query.field_modified', hint: 'u095query.hint_modified' },
		{ key: 'owner', label: 'u095query.field_owner', hint: 'u095query.hint_owner' },
		{ key: 'storage', label: 'u095query.field_storage', hint: 'u095query.hint_storage' },
		{ key: 'server', label: 'u095query.field_server', hint: 'u095query.hint_server' },
		{
			key: 'split',
			label: 'u095query.field_split',
			hint: 'u095query.hint_split',
			options: [
				{ value: 'yes', label: 'u095query.value_yes' },
				{ value: 'no', label: 'u095query.value_no' }
			]
		},
		{
			key: 'locked',
			label: 'u095query.field_locked',
			hint: 'u095query.hint_locked',
			options: [
				{ value: 'yes', label: 'u095query.value_yes' },
				{ value: 'no', label: 'u095query.value_no' }
			]
		},
		{ key: 'path', label: 'u095query.field_path', hint: 'u095query.hint_path' },
		{ key: 'user', label: 'u095query.field_user', hint: 'u095query.hint_user' },
		{ key: 'op', label: 'u095query.field_op', hint: 'u095query.hint_op' },
		{ key: 'status', label: 'u095query.field_status', hint: 'u095query.hint_status' },
		{
			key: 'level',
			label: 'u095query.field_level',
			hint: 'u095query.hint_level',
			options: [
				{ value: 'debug', label: 'u095query.value_level_debug' },
				{ value: 'info', label: 'u095query.value_level_info' },
				{ value: 'warn', label: 'u095query.value_level_warn' },
				{ value: 'error', label: 'u095query.value_level_error' }
			]
		}
	]
};

export interface QueryValueOption {
	value: string;
	label: string;
}

export type QuerySuggestions = Partial<Record<QueryField, QueryValueOption[]>>;

export interface CompletionOption {
	insert: string;
	label: string;
	hint: string;
}

export interface Completion {
	start: number;
	end: number;
	options: CompletionOption[];
}

const FIELDS: readonly QueryField[] = [
	'name',
	'type',
	'size',
	'modified',
	'owner',
	'storage',
	'server',
	'split',
	'locked',
	'path',
	'user',
	'op',
	'status',
	'level'
];
const COMPARATOR_PREFIXES: readonly Comparator[] = ['>=', '<=', '>', '<'];
const COMPARABLE_FIELDS: readonly QueryField[] = ['size', 'modified', 'status'];
const SIZE_MULTIPLIERS = new Map<string, number>([
	['', 1],
	['b', 1],
	['k', 1024],
	['kb', 1024],
	['kib', 1024],
	['m', 1024 ** 2],
	['mb', 1024 ** 2],
	['mib', 1024 ** 2],
	['g', 1024 ** 3],
	['gb', 1024 ** 3],
	['gib', 1024 ** 3],
	['t', 1024 ** 4],
	['tb', 1024 ** 4],
	['tib', 1024 ** 4]
]);
const SIZE_LADDER: readonly { unit: string; multiplier: number }[] = [
	{ unit: 'TB', multiplier: 1024 ** 4 },
	{ unit: 'GB', multiplier: 1024 ** 3 },
	{ unit: 'MB', multiplier: 1024 ** 2 },
	{ unit: 'KB', multiplier: 1024 },
	{ unit: 'B', multiplier: 1 }
];
const DURATION_MILLISECONDS = new Map<string, number>([
	['s', 1000],
	['h', 3600000],
	['d', 86400000],
	['w', 604800000]
]);
const DAY_MILLISECONDS = 86400000;
const LEVELS = new Map<string, QueryLevel>([
	['debug', 'debug'],
	['info', 'info'],
	['warn', 'warn'],
	['warning', 'warn'],
	['error', 'error'],
	['err', 'error']
]);
const BOOLEANS = new Map<string, boolean>([
	['yes', true],
	['true', true],
	['1', true],
	['no', false],
	['false', false],
	['0', false]
]);
const TYPES = new Map<string, 'file' | 'directory'>([
	['file', 'file'],
	['directory', 'directory'],
	['folder', 'directory'],
	['dir', 'directory']
]);

interface ScannedToken {
	start: number;
	end: number;
	raw: string;
	negated: boolean;
	key: string | null;
	value: string;
	unterminated: boolean;
}

function isSpace(character: string): boolean {
	return /\s/.test(character);
}

function scanToken(text: string, start: number): ScannedToken {
	const negated = '!' === text.charAt(start);
	const wordStart = negated ? start + 1 : start;
	let cursor = wordStart;
	while (/[a-z]/i.test(text.charAt(cursor))) {
		cursor += 1;
	}
	const hasKey = cursor > wordStart && ':' === text.charAt(cursor);
	const key = hasKey ? text.slice(wordStart, cursor) : null;
	const valueStart = hasKey ? cursor + 1 : wordStart;
	let value: string;
	let end: number;
	let unterminated = false;
	if ('"' === text.charAt(valueStart)) {
		const closing = text.indexOf('"', valueStart + 1);
		if (-1 === closing) {
			unterminated = true;
			value = text.slice(valueStart + 1);
			end = text.length;
		} else {
			value = text.slice(valueStart + 1, closing);
			end = closing + 1;
		}
	} else {
		end = valueStart;
		while (end < text.length && !isSpace(text.charAt(end))) {
			end += 1;
		}
		value = text.slice(valueStart, end);
	}
	return { start, end, raw: text.slice(start, end), negated, key, value, unterminated };
}

function fieldOf(key: string): QueryField | null {
	const lower = key.toLowerCase();
	return FIELDS.find((field) => field === lower) ?? null;
}

function splitComparator(field: QueryTokenField, value: string): { comparator: Comparator; value: string } {
	if ('text' !== field && COMPARABLE_FIELDS.includes(field)) {
		const prefix = COMPARATOR_PREFIXES.find((candidate) => value.startsWith(candidate));
		if (undefined !== prefix) {
			return { comparator: prefix, value: value.slice(prefix.length) };
		}
	}
	return { comparator: ':', value };
}

function isoOrNull(milliseconds: number): string | null {
	const date = new Date(milliseconds);
	return Number.isNaN(date.getTime()) ? null : date.toISOString();
}

function joinWords(current: string | undefined, word: string): string {
	return undefined === current || '' === current ? word : `${current} ${word}`;
}

function parseSize(text: string): number | null {
	const match = /^(\d+(?:\.\d+)?)([a-z]*)$/i.exec(text);
	if (null === match) {
		return null;
	}
	const [, amount = '', unit = ''] = match;
	const multiplier = SIZE_MULTIPLIERS.get(unit.toLowerCase());
	if (undefined === multiplier) {
		return null;
	}
	return Math.round(Number(amount) * multiplier);
}

function assignSize(target: SearchQuery, comparator: Comparator, value: string): QueryErrorReason | null {
	const separator = value.indexOf('..');
	if (-1 !== separator) {
		if (':' !== comparator) {
			return 'bad_value';
		}
		const low = parseSize(value.slice(0, separator));
		const high = parseSize(value.slice(separator + 2));
		if (null === low || null === high) {
			return 'bad_size';
		}
		if (low > high) {
			return 'bad_range';
		}
		target.minSize = low;
		target.maxSize = high;
		return null;
	}
	const bytes = parseSize(value);
	if (null === bytes) {
		return 'bad_size';
	}
	if (':' === comparator) {
		target.minSize = bytes;
		target.maxSize = bytes;
	} else if ('>=' === comparator) {
		target.minSize = bytes;
	} else if ('>' === comparator) {
		target.minSize = bytes + 1;
	} else if ('<=' === comparator) {
		target.maxSize = bytes;
	} else {
		target.maxSize = bytes - 1;
	}
	return null;
}

function assignModified(target: SearchQuery, comparator: Comparator, value: string, now: number): QueryErrorReason | null {
	const span = /^(\d+)([shdw])$/i.exec(value);
	if (null !== span) {
		const [, count = '0', unit = ''] = span;
		const boundary = isoOrNull(now - Number(count) * (DURATION_MILLISECONDS.get(unit.toLowerCase()) ?? 0));
		if (null === boundary) {
			return 'bad_date';
		}
		if ('>' === comparator || '>=' === comparator) {
			target.modifiedBefore = boundary;
		} else {
			target.modifiedAfter = boundary;
		}
		return null;
	}
	const calendar = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
	if (null === calendar) {
		return 'bad_date';
	}
	const [, year = '', month = '', day = ''] = calendar;
	const startMilliseconds = Date.UTC(Number(year), Number(month) - 1, Number(day));
	const dayStart = isoOrNull(startMilliseconds);
	const dayEnd = isoOrNull(startMilliseconds + DAY_MILLISECONDS);
	if (null === dayStart || null === dayEnd || dayStart.slice(0, 10) !== `${year}-${month}-${day}`) {
		return 'bad_date';
	}
	if (':' === comparator) {
		target.modifiedAfter = dayStart;
		target.modifiedBefore = dayEnd;
	} else if ('>' === comparator || '>=' === comparator) {
		target.modifiedAfter = dayStart;
	} else if ('<' === comparator) {
		target.modifiedBefore = dayStart;
	} else {
		target.modifiedBefore = dayEnd;
	}
	return null;
}

function assign(target: SearchQuery, field: QueryTokenField, comparator: Comparator, value: string, now: number): QueryErrorReason | null {
	switch (field) {
		case 'text':
			target.text = joinWords(target.text, value);
			return null;
		case 'name':
			target.name = joinWords(target.name, value);
			return null;
		case 'type': {
			const kind = TYPES.get(value.toLowerCase());
			if (undefined === kind) {
				return 'bad_value';
			}
			target.type = kind;
			return null;
		}
		case 'size':
			return assignSize(target, comparator, value);
		case 'modified':
			return assignModified(target, comparator, value, now);
		case 'owner':
			target.owner = value;
			return null;
		case 'storage':
			target.storage = value.toLowerCase();
			return null;
		case 'server':
			target.server = value;
			return null;
		case 'split': {
			const flag = BOOLEANS.get(value.toLowerCase());
			if (undefined === flag) {
				return 'bad_value';
			}
			target.split = flag;
			return null;
		}
		case 'locked': {
			const flag = BOOLEANS.get(value.toLowerCase());
			if (undefined === flag) {
				return 'bad_value';
			}
			target.locked = flag;
			return null;
		}
		case 'path':
			target.path = value;
			return null;
		case 'user':
			target.user = value;
			return null;
		case 'op':
			target.action = value.toLowerCase();
			return null;
		case 'status': {
			if (!/^\d{3}$/.test(value)) {
				return 'bad_status';
			}
			const code = Number(value);
			if (100 > code || 599 < code) {
				return 'bad_status';
			}
			target.status = { comparator, code };
			return null;
		}
		case 'level': {
			const level = LEVELS.get(value.toLowerCase());
			if (undefined === level) {
				return 'bad_value';
			}
			target.level = level;
			return null;
		}
	}
}

function errorOf(scanned: ScannedToken, reason: QueryErrorReason): QueryError {
	return { reason, raw: scanned.raw, start: scanned.start, end: scanned.end };
}

function interpret(query: SearchQuery, scanned: ScannedToken, now: number): QueryToken | QueryError | null {
	if (scanned.unterminated) {
		return errorOf(scanned, 'unterminated_quote');
	}
	const field: QueryTokenField | null = null === scanned.key ? 'text' : fieldOf(scanned.key);
	if (null === field) {
		return errorOf(scanned, 'unknown_field');
	}
	if ('text' === field && '' === scanned.value) {
		return null;
	}
	const { comparator, value } = splitComparator(field, scanned.value);
	if ('' === value) {
		return errorOf(scanned, 'empty_value');
	}
	const target: SearchQuery = scanned.negated ? { exclude: [] } : query;
	const reason = assign(target, field, comparator, value, now);
	if (null !== reason) {
		return errorOf(scanned, reason);
	}
	return {
		field,
		comparator,
		value,
		negated: scanned.negated,
		raw: scanned.raw,
		start: scanned.start,
		end: scanned.end
	};
}

export function parse(text: string, now: number = Date.now()): ParseResult {
	const tokens: QueryToken[] = [];
	const errors: QueryError[] = [];
	const query: SearchQuery = { exclude: [] };
	let cursor = 0;
	while (cursor < text.length) {
		if (isSpace(text.charAt(cursor))) {
			cursor += 1;
			continue;
		}
		const scanned = scanToken(text, cursor);
		cursor = scanned.end;
		const outcome = interpret(query, scanned, now);
		if (null === outcome) {
			continue;
		}
		if ('reason' in outcome) {
			errors.push(outcome);
			continue;
		}
		tokens.push(outcome);
		if (outcome.negated) {
			query.exclude.push(outcome);
		}
	}
	return { tokens, errors, query };
}

export function quoteValue(value: string): string {
	return '' === value || /[\s"]/.test(value) ? `"${value}"` : value;
}

function comparatorPrefix(comparator: Comparator): string {
	return ':' === comparator ? '' : comparator;
}

function bytesText(bytes: number): string {
	const rung = SIZE_LADDER.find((candidate) => 0 === bytes % candidate.multiplier) ?? { unit: 'B', multiplier: 1 };
	return `${bytes / rung.multiplier}${rung.unit}`;
}

function sizeParts(query: SearchQuery): string[] {
	const low = query.minSize;
	const high = query.maxSize;
	if (undefined !== low && low === high) {
		return [`size:${bytesText(low)}`];
	}
	if (undefined !== low && undefined !== high) {
		return [`size:${bytesText(low)}..${bytesText(high)}`];
	}
	if (undefined !== low) {
		return [`size:>=${bytesText(low)}`];
	}
	if (undefined !== high) {
		return [`size:<=${bytesText(high)}`];
	}
	return [];
}

function tokenText(token: QueryToken): string {
	if ('text' === token.field) {
		return quoteValue(token.value);
	}
	return `${token.field}:${comparatorPrefix(token.comparator)}${quoteValue(token.value)}`;
}

export function format(query: SearchQuery): string {
	const parts: string[] = [];
	if (undefined !== query.name && '' !== query.name) {
		parts.push(`name:${quoteValue(query.name)}`);
	}
	if (undefined !== query.text && '' !== query.text) {
		parts.push(quoteValue(query.text));
	}
	if (undefined !== query.type) {
		parts.push(`type:${query.type}`);
	}
	parts.push(...sizeParts(query));
	if (undefined !== query.modifiedAfter) {
		parts.push(`modified:>${query.modifiedAfter.slice(0, 10)}`);
	}
	if (undefined !== query.modifiedBefore) {
		parts.push(`modified:<${query.modifiedBefore.slice(0, 10)}`);
	}
	if (undefined !== query.owner && '' !== query.owner) {
		parts.push(`owner:${quoteValue(query.owner)}`);
	}
	if (undefined !== query.storage && '' !== query.storage) {
		parts.push(`storage:${quoteValue(query.storage)}`);
	}
	if (undefined !== query.server && '' !== query.server) {
		parts.push(`server:${quoteValue(query.server)}`);
	}
	if (undefined !== query.split) {
		parts.push(`split:${query.split ? 'yes' : 'no'}`);
	}
	if (undefined !== query.locked) {
		parts.push(`locked:${query.locked ? 'yes' : 'no'}`);
	}
	if (undefined !== query.path && '' !== query.path) {
		parts.push(`path:${quoteValue(query.path)}`);
	}
	if (undefined !== query.user && '' !== query.user) {
		parts.push(`user:${quoteValue(query.user)}`);
	}
	if (undefined !== query.action && '' !== query.action) {
		parts.push(`op:${quoteValue(query.action)}`);
	}
	if (undefined !== query.status) {
		parts.push(`status:${comparatorPrefix(query.status.comparator)}${query.status.code}`);
	}
	if (undefined !== query.level) {
		parts.push(`level:${query.level}`);
	}
	for (const token of query.exclude) {
		parts.push(`!${tokenText(token)}`);
	}
	return parts.join(' ');
}

export function removeRange(text: string, start: number, end: number): string {
	return [text.slice(0, start).trimEnd(), text.slice(end).trimStart()].filter(Boolean).join(' ');
}

export function completionAt(
	text: string,
	caret: number,
	grammar: QueryGrammar,
	suggestions: QuerySuggestions
): Completion | null {
	const result = parse(text);
	const span = [...result.tokens, ...result.errors].find((item) => item.start <= caret && caret <= item.end);
	const start = span?.start ?? caret;
	const end = span?.end ?? caret;
	const match = /^(!?)([a-z]*)(?::(.*))?$/i.exec(text.slice(start, caret));
	if (null === match) {
		return null;
	}
	const [, negation = '', partial = '', afterColon] = match;
	if (undefined === afterColon) {
		const options = grammar.fields
			.filter((definition) => definition.key.startsWith(partial.toLowerCase()))
			.map((definition) => ({ insert: `${negation}${definition.key}:`, label: definition.label, hint: definition.hint }));
		return { start, end, options };
	}
	const definition = grammar.fields.find((entry) => entry.key === partial.toLowerCase());
	if (undefined === definition) {
		return { start, end, options: [] };
	}
	const valuePartial = afterColon.replace(/^"/, '').toLowerCase();
	const options = (suggestions[definition.key] ?? [])
		.filter((option) => option.value.toLowerCase().startsWith(valuePartial))
		.map((option) => ({
			insert: `${negation}${definition.key}:${quoteValue(option.value)}`,
			label: option.label,
			hint: ''
		}));
	return { start, end, options };
}
