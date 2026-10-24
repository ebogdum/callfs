import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import NumberInput from './NumberInput.svelte';
import Select from './Select.svelte';
import Textarea from './Textarea.svelte';

afterEach(() => {
	cleanup();
});

describe('Textarea', () => {
	it('renders a textbox named by its aria-label with three rows by default', () => {
		render(Textarea, { props: { value: '', 'aria-label': 'Notes' } });
		const field = screen.getByRole('textbox', { name: 'Notes' });
		expect(field.getAttribute('rows')).toBe('3');
	});

	it('passes maxLength to the field', () => {
		render(Textarea, { props: { value: '', maxLength: 10, 'aria-label': 'Notes' } });
		expect(screen.getByRole('textbox', { name: 'Notes' }).getAttribute('maxlength')).toBe('10');
	});

	it('hides the character count below 80% of maxLength', () => {
		render(Textarea, { props: { value: 'abcdefg', maxLength: 10, 'aria-label': 'Notes' } });
		expect(screen.queryByText(t('form.char_count', { count: 7, max: 10 }))).toBeNull();
	});

	it('shows the character count from 80% of maxLength', () => {
		render(Textarea, { props: { value: 'abcdefgh', maxLength: 10, 'aria-label': 'Notes' } });
		expect(screen.getByText(t('form.char_count', { count: 8, max: 10 })).textContent).toBe(
			t('form.char_count', { count: 8, max: 10 })
		);
	});

	it('submits on mod+enter with the Meta key', async () => {
		const onsubmit = vi.fn();
		render(Textarea, { props: { value: 'draft', onsubmit, 'aria-label': 'Notes' } });
		await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Notes' }), { key: 'Enter', metaKey: true });
		expect(onsubmit).toHaveBeenCalledTimes(1);
	});

	it('submits on mod+enter with the Ctrl key', async () => {
		const onsubmit = vi.fn();
		render(Textarea, { props: { value: 'draft', onsubmit, 'aria-label': 'Notes' } });
		await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Notes' }), { key: 'Enter', ctrlKey: true });
		expect(onsubmit).toHaveBeenCalledTimes(1);
	});

	it('keeps plain enter as a line break', async () => {
		const onsubmit = vi.fn();
		render(Textarea, { props: { value: 'draft', onsubmit, 'aria-label': 'Notes' } });
		await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Notes' }), { key: 'Enter' });
		expect(onsubmit).not.toHaveBeenCalled();
	});

	it('forwards keydown to the caller handler', async () => {
		const onkeydown = vi.fn();
		render(Textarea, { props: { value: '', onkeydown, 'aria-label': 'Notes' } });
		await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Notes' }), { key: 'a' });
		expect(onkeydown).toHaveBeenCalledTimes(1);
	});

	it('exposes disabled and invalid state to assistive tech', () => {
		render(Textarea, { props: { value: '', disabled: true, 'aria-invalid': true, 'aria-label': 'Notes' } });
		const field = screen.getByRole('textbox', { name: 'Notes' }) as HTMLTextAreaElement;
		expect(field.disabled).toBe(true);
		expect(field.getAttribute('aria-invalid')).toBe('true');
	});
});

describe('NumberInput', () => {
	it('renders a decimal text field named by its aria-label', () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Timeout' } });
		const field = screen.getByRole('textbox', { name: 'Timeout' }) as HTMLInputElement;
		expect(field.getAttribute('inputmode')).toBe('decimal');
		expect(field.value).toBe('5');
	});

	it('shows an empty field for a null value', () => {
		render(NumberInput, { props: { value: null, 'aria-label': 'Timeout' } });
		expect((screen.getByRole('textbox', { name: 'Timeout' }) as HTMLInputElement).value).toBe('');
	});

	it('formats the value to the requested precision', () => {
		render(NumberInput, { props: { value: 1.5, precision: 2, 'aria-label': 'Ratio' } });
		expect((screen.getByRole('textbox', { name: 'Ratio' }) as HTMLInputElement).value).toBe('1.50');
	});

	it('steps by one on ArrowUp and ArrowDown', async () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.keyDown(field, { key: 'ArrowUp' });
		expect(field.value).toBe('6');
		await fireEvent.keyDown(field, { key: 'ArrowDown' });
		expect(field.value).toBe('5');
	});

	it('steps by ten with Shift', async () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.keyDown(field, { key: 'ArrowUp', shiftKey: true });
		expect(field.value).toBe('15');
	});

	it('starts from zero when the value is empty', async () => {
		render(NumberInput, { props: { value: null, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.keyDown(field, { key: 'ArrowUp' });
		expect(field.value).toBe('1');
	});

	it('steps with the labelled increase and decrease buttons', async () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.click(screen.getByRole('button', { name: t('common.increase') }));
		expect(field.value).toBe('6');
		await fireEvent.click(screen.getByRole('button', { name: t('common.decrease') }));
		expect(field.value).toBe('5');
	});

	it('clamps to min and max on blur', async () => {
		render(NumberInput, { props: { value: 5, min: 0, max: 10, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.input(field, { target: { value: '42' } });
		await fireEvent.blur(field);
		expect(field.value).toBe('10');
	});

	it('restores the last valid value on blur when the text is not a number', async () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.input(field, { target: { value: 'abc' } });
		await fireEvent.blur(field);
		expect(field.value).toBe('5');
	});

	it('keeps an emptied field empty on blur', async () => {
		render(NumberInput, { props: { value: 5, 'aria-label': 'Count' } });
		const field = screen.getByRole('textbox', { name: 'Count' }) as HTMLInputElement;
		await fireEvent.input(field, { target: { value: '' } });
		await fireEvent.blur(field);
		expect(field.value).toBe('');
	});

	it('shows the unit beside the field', () => {
		render(NumberInput, { props: { value: 300, unit: 'ms', 'aria-label': 'Timeout' } });
		expect(screen.getByText('ms').textContent).toBe('ms');
	});

	it('calls the caller blur handler', async () => {
		const onblur = vi.fn();
		render(NumberInput, { props: { value: 5, onblur, 'aria-label': 'Count' } });
		await fireEvent.blur(screen.getByRole('textbox', { name: 'Count' }));
		expect(onblur).toHaveBeenCalledTimes(1);
	});
});

describe('Select', () => {
	const options = [
		{ value: 'a', label: 'Alpha' },
		{ value: 'b', label: 'Beta', disabled: true }
	];

	it('renders a native select named by its aria-label with its options', () => {
		render(Select, { props: { value: 'a', options, 'aria-label': 'Sort' } });
		const select = screen.getByRole('combobox', { name: 'Sort' }) as HTMLSelectElement;
		expect(select.value).toBe('a');
		expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['Alpha', 'Beta']);
	});

	it('disables options marked disabled', () => {
		render(Select, { props: { value: 'a', options, 'aria-label': 'Sort' } });
		const beta = screen.getByRole('option', { name: 'Beta' }) as HTMLOptionElement;
		expect(beta.disabled).toBe(true);
	});

	it('changes value when another option is chosen', async () => {
		render(Select, {
			props: {
				value: 'a',
				options: [
					{ value: 'a', label: 'Alpha' },
					{ value: 'c', label: 'Gamma' }
				],
				'aria-label': 'Sort'
			}
		});
		const select = screen.getByRole('combobox', { name: 'Sort' }) as HTMLSelectElement;
		await fireEvent.change(select, { target: { value: 'c' } });
		expect(select.value).toBe('c');
	});

	it('exposes size on the wrapper', () => {
		const { container } = render(Select, { props: { value: 'a', options, size: 'lg', 'aria-label': 'Sort' } });
		expect(container.querySelector('.select')?.getAttribute('data-size')).toBe('lg');
	});

	it('passes class to the wrapper and disabled to the control', () => {
		const { container } = render(Select, {
			props: { value: 'a', options, class: 'extra', disabled: true, 'aria-label': 'Sort' }
		});
		expect(container.querySelector('.select.extra')?.tagName).toBe('SPAN');
		expect((screen.getByRole('combobox', { name: 'Sort' }) as HTMLSelectElement).disabled).toBe(true);
	});
});
