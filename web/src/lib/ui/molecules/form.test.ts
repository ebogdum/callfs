import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { createRawSnippet } from 'svelte';
import { t } from '$lib/copy/t';
import CheckboxGroup from './CheckboxGroup.svelte';
import EnumField from './EnumField.svelte';
import FormField from './FormField.svelte';
import RadioGroup from './RadioGroup.svelte';

const input = createRawSnippet<[{ id: string; describedBy: string | undefined; invalid: boolean }]>((params) => ({
	render: () =>
		`<input id="${params().id}" aria-describedby="${params().describedBy ?? ''}" aria-invalid="${String(params().invalid)}" />`
}));

const planOptions = [
	{ value: 'free', label: 'Free' },
	{ value: 'team', label: 'Team', description: 'Up to 10 people' },
	{ value: 'pro', label: 'Pro', recommended: true },
	{ value: 'org', label: 'Organisation', disabledReason: 'Ask an admin' }
];

const manyOptions = Array.from({ length: 6 }, (_, index) => ({ value: `v${index}`, label: `Option ${index}` }));

const hugeOptions = Array.from({ length: 16 }, (_, index) => ({ value: `r${index}`, label: `Region ${index}` }));

afterEach(() => {
	cleanup();
});

describe('FormField', () => {
	it('binds the label to the control and passes describedBy for help text', () => {
		const { container } = render(FormField, {
			props: { label: 'Access key', for: 'access-key', help: 'Ask your admin.', control: input }
		});
		const label = container.querySelector('label');
		const field = container.querySelector('input');
		expect(label?.getAttribute('for')).toBe('access-key');
		expect(field?.id).toBe('access-key');
		expect(field?.getAttribute('aria-describedby')).toBe('access-key-help');
		expect(screen.getByText('Ask your admin.').id).toBe('access-key-help');
	});

	it('renders the example copy with its value and links it in describedBy', () => {
		const { container } = render(FormField, {
			props: { label: 'Domain', for: 'domain', example: 'news.shop.com', control: input }
		});
		expect(screen.getByText(t('form.example', { value: 'news.shop.com' }))).toBeTruthy();
		expect(container.querySelector('input')?.getAttribute('aria-describedby')).toBe('domain-example');
	});

	it('marks the control invalid and shows the error in a polite live region', () => {
		const { container } = render(FormField, {
			props: { label: 'Name', for: 'name', error: 'Name is taken.', control: input }
		});
		const field = container.querySelector('input');
		const region = container.querySelector('[aria-live="polite"]');
		expect(field?.getAttribute('aria-invalid')).toBe('true');
		expect(field?.getAttribute('aria-describedby')).toBe('name-error');
		expect(region?.textContent).toContain('Name is taken.');
	});

	it('keeps the live region empty and the control valid without an error', () => {
		const { container } = render(FormField, {
			props: { label: 'Name', for: 'name', control: input }
		});
		expect(container.querySelector('[aria-live="polite"]')?.textContent?.trim()).toBe('');
		expect(container.querySelector('input')?.getAttribute('aria-invalid')).toBe('false');
	});

	it('shows the optional suffix from copy when optional is set', () => {
		const { container } = render(FormField, {
			props: { label: 'Nickname', for: 'nickname', optional: true, control: input }
		});
		expect(container.querySelector('label')?.textContent).toContain(t('common.optional'));
	});

	it('exposes the inline layout for wide screens', () => {
		const { container } = render(FormField, {
			props: { label: 'Enabled', for: 'enabled', layout: 'inline', control: input }
		});
		expect(container.querySelector('.form-field')?.getAttribute('data-layout')).toBe('inline');
	});
});

describe('RadioGroup', () => {
	it('names the radiogroup by its visible label and marks the value checked', () => {
		render(RadioGroup, { props: { value: 'pro', options: planOptions, label: 'Plan' } });
		const group = screen.getByRole('radiogroup', { name: 'Plan' });
		expect(group).toBeTruthy();
		expect(screen.getByRole('radio', { name: 'Pro' }).getAttribute('aria-checked')).toBe('true');
		expect(screen.getByRole('radio', { name: 'Free' }).getAttribute('aria-checked')).toBe('false');
	});

	it('shows the recommended badge from copy', () => {
		render(RadioGroup, { props: { value: 'free', options: planOptions, label: 'Plan' } });
		expect(screen.getByText(t('common.recommended'))).toBeTruthy();
	});

	it('moves the selection when an option is clicked', () => {
		render(RadioGroup, { props: { value: 'free', options: planOptions, label: 'Plan' } });
		fireEvent.click(screen.getByRole('radio', { name: 'Team' }));
		expect(screen.getByRole('radio', { name: 'Team' }).getAttribute('aria-checked')).toBe('true');
	});

	it('disables options with a reason and describes the reason', () => {
		render(RadioGroup, { props: { value: 'free', options: planOptions, label: 'Plan' } });
		const org = screen.getByRole('radio', { name: 'Organisation' });
		expect(org.hasAttribute('disabled')).toBe(true);
		expect(org.getAttribute('aria-describedby')).toBeTruthy();
		expect(screen.getByText('Ask an admin')).toBeTruthy();
	});

	it('describes options with their description text', () => {
		render(RadioGroup, { props: { value: 'free', options: planOptions, label: 'Plan' } });
		expect(screen.getByText('Up to 10 people')).toBeTruthy();
	});
});

describe('CheckboxGroup', () => {
	it('names the fieldset group by its legend and marks initial values checked', () => {
		render(CheckboxGroup, {
			props: {
				values: ['a'],
				label: 'Shared folders',
				options: [
					{ value: 'a', label: 'Alpha' },
					{ value: 'b', label: 'Beta' }
				]
			}
		});
		expect(screen.getByRole('group', { name: 'Shared folders' })).toBeTruthy();
		expect(screen.getByRole('checkbox', { name: 'Alpha' }).getAttribute('aria-checked')).toBe('true');
		expect(screen.getByRole('checkbox', { name: 'Beta' }).getAttribute('aria-checked')).toBe('false');
	});

	it('checks and unchecks options on click', () => {
		render(CheckboxGroup, {
			props: {
				values: ['a'],
				label: 'Shared folders',
				options: [
					{ value: 'a', label: 'Alpha' },
					{ value: 'b', label: 'Beta' }
				]
			}
		});
		fireEvent.click(screen.getByRole('checkbox', { name: 'Beta' }));
		expect(screen.getByRole('checkbox', { name: 'Beta' }).getAttribute('aria-checked')).toBe('true');
		fireEvent.click(screen.getByRole('checkbox', { name: 'Alpha' }));
		expect(screen.getByRole('checkbox', { name: 'Alpha' }).getAttribute('aria-checked')).toBe('false');
	});

	it('disables options with a reason', () => {
		render(CheckboxGroup, {
			props: {
				values: [],
				label: 'Permissions',
				options: [{ value: 'write', label: 'Write', disabledReason: 'Locked by admin' }]
			}
		});
		expect(screen.getByRole('checkbox', { name: 'Write' }).hasAttribute('disabled')).toBe(true);
		expect(screen.getByText('Locked by admin')).toBeTruthy();
	});
});

describe('EnumField', () => {
	it('renders a radiogroup for five or fewer options', () => {
		render(EnumField, {
			props: {
				value: 'free',
				label: 'Plan',
				options: planOptions.slice(0, 3)
			}
		});
		expect(screen.getByRole('radiogroup', { name: 'Plan' })).toBeTruthy();
	});

	it('renders a labelled select with help for six to fifteen options', () => {
		const { container } = render(EnumField, {
			props: { value: 'v0', label: 'Choice', help: 'Pick one.', options: manyOptions }
		});
		expect(screen.queryByRole('radiogroup')).toBeNull();
		const select = screen.getByLabelText('Choice');
		expect(select.tagName).toBe('SELECT');
		expect(select.getAttribute('aria-describedby')).toContain('-help');
		expect(screen.getByText('Pick one.')).toBeTruthy();
		expect(container.querySelectorAll('option').length).toBe(manyOptions.length);
	});

	it('does not render a radiogroup or select for more than fifteen options', () => {
		const { container } = render(EnumField, {
			props: { value: 'r0', label: 'Region', options: hugeOptions }
		});
		expect(screen.queryByRole('radiogroup')).toBeNull();
		expect(container.querySelector('select')).toBeNull();
	});
});
