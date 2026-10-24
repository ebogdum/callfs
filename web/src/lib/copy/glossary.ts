export interface GlossaryEntry {
	word: string;
	means: string;
	technical?: string;
}

export type GlossaryKey =
	| 'file'
	| 'folder'
	| 'path'
	| 'storage'
	| 'server'
	| 'cluster_member'
	| 'leader'
	| 'quorum'
	| 'file_index'
	| 'split_storage'
	| 'piece'
	| 'spare_pieces'
	| 'backup_copy'
	| 'version_tag'
	| 'edit_lock'
	| 'download_link'
	| 'access_key'
	| 'user'
	| 'owner'
	| 'admin'
	| 'request_limit'
	| 'index_cache'
	| 'finding'
	| 'job'
	| 'trash'
	| 'delete'
	| 'move_to_trash'
	| 'remove'
	| 'revoke';

export const GLOSSARY: Record<GlossaryKey, GlossaryEntry> = {
	file: { word: 'File', means: 'A stored file.' },
	folder: { word: 'Folder', means: 'A place that holds files and other folders.' },
	path: { word: 'Path', means: 'The full location of an item, like /team/report.pdf.' },
	storage: {
		word: 'Storage',
		means: 'Where the bytes live: Local disk or S3 bucket.',
		technical: 'backend'
	},
	server: {
		word: 'Server',
		means: 'One running callfs process, named by its server ID.',
		technical: 'instance_id'
	},
	cluster_member: {
		word: 'Cluster member',
		means: 'A server that keeps the shared file index.',
		technical: 'replica'
	},
	leader: { word: 'Leader', means: 'The cluster member that accepts changes right now.' },
	quorum: { word: 'Quorum', means: 'Enough cluster members agree to accept changes.' },
	file_index: {
		word: 'File index',
		means: 'The list of files and their details.',
		technical: 'metadata store'
	},
	split_storage: {
		word: 'Split storage',
		means: 'A file kept as data pieces plus spare pieces.',
		technical: 'erasure coding'
	},
	piece: {
		word: 'Piece',
		means: 'One part of a split storage file.',
		technical: 'shard'
	},
	spare_pieces: {
		word: 'Spare pieces',
		means: 'Extra pieces that rebuild lost ones.',
		technical: 'parity'
	},
	backup_copy: {
		word: 'Backup copy',
		means: 'A second copy of a file on another storage.',
		technical: 'replica'
	},
	version_tag: {
		word: 'Version tag',
		means: 'A fingerprint that changes when the content changes.',
		technical: 'ETag'
	},
	edit_lock: { word: 'Edit lock', means: 'A short lock while a file changes.' },
	download_link: {
		word: 'Download link',
		means: 'A link that works once and expires.',
		technical: 'presigned URL'
	},
	access_key: {
		word: 'Access key',
		means: 'The secret a person or app uses to sign in.',
		technical: 'API key'
	},
	user: { word: 'User', means: 'An app user who owns files. Never an OS account.' },
	owner: { word: 'Owner', means: 'The user who created a file.' },
	admin: { word: 'Admin', means: 'A user listed in auth.admin_users.' },
	request_limit: {
		word: 'Request limit',
		means: 'The most requests allowed per second.',
		technical: 'rate limit'
	},
	index_cache: {
		word: 'Index cache',
		means: 'A short-lived memory copy of file details.',
		technical: 'metadata cache'
	},
	finding: { word: 'Finding', means: 'Something that needs attention, with a fix.' },
	job: { word: 'Job', means: 'Long work that runs on the server, with progress.' },
	trash: { word: 'Trash', means: 'Deleted files kept for 30 days.' },
	delete: { word: 'Delete', means: 'Remove for good.' },
	move_to_trash: { word: 'Move to trash', means: 'Remove a file in a way you can undo.' },
	remove: { word: 'Remove', means: 'Take out of a set, such as a cluster member or a server address.' },
	revoke: { word: 'Revoke', means: 'Stop a download link before it is used.' }
};
