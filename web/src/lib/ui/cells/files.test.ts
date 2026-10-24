import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import ETagCell from './ETagCell.svelte';
import NameCell from './NameCell.svelte';
import PathCell from './PathCell.svelte';
import StatusCell from './StatusCell.svelte';

afterEach(() => {
	cleanup();
});

const renameInput = (container: HTMLElement): HTMLInputElement => {
	const input = container.querySelector<HTMLInputElement>('input.rename');
	expect(input).not.toBeNull();
	return input as HTMLInputElement;
};

describe('PathCell', () => {
	it('renders the path in mono with the full text on title and aria-label', () => {
		const { container } = render(PathCell, { props: { path: '/team/report.pdf' } });
		const truncated = container.querySelector('span.middle-truncate');
		expect(truncated?.textContent).toContain('/team/report.pdf');
		expect(truncated?.getAttribute('title')).toBe('/team/report.pdf');
		expect(truncated?.getAttribute('aria-label')).toBe('/team/report.pdf');
	});

	it('links the path when href is given', () => {
		const { container } = render(PathCell, { props: { path: '/team/report.pdf', href: '/files/team/report.pdf' } });
		expect(container.querySelector('a')?.getAttribute('href')).toBe('/files/team/report.pdf');
	});

	it('does not render a link without href', () => {
		const { container } = render(PathCell, { props: { path: '/team/report.pdf' } });
		expect(container.querySelector('a')).toBeNull();
	});

	it('marks the highlighted match with mark', () => {
		const { container } = render(PathCell, { props: { path: '/team/report-2024.pdf', highlight: '2024' } });
		const marks = container.querySelectorAll('mark');
		expect(marks).toHaveLength(1);
		expect(marks[0]?.textContent).toBe('2024');
	});

	it('matches the highlight without regard to case', () => {
		const { container } = render(PathCell, { props: { path: '/team/report.pdf', highlight: 'REPORT' } });
		expect(container.querySelector('mark')?.textContent).toBe('report');
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(PathCell, {
			props: { path: '/a', class: 'extra', 'data-testid': 'path' }
		});
		const root = container.querySelector('span.path-cell.extra');
		expect(root?.getAttribute('data-testid')).toBe('path');
	});
});

describe('NameCell', () => {
	it('renders the name and the kind on the root', () => {
		const { container } = render(NameCell, { props: { name: 'Reports', kind: 'folder' } });
		const root = container.querySelector('span.name-cell');
		expect(root?.getAttribute('data-kind')).toBe('folder');
		expect(root?.textContent).toContain('Reports');
	});

	it('links the name when href is given', () => {
		const { container } = render(NameCell, {
			props: { name: 'report.pdf', kind: 'file', href: '/files/report.pdf' }
		});
		expect(container.querySelector('a')?.getAttribute('href')).toBe('/files/report.pdf');
	});

	it('renders each badge with a visible tooltip word and a hidden word', () => {
		const { container } = render(NameCell, {
			props: { name: 'report.pdf', kind: 'file', badges: ['locked', 'link'] }
		});
		expect(container.querySelector('[data-badge="locked"]')?.textContent).toContain(t('status.file.locked'));
		expect(container.querySelector('[data-badge="link"]')?.textContent).toContain(t('cells.name.badge_link'));
		expect(container.querySelector('[data-badge="hidden"]')).toBeNull();
	});

	it('shows a rename input named with the file name and selects the base name', async () => {
		const { container } = render(NameCell, {
			props: { name: 'report-v2.pdf', kind: 'file', editing: true }
		});
		await tick();
		const input = container.querySelector('input.rename');
		expect(input?.getAttribute('aria-label')).toBe(t('cells.name.rename_label', { name: 'report-v2.pdf' }));
		expect(input?.getAttribute('aria-label')).toContain('report-v2.pdf');
		expect(container.querySelector('a')).toBeNull();
		expect(input).toBeInstanceOf(HTMLInputElement);
		if (input instanceof HTMLInputElement) {
			expect(input.selectionStart).toBe(0);
			expect(input.selectionEnd).toBe('report-v2'.length);
		}
	});

	it('calls onrename with the trimmed draft on Enter', async () => {
		const onrename = vi.fn();
		const { container } = render(NameCell, {
			props: { name: 'report-v2.pdf', kind: 'file', editing: true, onrename }
		});
		await tick();
		const input = renameInput(container);
		await fireEvent.input(input, { target: { value: '  report-v3.pdf ' } });
		await fireEvent.keyDown(input, { key: 'Enter' });
		expect(onrename).toHaveBeenCalledTimes(1);
		expect(onrename).toHaveBeenCalledWith('report-v3.pdf');
	});

	it('restores the name and calls oncancel on Escape without renaming', async () => {
		const onrename = vi.fn();
		const oncancel = vi.fn();
		const { container } = render(NameCell, {
			props: { name: 'report-v2.pdf', kind: 'file', editing: true, onrename, oncancel }
		});
		await tick();
		const input = renameInput(container);
		await fireEvent.input(input, { target: { value: 'other.pdf' } });
		await fireEvent.keyDown(input, { key: 'Escape' });
		expect(oncancel).toHaveBeenCalledTimes(1);
		expect(onrename).not.toHaveBeenCalled();
		expect(input.value).toBe('report-v2.pdf');
	});

	it('cancels instead of renaming when Enter is pressed on a blank name', async () => {
		const onrename = vi.fn();
		const oncancel = vi.fn();
		const { container } = render(NameCell, {
			props: { name: 'report-v2.pdf', kind: 'file', editing: true, onrename, oncancel }
		});
		await tick();
		const input = renameInput(container);
		await fireEvent.input(input, { target: { value: '   ' } });
		await fireEvent.keyDown(input, { key: 'Enter' });
		expect(onrename).not.toHaveBeenCalled();
		expect(oncancel).toHaveBeenCalledTimes(1);
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(NameCell, {
			props: { name: 'a.txt', kind: 'file', class: 'extra', 'data-testid': 'name' }
		});
		expect(container.querySelector('span.name-cell.extra')?.getAttribute('data-testid')).toBe('name');
	});
});

describe('StatusCell', () => {
	it('renders the catalog word for the status', () => {
		const { container } = render(StatusCell, { props: { status: 'file.locked' } });
		expect(container.querySelector('span.status-cell')?.textContent).toContain(t('status.file.locked'));
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(StatusCell, {
			props: { status: 'file.split', class: 'extra', 'data-testid': 'status' }
		});
		expect(container.querySelector('span.status-cell.extra')?.getAttribute('data-testid')).toBe('status');
	});
});

describe('ETagCell', () => {
	it('shows the first 12 characters followed by an ellipsis, with the full tag on title', () => {
		const { container } = render(ETagCell, { props: { value: 'abcdef1234567890ff' } });
		const value = container.querySelector('span.value');
		expect(value?.textContent).toBe('abcdef123456…');
		expect(value?.getAttribute('title')).toBe('abcdef1234567890ff');
	});

	it('shows a short tag without an ellipsis', () => {
		const { container } = render(ETagCell, { props: { value: 'abc123' } });
		expect(container.querySelector('span.value')?.textContent).toBe('abc123');
	});

	it('names the copy button with the visible label and the tag', () => {
		const { container } = render(ETagCell, { props: { value: 'abc123' } });
		const button = container.querySelector('button');
		expect(button?.getAttribute('aria-label')).toBe(t('cells.etag.copy'));
	});

	it('shows Not set with the warn tone when the tag is empty', () => {
		const { container } = render(ETagCell, { props: { value: '' } });
		const root = container.querySelector('span.etag-cell');
		expect(root?.getAttribute('data-tone')).toBe('warn');
		expect(root?.textContent).toContain(t('cells.etag.not_set'));
		expect(container.querySelector('button')).toBeNull();
	});

	it('forwards class and rest attributes to the root', () => {
		const { container } = render(ETagCell, {
			props: { value: 'abc123', class: 'extra', 'data-testid': 'etag' }
		});
		expect(container.querySelector('span.etag-cell.extra')?.getAttribute('data-testid')).toBe('etag');
	});
});
