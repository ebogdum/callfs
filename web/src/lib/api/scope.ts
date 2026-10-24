const NODES_PREFIX = '/v1/nodes';
const LOCAL_PREFIXES: readonly string[] = ['/v1/session', NODES_PREFIX];
const ME_PATH = '/v1/me';
const FORWARDABLE_PREFIXES: readonly string[] = ['/v1/', '/health'];

function pathnameOf(path: string): string {
	return path.split(/[?#]/, 1)[0] ?? '';
}

export function isLocalPath(path: string): boolean {
	const pathname = pathnameOf(path);
	if (ME_PATH === pathname || pathname.startsWith(`${ME_PATH}/`)) {
		return true;
	}
	return LOCAL_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

export function isForwardablePath(path: string): boolean {
	const pathname = pathnameOf(path);
	return FORWARDABLE_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

export function scopedPath(path: string, nodeId: string | null): string {
	if (null === nodeId || '' === nodeId || isLocalPath(path) || false === isForwardablePath(path)) {
		return path;
	}
	return `${NODES_PREFIX}/${encodeURIComponent(nodeId)}${path}`;
}
