import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import HexViewer from './HexViewer.svelte';
import MediaViewer from './MediaViewer.svelte';
import PreviewPane, { type PreviewKind, type PreviewStat } from './PreviewPane.svelte';
import TextViewer, { type RangeLoader } from './TextViewer.svelte';

const textBuffer = (text: string): ArrayBuffer => new TextEncoder().encode(text).buffer as ArrayBuffer;

const bytesBuffer = (bytes: number[]): ArrayBuffer => new Uint8Array(bytes).buffer as ArrayBuffer;

const serve = (content: ArrayBuffer): RangeLoader =>
	vi.fn(async (from: number, to: number) => content.slice(from, to));

const notes: PreviewStat = { path: '/docs/notes.txt', name: 'notes.txt', type: 'file', size: 5 };

interface PaneOverrides {
	stat?: PreviewStat;
	kind?: PreviewKind;
	siblings?: PreviewStat[];
	onnavigate?: (path: string) => void;
}

const pane = (overrides: PaneOverrides = {}) =>
	render(PreviewPane, {
		props: {
			stat: notes,
			src: '/download/notes',
			loadRange: serve(textBuffer('hello')),
			...overrides
		}
	});

describe('PreviewPane', () => {
	it('names the pane after the file and links to the source and download', () => {
		pane();
		const region = screen.getByRole('region', { name: 'Preview of notes.txt' });
		expect(region).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute('download', 'notes.txt');
		expect(screen.getByRole('link', { name: 'Download' })).toHaveAttribute('href', '/download/notes');
	});

	it('renders the text viewer for text files', async () => {
		const { container } = pane();
		expect(container.querySelector('.text-viewer')).not.toBeNull();
		expect(await screen.findByText('hello')).toBeInTheDocument();
	});

	it('renders an image with the file name as alt text', () => {
		pane({ stat: { ...notes, name: 'photo.png', path: '/docs/photo.png' }, kind: 'image' });
		expect(screen.getByRole('img', { name: 'photo.png' })).toBeInTheDocument();
	});

	it('renders a PDF object with a fallback link', () => {
		const { container } = pane({ stat: { ...notes, name: 'manual.pdf', path: '/docs/manual.pdf' }, kind: 'pdf' });
		expect(container.querySelector('object[type="application/pdf"]')).not.toBeNull();
		expect(screen.getByText('This browser cannot show this PDF here.')).toBeInTheDocument();
	});

	it('shows the unsupported copy and a download link for archives', () => {
		pane({ stat: { ...notes, name: 'backup.zip', path: '/docs/backup.zip' }, kind: 'archive' });
		expect(screen.getByText('No preview for this file type')).toBeInTheDocument();
		const links = screen.getAllByRole('link', { name: 'Download' });
		expect(links).toHaveLength(2);
		for (const link of links) expect(link).toHaveAttribute('download', 'backup.zip');
	});

	it('opens full screen with Space and closes with Escape', () => {
		pane();
		const region = screen.getByRole('region', { name: 'Preview of notes.txt' });
		fireEvent.keyDown(region, { key: ' ' });
		expect(region).toHaveAttribute('data-fullscreen', 'true');
		fireEvent.keyDown(region, { key: 'Escape' });
		expect(region).toHaveAttribute('data-fullscreen', 'false');
	});

	it('moves to the next sibling with the right arrow in full screen', () => {
		const onnavigate = vi.fn();
		const siblings: PreviewStat[] = [notes, { path: '/docs/b.txt', name: 'b.txt', type: 'file', size: 3 }];
		pane({ siblings, onnavigate });
		const region = screen.getByRole('region', { name: 'Preview of notes.txt' });
		fireEvent.keyDown(region, { key: ' ' });
		expect(screen.getByRole('navigation', { name: 'Files in this folder' })).toBeInTheDocument();
		fireEvent.keyDown(region, { key: 'ArrowRight' });
		expect(onnavigate).toHaveBeenCalledWith('/docs/b.txt');
	});
});

describe('TextViewer', () => {
	it('loads the first chunk and numbers the lines', async () => {
		const loadRange = serve(textBuffer('alpha\nbeta'));
		const { container } = render(TextViewer, { props: { size: 10, loadRange } });
		expect(await screen.findByText('alpha')).toBeInTheDocument();
		expect(loadRange).toHaveBeenCalledWith(0, 10);
		const gutters = Array.from(container.querySelectorAll('.gutter')).map((node) => node.textContent);
		expect(gutters).toEqual(['1', '2']);
	});

	it('shows the empty message for a zero-byte file without loading', () => {
		const loadRange = serve(textBuffer(''));
		render(TextViewer, { props: { size: 0, loadRange } });
		expect(screen.getByText('This file is empty.')).toBeInTheDocument();
		expect(loadRange).not.toHaveBeenCalled();
	});

	it('shows the error state and retries the failed range', async () => {
		const loadRange = vi
			.fn<RangeLoader>()
			.mockRejectedValueOnce(new Error('network'))
			.mockResolvedValue(textBuffer('alpha'));
		render(TextViewer, { props: { size: 5, loadRange } });
		expect(await screen.findByText('Part of this file did not load')).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
		expect(await screen.findByText('alpha')).toBeInTheDocument();
	});

	it('marks the viewer as wrapping when wrap is on', () => {
		const { container } = render(TextViewer, { props: { size: 5, loadRange: serve(textBuffer('hello')), wrap: true } });
		expect(container.querySelector('.text-viewer')).toHaveAttribute('data-wrap', 'true');
	});

	it('finds matches and reports the count', async () => {
		render(TextViewer, { props: { size: 24, loadRange: serve(textBuffer('one match\nno\nmatch again')) } });
		await screen.findByText('no');
		await fireEvent.input(screen.getByRole('searchbox', { name: 'Find in file' }), { target: { value: 'match' } });
		expect(await screen.findByText('2 matches')).toBeInTheDocument();
	});

	it('pretty prints JSON when the toggle is pressed', async () => {
		const loadRange = serve(textBuffer('{"a":1}'));
		render(TextViewer, { props: { size: 7, loadRange } });
		await screen.findByText('{"a":1}');
		await fireEvent.click(screen.getByRole('button', { name: 'Pretty JSON' }));
		expect(await screen.findByText('"a": 1')).toBeInTheDocument();
		expect(loadRange).toHaveBeenCalledWith(0, 7);
	});
});

describe('HexViewer', () => {
	it('renders offset, hex bytes and ASCII for a row', async () => {
		const loadRange = serve(bytesBuffer([0x48, 0x69, 0x21]));
		const { container } = render(HexViewer, { props: { size: 3, loadRange } });
		expect(await screen.findByText('Hi!')).toBeInTheDocument();
		const row = container.querySelector('.viewport .row');
		expect(row?.querySelector('.offset')?.textContent).toBe('00000000');
		expect(row?.querySelector('.bytes')?.textContent).toContain('48 69 21');
	});

	it('rejects an offset outside the file and marks the field invalid', async () => {
		render(HexViewer, { props: { size: 3, loadRange: serve(bytesBuffer([0x48, 0x69, 0x21])) } });
		await screen.findByText('Hi!');
		await fireEvent.input(screen.getByRole('textbox', { name: 'Go to offset' }), { target: { value: '0x10' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Go' }));
		expect(screen.getByText('Enter a byte offset from 0 to 2, such as 4096 or 0x1000.')).toBeInTheDocument();
		expect(screen.getByRole('textbox', { name: 'Go to offset' })).toHaveAttribute('aria-invalid', 'true');
	});

	it('accepts a decimal offset inside the file without an error', async () => {
		render(HexViewer, { props: { size: 3, loadRange: serve(bytesBuffer([0x48, 0x69, 0x21])) } });
		await screen.findByText('Hi!');
		await fireEvent.input(screen.getByRole('textbox', { name: 'Go to offset' }), { target: { value: '1' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Go' }));
		await waitFor(() => {
			expect(screen.queryByText(/Enter a byte offset/)).not.toBeInTheDocument();
		});
	});
});

describe('MediaViewer', () => {
	it('zooms the image in steps and shows the level', async () => {
		render(MediaViewer, { props: { kind: 'image', src: '/download/photo', alt: 'photo.png' } });
		expect(screen.getByText('Zoom 100%')).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
		expect(screen.getByText('Zoom 125%')).toBeInTheDocument();
	});

	it('renders video and audio players with accessible names', () => {
		const video = render(MediaViewer, { props: { kind: 'video', src: '/download/clip', alt: 'clip.mp4' } });
		expect(screen.getByLabelText('Video preview')).toHaveAttribute('controls');
		video.unmount();
		render(MediaViewer, { props: { kind: 'audio', src: '/download/song', alt: 'song.mp3' } });
		expect(screen.getByLabelText('Audio preview')).toHaveAttribute('controls');
	});
});
