import { describe, expect, it } from 'vitest';
import { GLOSSARY } from './glossary';
import { fleschKincaid, sentences, syllables, words, wordsPerSentence } from './readability';
import { t } from './t';

type AreaModule = { default: Record<string, string> };

interface CopyEntry {
	key: string;
	value: string;
	plain: string;
}

const areaModules: Record<string, unknown> = import.meta.glob('./en/*.ts', { eager: true });

const INNERMOST_BRACES_PATTERN = /\{[^{}]*\}/g;
const BANNED_WORDS_PATTERN =
	/\b(?:please|oops|whoops|click here|seamless|leverage|simply|just|metadata|inode|shard|parity|blob)\b/i;
const CASE_SENSITIVE_TECHNICAL_PATTERN = /\b(?:UID|GID)\b/;
const STRICT_PREFIXES = ['errors.', 'confirm.', 'empty.', 'login.', 'firstrun.'];
const MAX_WORDS_PER_SENTENCE = 25;
const MIN_GRADED_WORDS = 4;

function isAreaModule(module: unknown): module is AreaModule {
	if ('object' !== typeof module || null === module || !('default' in module)) {
		return false;
	}
	const record = module.default;
	if ('object' !== typeof record || null === record) {
		return false;
	}
	return Object.values(record).every((value) => 'string' === typeof value);
}

function areaOf(path: string): string {
	return path.slice('./en/'.length, -'.ts'.length);
}

function plainText(message: string): string {
	return message.replace(INNERMOST_BRACES_PATTERN, 'x').replace(INNERMOST_BRACES_PATTERN, 'x');
}

function maxGrade(key: string): number {
	return STRICT_PREFIXES.some((prefix) => key.startsWith(prefix)) ? 6 : 8;
}

const entries: CopyEntry[] = Object.entries(areaModules).flatMap(([, module]) => {
	if (!isAreaModule(module)) {
		return [];
	}
	return Object.entries(module.default).map(([key, value]) => ({
		key,
		value,
		plain: plainText(value)
	}));
});

describe('copy files', () => {
	it('export flat records of non-empty strings with keys prefixed by the file name', () => {
		const violations = Object.entries(areaModules).flatMap(([path, module]) => {
			if (!isAreaModule(module)) {
				return [`${path}: not a flat string record`];
			}
			const area = areaOf(path);
			return Object.entries(module.default)
				.filter(([key, value]) => !key.startsWith(`${area}.`) || 0 === value.trim().length)
				.map(([key]) => `${path}: ${key}`);
		});
		expect(violations).toEqual([]);
	});

	it('keeps every sentence at 25 words or fewer', () => {
		const violations = entries
			.filter((entry) =>
				sentences(entry.plain).some(
					(sentence) => MAX_WORDS_PER_SENTENCE < words(sentence).length
				)
			)
			.map((entry) => entry.key);
		expect(violations).toEqual([]);
	});

	it('uses no banned words or phrases outside technical keys', () => {
		const violations = entries
			.filter(
				(entry) =>
					BANNED_WORDS_PATTERN.test(entry.plain) ||
					'something went wrong' === entry.plain.trim().toLowerCase().replace(/[.!]$/, '') ||
					(!entry.key.startsWith('technical.') &&
						CASE_SENSITIVE_TECHNICAL_PATTERN.test(entry.plain))
			)
			.map((entry) => entry.key);
		expect(violations).toEqual([]);
	});

	it('keeps copy of four or more words within the grade limit (6 for errors, confirm, empty, login, firstrun; 8 otherwise)', () => {
		const violations = entries
			.filter(
				(entry) =>
					MIN_GRADED_WORDS <= words(entry.plain).length &&
					fleschKincaid(entry.plain) > maxGrade(entry.key)
			)
			.map((entry) => `${entry.key}: ${fleschKincaid(entry.plain).toFixed(1)}`);
		expect(violations).toEqual([]);
	});
});

describe('t', () => {
	it('interpolates named parameters', () => {
		expect(t('errors.not_found.title', { object: 'Report.pdf' })).toBe('Report.pdf is gone.');
	});

	it('selects the plural branch from the count', () => {
		expect(t('common.file_count', { count: 1 })).toBe('1 file');
		expect(t('common.file_count', { count: 3 })).toBe('3 files');
	});

	it('returns the key when no copy exists', () => {
		expect(t('missing.copy_key')).toBe('missing.copy_key');
	});
});

describe('GLOSSARY', () => {
	it('defines a word and a plain meaning for every concept', () => {
		const incomplete = Object.entries(GLOSSARY)
			.filter(([, entry]) => 0 === entry.word.length || 0 === entry.means.length)
			.map(([key]) => key);
		expect(incomplete).toEqual([]);
	});
});

describe('readability', () => {
	it('counts syllables', () => {
		expect([syllables('table'), syllables('make'), syllables('copy'), syllables('the')]).toEqual([
			2, 1, 2, 1
		]);
	});

	it('computes words per sentence', () => {
		expect(wordsPerSentence('One two. Three four five.')).toBe(2.5);
	});

	it('computes the Flesch-Kincaid grade', () => {
		expect(fleschKincaid('The cat sat.')).toBeCloseTo(-2.62, 2);
	});
});
