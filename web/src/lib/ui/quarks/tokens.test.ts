import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { contrastRatio } from '../../theme/oklch';

type Scope = Map<string, string>;
type CssBlock = { selector: string; body: string };
type Source = [file: string, selector: string];

const TOLERANCE = 0.05;
const TOKEN_DIR = join(dirname(fileURLToPath(import.meta.url)), 'tokens');
const TOKEN_FILES = ['primitives.css', 'semantic.css', 'themes.css', 'density.css', 'motion.css', 'base.css'];
const VAR_REFERENCE = /var\(\s*(--[\w-]+)/g;
const DECLARED_NAME = /(?:^|[\s{;])(--[\w-]+)\s*:/g;
const DECLARATION = /^(--[\w-]+)\s*:\s*([\s\S]+)$/;
const SINGLE_VAR = /^var\((--[\w-]+)\)$/;

const LIGHT_SOURCES: Source[] = [
	['primitives.css', ':root'],
	['semantic.css', ':root']
];
const DARK_SOURCES: Source[] = [
	...LIGHT_SOURCES,
	['primitives.css', ":root[data-theme='dark']"],
	['semantic.css', ":root[data-theme='dark']"]
];
const MORE_LIGHT_SOURCES: Source[] = [...LIGHT_SOURCES, ['themes.css', ":root[data-contrast='more']"]];
const MORE_DARK_SOURCES: Source[] = [
	...DARK_SOURCES,
	['themes.css', ":root[data-theme='dark'][data-contrast='more']"]
];

const SEMANTIC_TOKENS = [
	'--bg',
	'--bg-subtle',
	'--surface',
	'--surface-raised',
	'--surface-sunken',
	'--surface-overlay',
	'--surface-hover',
	'--surface-pressed',
	'--surface-selected',
	'--border-subtle',
	'--border',
	'--border-strong',
	'--border-control',
	'--text',
	'--text-muted',
	'--text-faint',
	'--text-on-accent',
	'--text-link',
	'--accent',
	'--accent-hover',
	'--accent-pressed',
	'--accent-soft',
	'--accent-soft-hover',
	'--accent-border',
	'--accent-text',
	'--focus-ring-color',
	...['ok', 'warn', 'danger', 'info'].flatMap((status) => [
		`--${status}`,
		`--${status}-soft`,
		`--${status}-border`,
		`--${status}-text`
	]),
	'--scrim',
	'--shadow-color',
	'--code-bg',
	'--selection-bg',
	'--hairline',
	'--focus-width'
];

const DENSITY_TOKENS = [
	'--control-h',
	'--control-px',
	'--row-h',
	'--row-h-2line',
	'--cell-px',
	'--table-edge-px',
	'--card-pad',
	'--table-text',
	'--icon-size'
];

const MOTION_TOKENS = [
	'--dur-0',
	'--dur-micro',
	'--dur-small',
	'--dur-medium',
	'--dur-large',
	'--dur-exit-micro',
	'--dur-exit-small',
	'--dur-exit-medium',
	'--dur-exit-large',
	'--dur-fade',
	'--dur-shimmer',
	'--ease-out',
	'--ease-standard',
	'--ease-in-out',
	'--ease-in'
];

function readToken(file: string): string {
	return readFileSync(join(TOKEN_DIR, file), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
}

function topLevelBlocks(css: string): CssBlock[] {
	const blocks: CssBlock[] = [];
	let cursor = 0;
	while (cursor < css.length) {
		const open = css.indexOf('{', cursor);
		if (-1 === open) {
			break;
		}
		let depth = 1;
		let index = open + 1;
		while (0 < depth && index < css.length) {
			const character = css.charAt(index);
			if ('{' === character) {
				depth += 1;
			} else if ('}' === character) {
				depth -= 1;
			}
			index += 1;
		}
		const prelude = css.slice(cursor, open);
		blocks.push({
			selector: prelude
				.slice(prelude.lastIndexOf(';') + 1)
				.replace(/\s+/g, ' ')
				.trim(),
			body: css.slice(open + 1, index - 1)
		});
		cursor = index;
	}
	return blocks;
}

function scopeFrom(sources: Source[]): Scope {
	const scope: Scope = new Map();
	for (const [file, selector] of sources) {
		for (const block of topLevelBlocks(readToken(file))) {
			if (selector !== block.selector) {
				continue;
			}
			for (const declaration of block.body.split(';')) {
				const match = DECLARATION.exec(declaration.trim());
				if (null !== match) {
					scope.set(match[1] ?? '', (match[2] ?? '').trim());
				}
			}
		}
	}
	return scope;
}

function resolveValue(scope: Scope, name: string, depth = 0): string {
	const value = scope.get(name) ?? '';
	const reference = SINGLE_VAR.exec(value);
	if (null === reference || 8 < depth) {
		return value;
	}
	return resolveValue(scope, reference[1] ?? '', depth + 1);
}

function contrastOf(scope: Scope, foreground: string, background: string): number {
	const ratio = contrastRatio(resolveValue(scope, foreground), resolveValue(scope, background));
	expect(ratio.ok, `${foreground} on ${background} is not an opaque hex color`).toBe(true);
	return ratio.ok ? ratio.value : 0;
}

function neutralSteps(count: number): string[] {
	return Array.from({ length: count }, (_, index) => `--n-${index + 1}`);
}

function expectNear(actual: number, expected: number): void {
	expect(Math.abs(actual - expected)).toBeLessThanOrEqual(TOLERANCE);
}

function expectDescendingRange(ratios: number[], first: number, last: number): void {
	expectNear(ratios.at(0) ?? 0, first);
	expectNear(ratios.at(-1) ?? 0, last);
	const descending = ratios.slice(1).every((ratio, index) => (ratios[index] ?? 0) + TOLERANCE >= ratio);
	expect(descending).toBe(true);
}

function namesIn(pattern: RegExp, css: string): string[] {
	return [...css.matchAll(pattern)].map((match) => match[1] ?? '');
}

const LIGHT = scopeFrom(LIGHT_SOURCES);
const DARK = scopeFrom(DARK_SOURCES);
const MODE_SCOPES: Array<[string, Scope]> = [
	['light', LIGHT],
	['dark', DARK],
	['more-light', scopeFrom(MORE_LIGHT_SOURCES)],
	['more-dark', scopeFrom(MORE_DARK_SOURCES)]
];

describe('light neutral contrast (section 2.1)', () => {
	it.each([
		['--text', 5, [16.24, 15.61, 14.68, 13.88, 13.06]],
		['--text-muted', 5, [7.26, 7.0, 6.56, 6.18, 5.82]],
		['--text-faint', 3, [5.15, 4.93, 4.65]],
		['--border-control', 4, [3.55, 3.4, 3.21, 3.03]]
	])('%s on neutral steps matches the measured ratios', (foreground, count, expected) => {
		neutralSteps(count).forEach((background, index) => {
			expectNear(contrastOf(LIGHT, foreground, background), expected[index] ?? 0);
		});
	});
});

describe('dark neutral contrast (section 2.1)', () => {
	it.each([
		['--text', 17.15, 12.51],
		['--text-muted', 8.97, 6.52],
		['--text-faint', 6.31, 4.62],
		['--border-control', 4.22, 3.09]
	])('%s on n1..n6 spans the measured range', (foreground, first, last) => {
		const ratios = neutralSteps(6).map((background) => contrastOf(DARK, foreground, background));
		expectDescendingRange(ratios, first, last);
	});
});

describe('accent and status contrast (section 2.1)', () => {
	it('matches the measured accent pairs in light and dark', () => {
		expectNear(contrastOf(LIGHT, '--text-on-accent', '--accent'), 5.1);
		expectNear(contrastOf(LIGHT, '--accent-text', '--n-1'), 7.41);
		expectNear(contrastOf(DARK, '--text-on-accent', '--accent'), 9.52);
	});

	it.each(['ok', 'warn', 'danger', 'info'])('keeps the %s solid on surface at or above 5.51:1 in light', (status) => {
		expect(contrastOf(LIGHT, `--${status}`, '--surface')).toBeGreaterThanOrEqual(5.51 - TOLERANCE);
	});

	it.each(['ok', 'warn', 'danger', 'info'])('keeps the %s text on soft background above the section 2.1 floors', (status) => {
		expect(contrastOf(LIGHT, `--${status}-text`, `--${status}-soft`)).toBeGreaterThanOrEqual(8.27 - TOLERANCE);
		expect(contrastOf(DARK, `--${status}-text`, `--${status}-soft`)).toBeGreaterThanOrEqual(11.76 - TOLERANCE);
	});
});

describe('token presence', () => {
	it.each(MODE_SCOPES)('defines every semantic token in %s', (_mode, scope) => {
		expect(SEMANTIC_TOKENS.filter((name) => false === scope.has(name))).toEqual([]);
	});

	it.each(['default', 'compact', 'comfortable'])('defines every density token for data-density=%s', (density) => {
		const scope = scopeFrom([['density.css', 'default' === density ? ":root, html[data-density='default']" : `html[data-density='${density}']`]]);
		expect(DENSITY_TOKENS.filter((name) => false === scope.has(name))).toEqual([]);
	});

	it('defines every motion token', () => {
		const scope = scopeFrom([['motion.css', ':root']]);
		expect(MOTION_TOKENS.filter((name) => false === scope.has(name))).toEqual([]);
	});
});

describe('custom property references', () => {
	it('references only custom properties declared in the token files', () => {
		const css = TOKEN_FILES.map(readToken).join('\n');
		const declared = new Set(namesIn(DECLARED_NAME, css));
		const unknown = [...new Set(namesIn(VAR_REFERENCE, css))].filter((name) => false === declared.has(name));
		expect(unknown).toEqual([]);
	});
});
