import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { mount, unmount, type Component } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import KeyValueEditor from './KeyValueEditor.svelte';
import ListEditor from './ListEditor.svelte';
import OctalModeEditor from './OctalModeEditor.svelte';
import SecretInput from './SecretInput.svelte';

const mounted: Parameters<typeof unmount>[0][] = [];

function host(): HTMLElement {
	const element = document.createElement('div');
	document.body.append(element);
	return element;
}

function withBinding<P extends object, K extends keyof P & string>(
	props: P,
	key: K
): { props: P; cell: { current: P[K] } } {
	const cell = { current: props[key] };
	Object.defineProperty(props, key, {
		enumerable: true,
		configurable: true,
		get: () => cell.current,
		set: (next: P[K]) => {
			cell.current = next;
		}
	});
	return { props, cell };
}

function place<P extends object>(component: Component<P>, props: P): void {
	mounted.push(mount(component, { target: host(), props }));
}

afterEach(() => {
	for (const instance of mounted.splice(0)) {
		void unmount(instance);
	}
	cleanup();
	document.body.replaceChildren();
});

describe('SecretInput', () => {
	it('shows Not set and a password field with the label when no value is stored', () => {
		render(SecretInput, { props: { label: 'Access key', isSet: false } });
		expect(screen.getByText('Not set')).toBeTruthy();
		const field = screen.getByLabelText('Access key') as HTMLInputElement;
		expect(field.type).toBe('password');
	});

	it('hides the stored value and offers Replace named after the label', async () => {
		const onreplace = vi.fn();
		render(SecretInput, { props: { label: 'Access key', isSet: true, onreplace } });
		expect(screen.getByText('Set')).toBeTruthy();
		expect(screen.queryByLabelText('Access key')).toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: 'Replace Access key' }));
		expect(onreplace).toHaveBeenCalledTimes(1);
		expect(screen.getByLabelText('Access key')).toBeTruthy();
	});

	it('returns to the Set view when Cancel is pressed during replace', async () => {
		render(SecretInput, { props: { label: 'Access key', isSet: true } });
		await fireEvent.click(screen.getByRole('button', { name: 'Replace Access key' }));
		await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
		expect(screen.getByRole('button', { name: 'Replace Access key' })).toBeTruthy();
	});

	it('fills a 32 byte base64url secret on Generate', async () => {
		render(SecretInput, { props: { label: 'Access key', isSet: false } });
		await fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
		const field = screen.getByLabelText('Access key') as HTMLInputElement;
		expect(field.value).toMatch(/^[A-Za-z0-9_-]{43}$/);
	});

	it('toggles the visible text with a pressed Show button', async () => {
		render(SecretInput, { props: { label: 'Access key', isSet: false } });
		const toggle = screen.getByRole('button', { name: 'Show' });
		expect(toggle.getAttribute('aria-pressed')).toBe('false');
		await fireEvent.click(toggle);
		expect(toggle.getAttribute('aria-pressed')).toBe('true');
		expect((screen.getByLabelText('Access key') as HTMLInputElement).type).toBe('text');
	});

	it('marks a too short value invalid and names the minimum', async () => {
		render(SecretInput, { props: { label: 'Access key', isSet: false, minLength: 16 } });
		const field = screen.getByLabelText('Access key');
		await fireEvent.input(field, { target: { value: 'short' } });
		expect(screen.getByText('Use at least 16 characters.')).toBeTruthy();
		expect(field.getAttribute('aria-invalid')).toBe('true');
	});
});

describe('KeyValueEditor', () => {
	const labels = { keyLabel: 'Member ID', valueLabel: 'Address', addLabel: 'Add member' };

	it('lists each entry with named key and value fields and a Remove button', () => {
		const { props } = withBinding({ entries: { alpha: '10.0.0.1:9000' }, ...labels }, 'entries');
		place(KeyValueEditor, props);
		const key = screen.getByRole('textbox', { name: 'Member ID 1' }) as HTMLInputElement;
		const value = screen.getByRole('textbox', { name: 'Address 1' }) as HTMLInputElement;
		expect(key.value).toBe('alpha');
		expect(value.value).toBe('10.0.0.1:9000');
		expect(screen.getByRole('button', { name: 'Remove alpha' })).toBeTruthy();
	});

	it('removes an entry and writes the record back', async () => {
		const { props, cell } = withBinding({ entries: { alpha: '1', beta: '2' }, ...labels }, 'entries');
		place(KeyValueEditor, props);
		await fireEvent.click(screen.getByRole('button', { name: 'Remove alpha' }));
		expect(cell.current).toEqual({ beta: '2' });
	});

	it('adds a row and writes the new key', async () => {
		const { props, cell } = withBinding({ entries: {}, ...labels }, 'entries');
		place(KeyValueEditor, props);
		expect(screen.getByText('Nothing added yet.')).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: 'Add member' }));
		await fireEvent.input(screen.getByRole('textbox', { name: 'Member ID 1' }), { target: { value: 'gamma' } });
		expect(cell.current).toEqual({ gamma: '' });
	});

	it('shows a duplicate key error and keeps the last valid record', async () => {
		const { props, cell } = withBinding({ entries: { alpha: '1', beta: '2' }, ...labels }, 'entries');
		place(KeyValueEditor, props);
		const secondKey = screen.getByRole('textbox', { name: 'Member ID 2' });
		await fireEvent.input(secondKey, { target: { value: 'alpha' } });
		expect(screen.getAllByText('Key alpha is used twice. Keep one.')).toHaveLength(2);
		expect(secondKey.getAttribute('aria-invalid')).toBe('true');
		expect(cell.current).toEqual({ alpha: '1', beta: '2' });
	});
});

describe('ListEditor', () => {
	const labels = { label: 'Servers', itemLabel: 'Server', addLabel: 'Add server' };

	it('disables Move up on the first item with the reason', () => {
		const { props } = withBinding({ items: ['alpha', 'beta'], ...labels }, 'items');
		place(ListEditor, props);
		expect(screen.getByRole('button', { name: 'Move up alpha' }).getAttribute('aria-disabled')).toBe('true');
	});

	it('moves an item down and writes the new order', async () => {
		const { props, cell } = withBinding({ items: ['alpha', 'beta'], ...labels }, 'items');
		place(ListEditor, props);
		await fireEvent.click(screen.getByRole('button', { name: 'Move down alpha' }));
		expect(cell.current).toEqual(['beta', 'alpha']);
	});

	it('removes an item by its name', async () => {
		const { props, cell } = withBinding({ items: ['alpha', 'beta'], ...labels }, 'items');
		place(ListEditor, props);
		await fireEvent.click(screen.getByRole('button', { name: 'Remove beta' }));
		expect(cell.current).toEqual(['alpha']);
	});

	it('adds a blank row that is written once it has a value', async () => {
		const { props, cell } = withBinding({ items: ['alpha', 'beta'], ...labels }, 'items');
		place(ListEditor, props);
		await fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
		expect(cell.current).toEqual(['alpha', 'beta']);
		await fireEvent.input(screen.getByRole('textbox', { name: 'Server 3' }), { target: { value: 'gamma' } });
		expect(cell.current).toEqual(['alpha', 'beta', 'gamma']);
	});

	it('shows the validation message for an invalid item and keeps the last valid list', async () => {
		const validate = (item: string): string | null => (item.includes(' ') ? 'Remove spaces.' : null);
		const { props, cell } = withBinding({ items: ['alpha'], ...labels, validate }, 'items');
		place(ListEditor, props);
		await fireEvent.input(screen.getByRole('textbox', { name: 'Server 1' }), { target: { value: 'a b' } });
		expect(screen.getByText('Remove spaces.')).toBeTruthy();
		expect(cell.current).toEqual(['alpha']);
	});
});

describe('OctalModeEditor', () => {
	it('shows the symbolic form of the stored mode', () => {
		const { props } = withBinding({ value: '0644', label: 'Permissions' }, 'value');
		place(OctalModeEditor, props);
		expect((screen.getByRole('textbox', { name: 'Letters, like u=rw,go=r' }) as HTMLInputElement).value).toBe('rw-r--r--');
	});

	it('flags a value that is not octal and names the rule', () => {
		const { props } = withBinding({ value: '89', label: 'Permissions' }, 'value');
		place(OctalModeEditor, props);
		const field = screen.getByRole('textbox', { name: 'Permission number' });
		expect(field.getAttribute('aria-invalid')).toBe('true');
		expect(screen.getByText('Enter three or four digits from 0 to 7.')).toBeTruthy();
	});

	it('updates the octal value when a permission checkbox changes', async () => {
		const { props, cell } = withBinding({ value: '0644', label: 'Permissions' }, 'value');
		place(OctalModeEditor, props);
		const group = within(screen.getByRole('group', { name: 'Group' }));
		await fireEvent.click(group.getByRole('checkbox', { name: 'Write' }));
		expect(cell.current).toBe('0664');
	});

	it('updates the checkboxes when the octal value is typed', async () => {
		render(OctalModeEditor, { props: { value: '644', label: 'Permissions' } });
		const owner = within(screen.getByRole('group', { name: 'Owner' }));
		expect(owner.getByRole('checkbox', { name: 'Run' }).getAttribute('aria-checked')).toBe('false');
		await fireEvent.input(screen.getByRole('textbox', { name: 'Permission number' }), {
			target: { value: '755' }
		});
		expect(owner.getByRole('checkbox', { name: 'Run' }).getAttribute('aria-checked')).toBe('true');
		expect((screen.getByRole('textbox', { name: 'Letters, like u=rw,go=r' }) as HTMLInputElement).value).toBe('rwxr-xr-x');
	});
});
