import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

type ImportRecord = { specifier: string; typeOnly: boolean };
type Target = { kind: 'lib'; path: string } | { kind: 'package'; name: string } | { kind: 'other' };
type Owner = { libPath: string | null; level: string | null; route: boolean };

const SOURCE_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const LIB_ROOT = join(SOURCE_ROOT, 'lib');
const LEVELS: readonly string[] = [
	'quarks',
	'atoms',
	'cells',
	'molecules',
	'organisms',
	'environments',
	'planets',
	'systems',
	'galaxies',
	'clusters',
	'universes',
];
const PLANETS_RANK = LEVELS.indexOf('planets');
const BITS_UI_LEVELS: readonly string[] = ['quarks', 'atoms', 'cells', 'molecules', 'organisms'];
const PURE_LIBS: readonly string[] = ['format', 'copy', 'status', 'theme'];
const DATA_LIBS: readonly string[] = ['api', 'data', 'live', 'stores'];
const ICON_MODULE = 'ui/quarks/icons.ts';
const SOURCE_EXTENSIONS: readonly string[] = ['.svelte', '.ts', '.js'];
const IMPORT_FROM = /(?:^|[\s;{}])(?:import|export)\s+(type\s+)?([^'"`;]*?)\bfrom\s*['"]([^'"]+)['"]/g;
const IMPORT_BARE = /(?:^|[\s;{}])import\s*['"]([^'"]+)['"]/g;
const IMPORT_DYNAMIC = /\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)/g;

const toPosix = (path: string): string => path.split(sep).join('/');

const listFiles = (directory: string): string[] => {
	if (false === existsSync(directory)) return [];
	return readdirSync(directory, { withFileTypes: true }).flatMap((entry: { name: string; isDirectory: () => boolean }) => {
		const entryPath = join(directory, entry.name);
		return entry.isDirectory() ? listFiles(entryPath) : [entryPath];
	});
};

const isTypeOnlyClause = (clause: string): boolean => {
	const named = /\{([^}]*)\}/.exec(clause)?.[1];
	if (undefined === named) return false;
	const withoutNamed = clause.replace(/\{[^}]*\}/, '').replace(/[,\s]/g, '');
	const specifiers = named
		.split(',')
		.map((part) => part.trim())
		.filter((part) => '' !== part);
	return '' === withoutNamed && specifiers.length > 0 && specifiers.every((part) => part.startsWith('type '));
};

const extractImports = (source: string): ImportRecord[] => {
	const fromStatements = [...source.matchAll(IMPORT_FROM)].map((match) => ({
		specifier: match[3] ?? '',
		typeOnly: undefined !== match[1] || isTypeOnlyClause(match[2] ?? ''),
	}));
	const bareStatements = [...source.matchAll(IMPORT_BARE), ...source.matchAll(IMPORT_DYNAMIC)].map((match) => ({
		specifier: match[1] ?? '',
		typeOnly: false,
	}));
	return [...fromStatements, ...bareStatements].filter((record) => '' !== record.specifier);
};

const classify = (specifier: string, file: string): Target => {
	if (specifier.startsWith('.')) {
		const absolute = resolve(dirname(file), specifier);
		return absolute.startsWith(LIB_ROOT + sep)
			? { kind: 'lib', path: toPosix(relative(LIB_ROOT, absolute)) }
			: { kind: 'other' };
	}
	if (specifier.startsWith('$lib/')) return { kind: 'lib', path: specifier.slice('$lib/'.length) };
	if (specifier.startsWith('$')) return { kind: 'other' };
	const name = /^(@[^/]+\/[^/]+|[^/]+)/.exec(specifier)?.[1] ?? specifier;
	return { kind: 'package', name };
};

const ownerOf = (file: string): Owner => {
	const sourcePath = toPosix(relative(SOURCE_ROOT, file));
	const libPath = sourcePath.startsWith('lib/') ? sourcePath.slice('lib/'.length) : null;
	const level = null === libPath ? null : (LEVELS.find((name) => libPath.startsWith(`ui/${name}/`)) ?? null);
	return { libPath, level, route: sourcePath.startsWith('routes/') };
};

const uiGroupOf = (libPath: string | null): string | null => {
	if (null === libPath) return null;
	const segments = libPath.split('/');
	return 3 < segments.length ? (segments[2] ?? null) : '';
};

const isInList = (list: readonly string[], value: string | null): boolean => null !== value && list.includes(value);

const packageReasons = (name: string, owner: Owner): string[] => {
	if ('bits-ui' === name && false === isInList(BITS_UI_LEVELS, owner.level)) {
		return ['bits-ui is allowed only in quarks, atoms, molecules and organisms'];
	}
	if ('@lucide/svelte' === name && ICON_MODULE !== owner.libPath) {
		return ['@lucide/svelte is allowed only in ui/quarks/icons.ts'];
	}
	return [];
};

const uiReasons = (path: string, owner: Owner): string[] => {
	const targetLevel = path.split('/')[1] ?? '';
	if (false === isInList(LEVELS, targetLevel)) return [];
	const targetRank = LEVELS.indexOf(targetLevel);
	if (owner.route) {
		return PLANETS_RANK <= targetRank ? [] : ['routes import only planets and higher levels'];
	}
	if (null === owner.level) return [];
	const ownRank = LEVELS.indexOf(owner.level);
	if (ownRank < targetRank) return [`${owner.level} may import only lower levels`];
	if (ownRank > targetRank) return [];
	const group = uiGroupOf(owner.libPath);
	const targetGroup = uiGroupOf(path);
	if (null !== owner.libPath && owner.libPath.endsWith('.test.ts')) return [];
	return null !== group && ('' === group || '' === targetGroup || group === targetGroup) ? [] : ['same-level imports stay inside one folder group'];
};

const libReasons = (path: string, owner: Owner, typeOnly: boolean): string[] => {
	const head = path.split('/')[0] ?? '';
	if ('ui' === head) return uiReasons(path, owner);
	if (null === owner.level || PLANETS_RANK <= LEVELS.indexOf(owner.level)) return [];
	if (isInList(DATA_LIBS, head)) return typeOnly ? [] : ['data access (api, data, live, stores) starts at planets'];
	if ('commands' === head) return typeOnly ? [] : ['below planets $lib/commands may be imported only as types'];
	if (isInList(PURE_LIBS, head)) return [];
	return ['below planets only format, copy, status, theme and command types may be imported from lib'];
};

const importReasons = (file: string, record: ImportRecord): string[] => {
	const owner = ownerOf(file);
	const target = classify(record.specifier, file);
	if ('package' === target.kind) return packageReasons(target.name, owner);
	if ('other' === target.kind) return [];
	return libReasons(target.path, owner, record.typeOnly);
};

const collectViolations = (): string[] =>
	[...listFiles(join(LIB_ROOT, 'ui')), ...listFiles(join(SOURCE_ROOT, 'routes'))]
		.filter((file) => SOURCE_EXTENSIONS.some((extension) => file.endsWith(extension)))
		.flatMap((file) => {
			const source = readFileSync(file, 'utf8');
			return extractImports(source).flatMap((record) =>
				importReasons(file, record).map(
					(reason) => `${toPosix(relative(SOURCE_ROOT, file))} imports '${record.specifier}': ${reason}`,
				),
			);
		});

describe('architecture import law', () => {
	it('keeps every src/lib/ui and src/routes import inside the level rules of section 4.0', () => {
		expect(collectViolations()).toEqual([]);
	});
});
