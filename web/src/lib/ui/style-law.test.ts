import { existsSync, type Dirent, readdirSync, readFileSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

type Declaration = { property: string; value: string };
type Rule = { selector: string; media: string[]; declarations: Declaration[] };
type Frame = { prelude: string; rule: Rule | null };

const SOURCE_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const TOKENS_PREFIX = 'lib/ui/quarks/tokens/';
const STYLE_BLOCK = /<style\b[^>]*>([\s\S]*?)<\/style>/g;
const COMMENT = /\/\*[\s\S]*?\*\//g;
const VAR_NAME = /var\(\s*--[\w-]+/g;
const MARGIN_PROPERTY = /^margin(-|$)/;
const ALLOWED_MARGINS: ReadonlySet<string> = new Set([
	'margin: 0',
	'margin-block: 0',
	'margin-inline: 0',
	'margin-inline: auto',
]);
const RAW_COLOR = /#[0-9a-f]{3,8}(?![\w-])|\b(?:rgba?|hsla?|oklch)\(/i;
const RAW_LENGTH = /(?<![\w.#])[+-]?(?:\d+\.?\d*|\.\d+)(?:px|rem|em|ms|s|vh|vw|vmin|vmax|ch|ex|pt)(?![\w%-])/g;
const INTEGER_LITERAL = /^[+-]?\d+$/;
const DISABLED_SELECTOR = /:disabled|\[disabled\]|\[aria-disabled/;
const OUTLINE_PROPERTIES: readonly string[] = ['outline', 'outline-style'];
const NO_OUTLINE_VALUES: readonly string[] = ['none', '0'];
const HOVER_MEDIA = /\(\s*hover\s*:\s*hover\s*\)/;

const toPosix = (path: string): string => path.split(sep).join('/');

const listFiles = (directory: string): string[] => {
	if (false === existsSync(directory)) return [];
	return readdirSync(directory, { withFileTypes: true }).flatMap((entry: Dirent) => {
		const entryPath = join(directory, entry.name);
		return entry.isDirectory() ? listFiles(entryPath) : [entryPath];
	});
};

const parseStylesheet = (source: string): Rule[] => {
	const text = source.replace(COMMENT, '');
	const rules: Rule[] = [];
	const frames: Frame[] = [];
	let buffer = '';

	const enclosingMedia = (): string[] =>
		frames.flatMap((frame) => (null === frame.rule && frame.prelude.startsWith('@media') ? [frame.prelude] : []));

	const closeStatement = (): void => {
		const statement = buffer.trim();
		buffer = '';
		const frame = frames[frames.length - 1];
		const colon = statement.indexOf(':');
		if (undefined === frame || null === frame.rule || 0 > colon) return;
		const property = statement.slice(0, colon).trim().toLowerCase();
		if (false === /^[a-z-]+$/.test(property)) return;
		frame.rule.declarations.push({ property, value: statement.slice(colon + 1).trim() });
	};

	for (const character of text) {
		if ('{' === character) {
			const prelude = buffer.trim();
			buffer = '';
			if (prelude.startsWith('@')) {
				frames.push({ prelude, rule: null });
			} else {
				const rule: Rule = { selector: prelude, media: enclosingMedia(), declarations: [] };
				rules.push(rule);
				frames.push({ prelude, rule });
			}
		} else if (';' === character) {
			closeStatement();
		} else if ('}' === character) {
			closeStatement();
			frames.pop();
		} else {
			buffer += character;
		}
	}
	return rules;
};

const ruleViolations = (rule: Rule): string[] => {
	const violations: string[] = [];
	if (rule.selector.includes(':hover') && false === rule.media.some((media) => HOVER_MEDIA.test(media))) {
		violations.push(`:hover outside @media (hover: hover): ${rule.selector}`);
	}
	const hasFocusRing = rule.declarations.some(
		({ property, value }) => 'box-shadow' === property && value.includes('var(--focus-ring)'),
	);
	const disabledRule = DISABLED_SELECTOR.test(rule.selector);
	for (const { property, value } of rule.declarations) {
		if (MARGIN_PROPERTY.test(property) && false === ALLOWED_MARGINS.has(`${property}: ${value}`)) {
			violations.push(`margin not allowed: ${property}: ${value}`);
		}
		if (RAW_COLOR.test(value)) violations.push(`raw color: ${property}: ${value}`);
		for (const literal of value.replace(VAR_NAME, 'var(').matchAll(RAW_LENGTH)) {
			violations.push(`raw length ${literal[0]} in ${property}: ${rule.selector}`);
		}
		if ('z-index' === property && INTEGER_LITERAL.test(value)) {
			violations.push(`literal z-index: ${value}`);
		}
		if (disabledRule && 'opacity' === property) {
			violations.push(`opacity on disabled selector: ${rule.selector}`);
		}
		if (
			OUTLINE_PROPERTIES.includes(property) &&
			NO_OUTLINE_VALUES.includes(value.split(/\s+/)[0] ?? '') &&
			false === hasFocusRing
		) {
			violations.push(`outline removed without var(--focus-ring) box-shadow: ${rule.selector}`);
		}
	}
	return violations;
};

const stylesheetViolations = (source: string): string[] => parseStylesheet(source).flatMap(ruleViolations);

const collectViolations = (): string[] =>
	listFiles(SOURCE_ROOT).flatMap((file) => {
		const rel = toPosix(relative(SOURCE_ROOT, file));
		if (file.endsWith('.svelte')) {
			return [...readFileSync(file, 'utf8').matchAll(STYLE_BLOCK)].flatMap((match) =>
				stylesheetViolations(match[1] ?? '').map((violation) => `${rel}: ${violation}`),
			);
		}
		if (file.endsWith('.css') && false === rel.startsWith(TOKENS_PREFIX)) {
			return stylesheetViolations(readFileSync(file, 'utf8')).map((violation) => `${rel}: ${violation}`);
		}
		return [];
	});

describe('style law', () => {
	it('keeps component and global styles on tokens, spacing, focus and hover rules of sections 3 and 8.3', () => {
		expect(collectViolations()).toEqual([]);
	});
});
