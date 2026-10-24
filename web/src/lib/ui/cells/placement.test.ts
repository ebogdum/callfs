import { cleanup, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { t } from '$lib/copy/t';
import ModeCell from './ModeCell.svelte';
import OwnerCell from './OwnerCell.svelte';
import ServerCell from './ServerCell.svelte';
import StorageCell from './StorageCell.svelte';

afterEach(() => {
	cleanup();
});

describe('OwnerCell', () => {
	it('renders the user ID beside an avatar', () => {
		const { getByText, container } = render(OwnerCell, { props: { value: 'user-1024' } });
		expect(getByText('user-1024')).toBeTruthy();
		expect(container.querySelector('[data-state]')?.getAttribute('data-state')).toBe('set');
	});

	it('renders the no-owner warning word when the owner is empty', () => {
		const { getByText, container } = render(OwnerCell, { props: { value: '' } });
		expect(getByText(t('status.file.no_owner'))).toBeTruthy();
		expect(container.querySelector('[data-state]')?.getAttribute('data-state')).toBe('empty');
	});

	it('passes class and rest attributes to the root element', () => {
		const { container } = render(OwnerCell, {
			props: { value: 'user-1024', class: 'extra', 'data-testid': 'owner' },
		});
		const root = container.querySelector('[data-testid="owner"]');
		expect(root?.classList.contains('owner-cell')).toBe(true);
		expect(root?.classList.contains('extra')).toBe(true);
	});
});

describe('ModeCell', () => {
	it('shows the octal mode and puts the symbolic form in title and hidden text', () => {
		const { getByText, container } = render(ModeCell, { props: { value: '0644' } });
		const root = container.querySelector('.mode-cell');
		expect(getByText('0644')).toBeTruthy();
		expect(root?.getAttribute('title')).toBe('rw-r--r--');
		expect(getByText('rw-r--r--')).toBeTruthy();
	});

	it('uses the invalid-mode copy when the value is not an octal mode', () => {
		const { container } = render(ModeCell, { props: { value: '9x' } });
		const root = container.querySelector('.mode-cell');
		expect(root?.getAttribute('title')).toBe(t('placement.mode.invalid'));
	});

	it('passes class and rest attributes to the root element', () => {
		const { container } = render(ModeCell, {
			props: { value: '0755', class: 'extra', 'data-testid': 'mode' },
		});
		const root = container.querySelector('[data-testid="mode"]');
		expect(root?.classList.contains('mode-cell')).toBe(true);
		expect(root?.classList.contains('extra')).toBe(true);
	});
});

describe('StorageCell', () => {
	it.each([
		['localfs', 'Local disk'],
		['s3', 'S3 bucket'],
		['erasure', 'Split storage'],
		['internalproxy', 'Other server'],
		['some-other', 'Other server'],
	])('renders the plain word for %s', (value, word) => {
		const { getByText, container } = render(StorageCell, { props: { value } });
		expect(getByText(word)).toBeTruthy();
		expect(container.querySelector('.storage-cell')?.getAttribute('data-kind')).toBe(value);
	});

	it('passes class and rest attributes to the root element', () => {
		const { container } = render(StorageCell, {
			props: { value: 's3', class: 'extra', 'data-testid': 'storage' },
		});
		const root = container.querySelector('[data-testid="storage"]');
		expect(root?.classList.contains('storage-cell')).toBe(true);
		expect(root?.classList.contains('extra')).toBe(true);
	});
});

describe('ServerCell', () => {
	it('shows the server ID and the reachable word for a reachable server', () => {
		const { getByText, container } = render(ServerCell, {
			props: { id: 'callfs-instance-1', reachable: true },
		});
		expect(getByText('callfs-instance-1')).toBeTruthy();
		expect(getByText(t('status.server.up'))).toBeTruthy();
		expect(container.querySelector('.server-cell')?.getAttribute('data-state')).toBe('up');
	});

	it('shows the unreachable word for an unreachable server', () => {
		const { getByText, container } = render(ServerCell, {
			props: { id: 'callfs-instance-2', reachable: false },
		});
		expect(getByText(t('status.server.down'))).toBeTruthy();
		expect(container.querySelector('.server-cell')?.getAttribute('data-state')).toBe('down');
	});

	it('shows the not-checked word when reachability is unknown', () => {
		const { getByText, container } = render(ServerCell, { props: { id: 'callfs-instance-3' } });
		expect(getByText(t('status.server.unknown'))).toBeTruthy();
		expect(container.querySelector('.server-cell')?.getAttribute('data-state')).toBe('unknown');
	});

	it('shows the this-server badge only when self is set', () => {
		const { queryByText, rerender } = render(ServerCell, {
			props: { id: 'callfs-instance-1', self: true, reachable: true },
		});
		expect(queryByText(t('status.server.self'))).toBeTruthy();
		rerender({ id: 'callfs-instance-1', self: false, reachable: true });
		expect(queryByText(t('status.server.self'))).toBeNull();
	});

	it('passes class and rest attributes to the root element', () => {
		const { container } = render(ServerCell, {
			props: { id: 'callfs-instance-1', class: 'extra', 'data-testid': 'server' },
		});
		const root = container.querySelector('[data-testid="server"]');
		expect(root?.classList.contains('server-cell')).toBe(true);
		expect(root?.classList.contains('extra')).toBe(true);
	});
});
