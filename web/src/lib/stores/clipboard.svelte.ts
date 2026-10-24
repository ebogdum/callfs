export interface FilesClipboard {
	readonly mode: 'cut';
	readonly paths: readonly string[];
}

class ClipboardStore {
	value = $state<FilesClipboard | null>(null);

	cut(paths: readonly string[]): void {
		this.value = 0 === paths.length ? null : { mode: 'cut', paths: [...paths] };
	}

	clear(): void {
		this.value = null;
	}
}

export const filesClipboard = new ClipboardStore();
