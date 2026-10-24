import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { createRawSnippet } from 'svelte';
import { t } from '$lib/copy/t';
import FieldError from './FieldError.svelte';
import HelpText from './HelpText.svelte';
import Input from './Input.svelte';
import Label from './Label.svelte';

const text = (value: string) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));

afterEach(() => {
	cleanup();
});

describe('Label', () => {
	it('renders a label bound to its control by for', () => {
		const { container } = render(Label, {
			props: { for: 'access-key', children: text('Access key') }
		});
		const label = container.querySelector('label');
		expect(label?.getAttribute('for')).toBe('access-key');
		expect(label?.textContent).toBe('Access key');
	});

	it('shows the optional suffix from copy when optional is set', () => {
		const { container } = render(Label, {
			props: { for: 'nickname', optional: true, children: text('Nickname') }
		});
		const label = container.querySelector('label');
		expect(label?.textContent).toContain(t('common.optional'));
		expect(label?.getAttribute('for')).toBe('nickname');
	});

	it('omits the optional suffix by default', () => {
		const { container } = render(Label, {
			props: { for: 'name', children: text('Name') }
		});
		expect(container.querySelector('.optional')).toBeNull();
	});

	it('merges class onto the label root', () => {
		const { container } = render(Label, {
			props: { for: 'name', class: 'extra', children: text('Name') }
		});
		expect(container.querySelector('label')?.classList.contains('extra')).toBe(true);
	});
});

describe('HelpText', () => {
	it('renders a paragraph with the given id and text', () => {
		const { container } = render(HelpText, {
			props: { id: 'help-key', children: text('Ask your admin for a key.') }
		});
		const help = container.querySelector('p');
		expect(help?.id).toBe('help-key');
		expect(help?.textContent).toBe('Ask your admin for a key.');
	});

	it('is linkable through aria-describedby on the control', () => {
		const { getByRole } = render(Input, {
			props: { value: '', 'aria-describedby': 'help-key' }
		});
		expect(getByRole('textbox').getAttribute('aria-describedby')).toBe('help-key');
	});
});

describe('FieldError', () => {
	it('renders the message with an id and a decorative danger glyph', () => {
		const { container } = render(FieldError, {
			props: { id: 'key-error', message: 'That key did not work.' }
		});
		const error = container.querySelector('p');
		expect(error?.id).toBe('key-error');
		expect(error?.textContent).toContain('That key did not work.');
		expect(error?.hasAttribute('role')).toBe(false);
		expect(container.querySelector('[data-tone="danger"]')?.getAttribute('aria-hidden')).toBe('true');
	});

	it('merges class onto the paragraph root', () => {
		const { container } = render(FieldError, {
			props: { id: 'key-error', message: 'Required.', class: 'extra' }
		});
		expect(container.querySelector('p')?.classList.contains('extra')).toBe(true);
	});
});

describe('Input', () => {
	it('renders a textbox with the given type and no invalid marker by default', () => {
		const { container, getByRole } = render(Input, {
			props: { type: 'email', id: 'mail', value: 'a@b.c' }
		});
		const field = getByRole('textbox') as HTMLInputElement;
		expect(field.type).toBe('email');
		expect(field.value).toBe('a@b.c');
		expect(field.hasAttribute('aria-invalid')).toBe(false);
		expect(container.querySelector('.control')?.hasAttribute('data-invalid')).toBe(false);
	});

	it('exposes invalid state through aria-invalid and data-invalid', () => {
		const { container, getByRole } = render(Input, {
			props: { invalid: true, value: 'x' }
		});
		expect(getByRole('textbox').getAttribute('aria-invalid')).toBe('true');
		expect(container.querySelector('.control')?.getAttribute('data-invalid')).toBe('true');
	});

	it('is never disabled, so paste stays allowed', () => {
		const { getByRole } = render(Input, { props: { value: '' } });
		const field = getByRole('textbox') as HTMLInputElement;
		expect(field.disabled).toBe(false);
		expect(field.readOnly).toBe(false);
	});

	it('updates value on input', () => {
		const { getByRole } = render(Input, { props: { value: '' } });
		const field = getByRole('textbox') as HTMLInputElement;
		fireEvent.input(field, { target: { value: 'typed' } });
		expect(field.value).toBe('typed');
	});

	it('shows no clear button while empty and a clear button named from copy when filled', () => {
		const empty = render(Input, { props: { clearable: true, value: '' } });
		expect(empty.queryByRole('button')).toBeNull();
		empty.unmount();

		const filled = render(Input, { props: { clearable: true, value: 'abc' } });
		expect(filled.getByRole('button', { name: t('common.clear') })).toBeTruthy();
	});

	it('clears the value and returns focus to the field when clear is pressed', () => {
		const { getByRole } = render(Input, { props: { clearable: true, value: 'abc' } });
		const field = getByRole('textbox') as HTMLInputElement;
		fireEvent.click(getByRole('button', { name: t('common.clear') }));
		expect(field.value).toBe('');
		expect(document.activeElement).toBe(field);
	});

	it('hides the clear button when clearable is not set', () => {
		const { queryByRole } = render(Input, { props: { value: 'abc' } });
		expect(queryByRole('button')).toBeNull();
	});

	it('applies size, mono and class to the control and passes rest props to the field', () => {
		const { container, getByRole } = render(Input, {
			props: { size: 'sm', mono: true, class: 'extra', name: 'bucket', value: '' }
		});
		const control = container.querySelector('.control');
		expect(control?.getAttribute('data-size')).toBe('sm');
		expect(control?.getAttribute('data-mono')).toBe('true');
		expect(control?.classList.contains('extra')).toBe(true);
		expect(getByRole('textbox').getAttribute('name')).toBe('bucket');
	});

	it('renders prefix and suffix addons', () => {
		const { container } = render(Input, {
			props: {
				value: '',
				prefix: text('https://'),
				suffix: text('/api')
			}
		});
		const addons = container.querySelectorAll('.addon');
		expect(addons.length).toBe(2);
		expect(addons[0]?.textContent).toBe('https://');
		expect(addons[1]?.textContent).toBe('/api');
	});
});
