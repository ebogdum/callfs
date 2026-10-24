import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import CodeCell from './CodeCell.svelte';
import IPCell from './IPCell.svelte';
import KeyCell from './KeyCell.svelte';
import PieceCell from './PieceCell.svelte';
import ValueCell from './ValueCell.svelte';

const DASH = '—';

class ResizeObserverStub {
	observe(): void {}
	unobserve(): void {}
	disconnect(): void {}
}

beforeEach(() => {
	vi.stubGlobal('ResizeObserver', ResizeObserverStub);
});

afterEach(() => {
	cleanup();
	vi.unstubAllGlobals();
	Reflect.deleteProperty(navigator, 'clipboard');
});

describe('KeyCell', () => {
	it('renders the setting key in mono with the full key as title and a named copy button', () => {
		const { container, getByRole } = render(KeyCell, { props: { value: 'server.max_upload_size' } });
		const key = container.querySelector('.middle-truncate');
		expect(key?.getAttribute('title')).toBe('server.max_upload_size');
		expect(getByRole('button', { name: t('u065cells.copy_key') })).not.toBeNull();
	});

	it('copies the key to the clipboard when the copy button is pressed', async () => {
		const writeText = vi.fn(() => Promise.resolve());
		Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
		const { getByRole } = render(KeyCell, { props: { value: 'server.max_upload_size' } });
		await fireEvent.click(getByRole('button', { name: t('u065cells.copy_key') }));
		expect(writeText).toHaveBeenCalledWith('server.max_upload_size');
	});

	it('passes class through to the root element', () => {
		const { container } = render(KeyCell, { props: { value: 'a.b', class: 'extra' } });
		expect(container.querySelector('.key-cell.extra')).not.toBeNull();
	});
});

describe('ValueCell', () => {
	it('shows a string value with the full text as title', () => {
		const { container } = render(ValueCell, { props: { value: 'localhost:4000' } });
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe('localhost:4000');
	});

	it('formats a number with the number formatter', () => {
		const { container } = render(ValueCell, { props: { value: 42 } });
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe('42');
	});

	it('renders booleans as Yes and No', () => {
		const yes = render(ValueCell, { props: { value: true } });
		expect(yes.container.querySelector('.middle-truncate')?.getAttribute('title')).toBe(t('u065cells.yes'));
		cleanup();
		const no = render(ValueCell, { props: { value: false } });
		expect(no.container.querySelector('.middle-truncate')?.getAttribute('title')).toBe(t('u065cells.no'));
	});

	it('marks an unset value as empty and shows Not set', () => {
		const { container } = render(ValueCell, { props: { value: null } });
		const root = container.querySelector('.value-cell');
		expect(root?.getAttribute('data-state')).toBe('empty');
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe(t('u065cells.not_set'));
	});

	it('shows only Set for a secret and never the secret text', () => {
		const { container } = render(ValueCell, { props: { value: 'hunter2-secret', secret: true } });
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe(t('u065cells.set'));
		expect(container.textContent).not.toContain('hunter2-secret');
	});

	it('shows Not set for an empty secret', () => {
		const { container } = render(ValueCell, { props: { value: '', secret: true } });
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe(t('u065cells.not_set'));
	});

	it('shows the configuration source word as a badge', () => {
		const { getByText } = render(ValueCell, { props: { value: 8080, source: 'env' } });
		expect(getByText(t('status.source.env'))).not.toBeNull();
	});
});

describe('CodeCell', () => {
	it('renders the value inside a code element', () => {
		const { container } = render(CodeCell, { props: { value: 'ls -la' } });
		expect(container.querySelector('code')?.textContent).toBe('ls -la');
	});

	it('exposes the full value as title for hover', () => {
		const { container } = render(CodeCell, { props: { value: 'ls -la' } });
		expect(container.querySelector('.code-cell')?.getAttribute('title')).toBe('ls -la');
	});
});

describe('IPCell', () => {
	it('renders an address in mono with the full address as title', () => {
		const { container } = render(IPCell, { props: { value: '10.0.0.12' } });
		expect(container.querySelector('.middle-truncate')?.getAttribute('title')).toBe('10.0.0.12');
	});

	it('renders a dash and a hidden Unknown word for a null address', () => {
		const { container, getByText } = render(IPCell, { props: { value: null } });
		expect(container.querySelector('.ip-cell')?.getAttribute('data-state')).toBe('empty');
		expect(container.querySelector('.ip-cell')?.textContent).toContain(DASH);
		expect(getByText(t('u065cells.unknown'))).not.toBeNull();
	});
});

describe('PieceCell', () => {
	it('renders the piece number, its kind and the status word', () => {
		const { container, getByText } = render(PieceCell, {
			props: { index: 3, kind: 'spare', state: 'piece.missing' }
		});
		expect(container.querySelector('.title')?.textContent).toBe('Piece 3 · Spare');
		expect(getByText(t('status.piece.missing'))).not.toBeNull();
	});

	it('renders a data piece with the Data kind word', () => {
		const { container } = render(PieceCell, {
			props: { index: 1, kind: 'data', state: 'piece.ok' }
		});
		expect(container.querySelector('.title')?.textContent).toBe('Piece 1 · Data');
	});
});
