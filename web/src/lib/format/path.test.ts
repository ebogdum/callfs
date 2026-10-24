import { describe, expect, it } from 'vitest';
import {
	baseName,
	extension,
	isHidden,
	isSystemPath,
	joinPath,
	normalizePath,
	parentPath,
	prefixBoundary,
	segments,
	validatePath
} from './path';

describe('validatePath', () => {
	it('accepts files, folders, the root and paths without a leading slash', () => {
		expect(validatePath('/team/report.pdf')).toEqual({ ok: true });
		expect(validatePath('team/report.pdf')).toEqual({ ok: true });
		expect(validatePath('/team/')).toEqual({ ok: true });
		expect(validatePath('/')).toEqual({ ok: true });
		expect(validatePath('')).toEqual({ ok: true });
	});

	it('rejects backslashes', () => {
		expect(validatePath('/a\\b')).toEqual({ ok: false, error: 'path.error.backslash' });
	});

	it('rejects NUL characters', () => {
		expect(validatePath('/a\u0000b')).toEqual({ ok: false, error: 'path.error.nul' });
	});

	it('rejects parent segments', () => {
		expect(validatePath('/a/../b')).toEqual({ ok: false, error: 'path.error.parent_segment' });
		expect(validatePath('..')).toEqual({ ok: false, error: 'path.error.parent_segment' });
	});

	it('rejects current-directory segments', () => {
		expect(validatePath('/a/./b')).toEqual({ ok: false, error: 'path.error.current_segment' });
	});

	it('rejects empty segments in the middle of a path', () => {
		expect(validatePath('/a//b')).toEqual({ ok: false, error: 'path.error.empty_segment' });
	});
});

describe('normalizePath', () => {
	it('adds a leading slash and trims whitespace', () => {
		expect(normalizePath('team/a')).toBe('/team/a');
		expect(normalizePath('  /x  ')).toBe('/x');
	});

	it('maps an empty input to the root', () => {
		expect(normalizePath('')).toBe('/');
	});
});

describe('joinPath', () => {
	it('joins a name onto a folder', () => {
		expect(joinPath('/team', 'report.pdf')).toBe('/team/report.pdf');
		expect(joinPath('/team/', 'report.pdf')).toBe('/team/report.pdf');
	});

	it('joins onto the root without a double slash', () => {
		expect(joinPath('/', 'a')).toBe('/a');
	});

	it('keeps a trailing slash for folders', () => {
		expect(joinPath('/team/', '/x/')).toBe('/team/x/');
	});
});

describe('parentPath', () => {
	it('returns the containing folder', () => {
		expect(parentPath('/team/report.pdf')).toBe('/team');
		expect(parentPath('/team/')).toBe('/');
	});

	it('returns the root for top-level paths and for the root', () => {
		expect(parentPath('/team')).toBe('/');
		expect(parentPath('/')).toBe('/');
	});
});

describe('baseName', () => {
	it('returns the last name without a trailing slash', () => {
		expect(baseName('/team/report.pdf')).toBe('report.pdf');
		expect(baseName('/team/docs/')).toBe('docs');
	});

	it('returns an empty name for the root', () => {
		expect(baseName('/')).toBe('');
	});
});

describe('extension', () => {
	it('returns the lowercase last extension without the dot', () => {
		expect(extension('/a/Archive.TAR.GZ')).toBe('gz');
	});

	it('returns an empty extension for dotfiles and names without a dot', () => {
		expect(extension('/.env')).toBe('');
		expect(extension('/README')).toBe('');
	});
});

describe('segments', () => {
	it('returns each folder level with its cumulative path', () => {
		expect(segments('/a/b/c')).toEqual([
			{ label: 'a', path: '/a' },
			{ label: 'b', path: '/a/b' },
			{ label: 'c', path: '/a/b/c' }
		]);
	});

	it('returns no segments for the root', () => {
		expect(segments('/')).toEqual([]);
	});
});

describe('isHidden', () => {
	it('is true when any segment starts with a dot', () => {
		expect(isHidden('/a/.cache/x')).toBe(true);
		expect(isHidden('/.env')).toBe(true);
	});

	it('is false when no segment starts with a dot', () => {
		expect(isHidden('/a/b.txt')).toBe(false);
	});
});

describe('isSystemPath', () => {
	it('is true for the system folder and anything inside it', () => {
		expect(isSystemPath('/.callfs')).toBe(true);
		expect(isSystemPath('/.callfs/')).toBe(true);
		expect(isSystemPath('/.callfs/admin/x.json')).toBe(true);
	});

	it('is false for names that only start the same way', () => {
		expect(isSystemPath('/.callfsx')).toBe(false);
		expect(isSystemPath('/a/.callfs')).toBe(false);
	});
});

describe('prefixBoundary', () => {
	it('adds a trailing slash so a prefix never matches a sibling with the same start', () => {
		expect(prefixBoundary('/team')).toBe('/team/');
		expect(prefixBoundary('/team/')).toBe('/team/');
	});

	it('keeps the root as a single slash', () => {
		expect(prefixBoundary('/')).toBe('/');
	});
});
