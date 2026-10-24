import { extension } from './path';

export type FileKind =
	| 'text'
	| 'code'
	| 'json'
	| 'log'
	| 'csv'
	| 'image'
	| 'video'
	| 'audio'
	| 'pdf'
	| 'archive'
	| 'binary';

export type FileIconName =
	| 'file'
	| 'file-text'
	| 'file-code'
	| 'file-json'
	| 'file-image'
	| 'file-video'
	| 'file-audio'
	| 'file-archive'
	| 'file-pdf'
	| 'table';

const EXTENSIONS: [FileKind, string[]][] = [
	['text', ['txt', 'md', 'markdown', 'rtf']],
	[
		'code',
		[
			'js', 'mjs', 'cjs', 'ts', 'jsx', 'tsx', 'svelte', 'vue', 'go', 'py', 'rb', 'rs', 'java', 'kt',
			'c', 'h', 'cc', 'cpp', 'hpp', 'cs', 'php', 'swift', 'sh', 'bash', 'zsh', 'sql', 'html', 'htm',
			'css', 'scss', 'less', 'xml', 'yaml', 'yml', 'toml', 'ini', 'env', 'proto'
		]
	],
	['json', ['json', 'jsonl', 'ndjson', 'geojson']],
	['log', ['log']],
	['csv', ['csv', 'tsv']],
	['image', ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif', 'heic', 'ico', 'tif', 'tiff']],
	['video', ['mp4', 'm4v', 'webm', 'mov', 'mkv', 'avi', 'ogv']],
	['audio', ['mp3', 'wav', 'ogg', 'oga', 'flac', 'm4a', 'aac', 'opus']],
	['pdf', ['pdf']],
	['archive', ['zip', 'tar', 'gz', 'tgz', 'bz2', 'tbz2', 'xz', 'txz', 'zst', '7z', 'rar']]
];

const KIND_BY_EXTENSION = new Map<string, FileKind>(
	EXTENSIONS.flatMap(([kind, extensions]) => extensions.map((ext) => [ext, kind] as const))
);

const ICON_BY_KIND: Record<FileKind, FileIconName> = {
	text: 'file-text',
	code: 'file-code',
	json: 'file-json',
	log: 'file-text',
	csv: 'table',
	image: 'file-image',
	video: 'file-video',
	audio: 'file-audio',
	pdf: 'file-pdf',
	archive: 'file-archive',
	binary: 'file'
};

export function kindFromName(name: string): FileKind {
	return KIND_BY_EXTENSION.get(extension(name)) ?? 'binary';
}

export function iconForKind(kind: FileKind): FileIconName {
	return ICON_BY_KIND[kind];
}
