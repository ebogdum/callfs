import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import { formatBytes } from '$lib/format/bytes';
import DropZone from './DropZone.svelte';
import RangeInput from './RangeInput.svelte';
import ResizeHandle from './ResizeHandle.svelte';

afterEach(() => {
	cleanup();
});

const fileNamed = (name: string, type = '', size = 'x'): File => new File([size], name, { type });

const transferOf = (files: File[], directories = 0): DataTransfer =>
	({
		types: ['Files'],
		files,
		items: [
			...files.map((file) => ({
				kind: 'file',
				getAsFile: () => file,
				webkitGetAsEntry: () => ({ isDirectory: false }),
			})),
			...Array.from({ length: directories }, () => ({
				kind: 'file',
				getAsFile: () => null,
				webkitGetAsEntry: () => ({ isDirectory: true }),
			})),
		],
	}) as unknown as DataTransfer;

const pickers = (): [HTMLInputElement, HTMLInputElement] => {
	const [filePicker, folderPicker] = Array.from(document.querySelectorAll<HTMLInputElement>('input[type="file"]'));
	return [filePicker, folderPicker];
};

const dropZoneProps = {
	label: 'Upload to /team',
	hint: 'Up to 2 GB per file',
};

describe('DropZone', () => {
	it('names the region by its label and shows both pick buttons', () => {
		render(DropZone, { props: { ...dropZoneProps, onfiles: vi.fn() } });
		expect(screen.getByRole('region', { name: 'Upload to /team' })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('u082molecules.drop.choose_files') })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('u082molecules.drop.choose_folder') })).toBeTruthy();
	});

	it('passes files from the file picker with folders false', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, onfiles } });
		const file = fileNamed('report.pdf', 'application/pdf');
		const [filePicker] = pickers();
		await fireEvent.change(filePicker, { target: { files: [file] } });
		expect(onfiles).toHaveBeenCalledWith([file], false);
	});

	it('passes files from the folder picker with folders true', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, onfiles } });
		const file = fileNamed('photos/a.png', 'image/png');
		const [, folderPicker] = pickers();
		expect(folderPicker.hasAttribute('webkitdirectory')).toBe(true);
		await fireEvent.change(folderPicker, { target: { files: [file] } });
		expect(onfiles).toHaveBeenCalledWith([file], true);
	});

	it('rejects files outside accept, adds none and names the reason', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, accept: ['.pdf'], onfiles } });
		const [filePicker] = pickers();
		await fireEvent.change(filePicker, { target: { files: [fileNamed('virus.exe')] } });
		expect(onfiles).not.toHaveBeenCalled();
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		expect(region.getAttribute('data-state')).toBe('reject');
		expect(screen.getByRole('status').textContent?.trim()).toBe(
			t('u082molecules.drop.rejected', { count: 1, reason: t('u082molecules.drop.reason_type') }),
		);
	});

	it('rejects files over maxBytes and names the largest allowed size', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, maxBytes: 3, onfiles } });
		const [filePicker] = pickers();
		await fireEvent.change(filePicker, { target: { files: [fileNamed('big.bin', '', 'abcde')] } });
		expect(onfiles).not.toHaveBeenCalled();
		expect(screen.getByRole('status').textContent?.trim()).toBe(
			t('u082molecules.drop.rejected', {
				count: 1,
				reason: t('u082molecules.drop.reason_size', { max: formatBytes(3).text }),
			}),
		);
	});

	it('reports the ready count when every dropped file is accepted', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, accept: ['.pdf'], onfiles } });
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		const first = fileNamed('a.pdf', 'application/pdf');
		const second = fileNamed('b.pdf', 'application/pdf');
		await fireEvent.drop(region, { dataTransfer: transferOf([first, second]) });
		expect(onfiles).toHaveBeenCalledWith([first, second], false);
		expect(region.getAttribute('data-state')).toBe('accept');
		expect(screen.getByRole('status').textContent?.trim()).toBe(t('u082molecules.drop.ready', { count: 2 }));
	});

	it('passes only accepted files from a drop and marks the rest as rejected', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, accept: ['image/*'], onfiles } });
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		const picture = fileNamed('a.png', 'image/png');
		const note = fileNamed('b.txt', 'text/plain');
		await fireEvent.drop(region, { dataTransfer: transferOf([picture, note]) });
		expect(onfiles).toHaveBeenCalledWith([picture], false);
		expect(region.getAttribute('data-state')).toBe('reject');
	});

	it('reports folders from a drop with folders true', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, onfiles } });
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		const file = fileNamed('inside.txt', 'text/plain');
		await fireEvent.drop(region, { dataTransfer: transferOf([file], 1) });
		expect(onfiles).toHaveBeenCalledWith([file], true);
	});

	it('shows the drop highlight while files are dragged over and clears it on leave', async () => {
		render(DropZone, { props: { ...dropZoneProps, onfiles: vi.fn() } });
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		await fireEvent.dragEnter(region, { dataTransfer: transferOf([]) });
		expect(region.getAttribute('data-state')).toBe('dragging');
		expect(screen.getByText(t('u082molecules.drop.dragging'))).toBeTruthy();
		await fireEvent.dragLeave(region);
		expect(region.getAttribute('data-state')).toBe('idle');
	});

	it('accepts pasted files with folders false', async () => {
		const onfiles = vi.fn();
		render(DropZone, { props: { ...dropZoneProps, onfiles } });
		const region = screen.getByRole('region', { name: 'Upload to /team' });
		const file = fileNamed('clip.png', 'image/png');
		await fireEvent.paste(region, { clipboardData: transferOf([file]) });
		expect(onfiles).toHaveBeenCalledWith([file], false);
	});

	it('renders extra content from the children snippet', () => {
		const note = createRawSnippet(() => ({ render: () => '<span>Extra note</span>' }));
		render(DropZone, { props: { ...dropZoneProps, onfiles: vi.fn(), children: note } });
		expect(screen.getByText('Extra note')).toBeTruthy();
	});
});

describe('ResizeHandle', () => {
	it('exposes a labelled vertical separator with its value range', () => {
		render(ResizeHandle, {
			props: { orientation: 'vertical', value: 200, min: 100, max: 400, label: 'Resize detail' },
		});
		const handle = screen.getByRole('separator', { name: 'Resize detail' });
		expect(handle.getAttribute('aria-orientation')).toBe('vertical');
		expect(handle.getAttribute('aria-valuenow')).toBe('200');
		expect(handle.getAttribute('aria-valuemin')).toBe('100');
		expect(handle.getAttribute('aria-valuemax')).toBe('400');
		expect(handle.getAttribute('tabindex')).toBe('0');
	});

	it('moves 16px per arrow key and stops at the range edges', async () => {
		render(ResizeHandle, {
			props: { orientation: 'vertical', value: 200, min: 100, max: 400, label: 'Resize detail' },
		});
		const handle = screen.getByRole('separator', { name: 'Resize detail' });
		await fireEvent.keyDown(handle, { key: 'ArrowRight' });
		expect(handle.getAttribute('aria-valuenow')).toBe('216');
		await fireEvent.keyDown(handle, { key: 'ArrowLeft' });
		await fireEvent.keyDown(handle, { key: 'ArrowLeft' });
		expect(handle.getAttribute('aria-valuenow')).toBe('184');
		await fireEvent.keyDown(handle, { key: 'End' });
		await fireEvent.keyDown(handle, { key: 'ArrowRight' });
		expect(handle.getAttribute('aria-valuenow')).toBe('400');
	});

	it('jumps to the minimum on Home', async () => {
		render(ResizeHandle, {
			props: { orientation: 'horizontal', value: 200, min: 100, max: 400, label: 'Resize list' },
		});
		const handle = screen.getByRole('separator', { name: 'Resize list' });
		await fireEvent.keyDown(handle, { key: 'Home' });
		expect(handle.getAttribute('aria-valuenow')).toBe('100');
	});

	it('follows the pointer while dragged and stops after pointer up', async () => {
		render(ResizeHandle, {
			props: { orientation: 'vertical', value: 200, min: 100, max: 400, label: 'Resize detail' },
		});
		const handle = screen.getByRole('separator', { name: 'Resize detail' });
		handle.dispatchEvent(new MouseEvent('pointerdown', { clientX: 200, bubbles: true }));
		await tick();
		expect(handle.getAttribute('data-state')).toBe('dragging');
		window.dispatchEvent(new MouseEvent('pointermove', { clientX: 230 }));
		await tick();
		expect(handle.getAttribute('aria-valuenow')).toBe('230');
		window.dispatchEvent(new MouseEvent('pointerup'));
		await tick();
		expect(handle.getAttribute('data-state')).toBe('idle');
		window.dispatchEvent(new MouseEvent('pointermove', { clientX: 300 }));
		await tick();
		expect(handle.getAttribute('aria-valuenow')).toBe('230');
	});

	it('drags a horizontal separator along the vertical pointer axis', async () => {
		render(ResizeHandle, {
			props: { orientation: 'horizontal', value: 200, min: 100, max: 400, label: 'Resize list' },
		});
		const handle = screen.getByRole('separator', { name: 'Resize list' });
		handle.dispatchEvent(new MouseEvent('pointerdown', { clientY: 50, bubbles: true }));
		window.dispatchEvent(new MouseEvent('pointermove', { clientY: 70 }));
		await tick();
		expect(handle.getAttribute('aria-valuenow')).toBe('220');
		window.dispatchEvent(new MouseEvent('pointerup'));
	});
});

describe('RangeInput', () => {
	const size = 5_000_000;

	it('groups the fields under its label', () => {
		render(RangeInput, { props: { size, from: 0, to: null, label: 'Download range' } });
		expect(screen.getByRole('group', { name: 'Download range' })).toBeTruthy();
	});

	it('shows both byte fields with help text when the range is valid', () => {
		render(RangeInput, { props: { size, from: 0, to: null, label: 'Download range' } });
		const from = screen.getByLabelText(t('u082molecules.range.from')) as HTMLInputElement;
		const to = screen.getByLabelText(t('u082molecules.range.to')) as HTMLInputElement;
		expect(from.value).toBe('0');
		expect(to.value).toBe('');
		expect(from.getAttribute('aria-invalid')).toBeNull();
		expect(screen.getByText(t('u082molecules.range.help'))).toBeTruthy();
	});

	it('fills the first megabyte when the First 1 MB preset is chosen', async () => {
		render(RangeInput, { props: { size, from: 0, to: null, label: 'Download range' } });
		await fireEvent.change(screen.getByRole('combobox', { name: t('u082molecules.range.preset') }), {
			target: { value: 'first' },
		});
		expect((screen.getByLabelText(t('u082molecules.range.from')) as HTMLInputElement).value).toBe('0');
		expect((screen.getByLabelText(t('u082molecules.range.to')) as HTMLInputElement).value).toBe('999999');
	});

	it('fills the last megabyte when the Last 1 MB preset is chosen', async () => {
		render(RangeInput, { props: { size, from: 0, to: null, label: 'Download range' } });
		await fireEvent.change(screen.getByRole('combobox', { name: t('u082molecules.range.preset') }), {
			target: { value: 'last' },
		});
		expect((screen.getByLabelText(t('u082molecules.range.from')) as HTMLInputElement).value).toBe('4000000');
		expect((screen.getByLabelText(t('u082molecules.range.to')) as HTMLInputElement).value).toBe('4999999');
	});

	it('flags a start past the end of the file with the size it must fit in', () => {
		render(RangeInput, { props: { size: 1000, from: 2000, to: null, label: 'Download range' } });
		const from = screen.getByLabelText(t('u082molecules.range.from'));
		expect(from.getAttribute('aria-invalid')).toBe('true');
		expect(from.getAttribute('aria-describedby')).toContain('error');
		const error = document.getElementById(from.getAttribute('aria-describedby') ?? '');
		expect(error?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
			t('u082molecules.range.outside', { size: formatBytes(1000).text }).replace(/\s+/g, ' '),
		);
	});

	it('flags an end before the start', () => {
		render(RangeInput, { props: { size, from: 500, to: 100, label: 'Download range' } });
		expect(screen.getByLabelText(t('u082molecules.range.to')).getAttribute('aria-invalid')).toBe('true');
		expect(screen.getByText(t('u082molecules.range.order'))).toBeTruthy();
	});
});
