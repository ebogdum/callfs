import { describe, expect, it } from 'vitest';
import { filesClipboard } from './clipboard.svelte';

describe('filesClipboard', () => {
	it('copies cut paths and clears on empty input or clear', () => {
		const paths = ['/a/one.txt', '/a/two.txt'];
		filesClipboard.cut(paths);
		expect(filesClipboard.value).toEqual({ mode: 'cut', paths });
		expect(filesClipboard.value?.paths).not.toBe(paths);
		filesClipboard.cut([]);
		expect(filesClipboard.value).toBeNull();
		filesClipboard.cut(paths);
		filesClipboard.clear();
		expect(filesClipboard.value).toBeNull();
	});
});
