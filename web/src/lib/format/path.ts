export type PathCheck = { ok: true } | { ok: false; error: string };

export interface PathSegment {
	label: string;
	path: string;
}

const SYSTEM_ROOT = '/.callfs';

function withoutFolderMarks(path: string): string {
	const withoutLeadingSlash = path.startsWith('/') ? path.slice(1) : path;
	return withoutLeadingSlash.endsWith('/') ? withoutLeadingSlash.slice(0, -1) : withoutLeadingSlash;
}

function segmentError(segment: string): string | null {
	if ('' === segment) return 'path.error.empty_segment';
	if ('..' === segment) return 'path.error.parent_segment';
	if ('.' === segment) return 'path.error.current_segment';
	return null;
}

function namesOf(path: string): string[] {
	return normalizePath(path)
		.split('/')
		.filter((name) => '' !== name);
}

export function validatePath(path: string): PathCheck {
	if (path.includes('\\')) return { ok: false, error: 'path.error.backslash' };
	if (path.includes('\u0000')) return { ok: false, error: 'path.error.nul' };
	const body = withoutFolderMarks(path);
	if ('' === body) return { ok: true };
	for (const segment of body.split('/')) {
		const error = segmentError(segment);
		if (error) return { ok: false, error };
	}
	return { ok: true };
}

export function normalizePath(path: string): string {
	const trimmed = path.trim();
	return '/' === trimmed.charAt(0) ? trimmed : `/${trimmed}`;
}

export function joinPath(base: string, name: string): string {
	const head = normalizePath(base).replace(/\/+$/, '');
	return `${head}/${name.replace(/^\/+/, '')}`;
}

export function parentPath(path: string): string {
	return `/${namesOf(path).slice(0, -1).join('/')}`;
}

export function baseName(path: string): string {
	const names = namesOf(path);
	return names[names.length - 1] ?? '';
}

export function extension(path: string): string {
	const name = baseName(path);
	const dot = name.lastIndexOf('.');
	return 0 < dot ? name.slice(dot + 1).toLowerCase() : '';
}

export function segments(path: string): PathSegment[] {
	const names = namesOf(path);
	return names.map((label, index) => ({
		label,
		path: `/${names.slice(0, index + 1).join('/')}`
	}));
}

export function isHidden(path: string): boolean {
	return namesOf(path).some((name) => name.startsWith('.'));
}

export function isSystemPath(path: string): boolean {
	const normalized = normalizePath(path);
	return SYSTEM_ROOT === normalized || normalized.startsWith(`${SYSTEM_ROOT}/`);
}

export function prefixBoundary(prefix: string): string {
	const normalized = normalizePath(prefix);
	return normalized.endsWith('/') ? normalized : `${normalized}/`;
}
