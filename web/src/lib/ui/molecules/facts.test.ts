import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import AvatarLabel from './AvatarLabel.svelte';
import ConfirmInline from './ConfirmInline.svelte';
import CopyField from './CopyField.svelte';
import DescriptionList, { type DescriptionItem } from './DescriptionList.svelte';
import ShortcutHint from './ShortcutHint.svelte';

const snippetOf = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const deleteQuestion = 'Delete report.pdf for good?';
const deleteConsequence = 'This cannot be undone. Download links to it stop working.';
const deleteLabel = 'Delete 1 file';

afterEach(() => {
	cleanup();
});

describe('ConfirmInline', () => {
	it('names the group by its question and states the consequence', () => {
		render(ConfirmInline, {
			props: { question: deleteQuestion, consequence: deleteConsequence, confirmLabel: deleteLabel }
		});
		expect(screen.getByRole('group', { name: deleteQuestion })).toBeTruthy();
		expect(screen.getByText(deleteConsequence)).toBeTruthy();
	});

	it('renders a snippet consequence in place of text', () => {
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: snippetOf('Shared links stop working.'),
				confirmLabel: deleteLabel
			}
		});
		expect(screen.getByText('Shared links stop working.')).toBeTruthy();
	});

	it('starts focus on Cancel', async () => {
		render(ConfirmInline, {
			props: { question: deleteQuestion, consequence: deleteConsequence, confirmLabel: deleteLabel }
		});
		await tick();
		expect(document.activeElement).toBe(screen.getByRole('button', { name: t('common.cancel') }));
	});

	it('calls oncancel on Cancel click and on Escape', () => {
		const oncancel = vi.fn();
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				oncancel
			}
		});
		fireEvent.click(screen.getByRole('button', { name: t('common.cancel') }));
		fireEvent.keyDown(screen.getByRole('group'), { key: 'Escape' });
		expect(oncancel).toHaveBeenCalledTimes(2);
	});

	it('calls onconfirm when the confirm button is clicked', () => {
		const onconfirm = vi.fn();
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				onconfirm
			}
		});
		fireEvent.click(screen.getByRole('button', { name: deleteLabel }));
		expect(onconfirm).toHaveBeenCalledTimes(1);
	});

	it('marks the confirm button busy and does not confirm while busy', () => {
		const onconfirm = vi.fn();
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				busy: true,
				onconfirm
			}
		});
		const confirmButton = screen.getByRole('button', { name: deleteLabel });
		expect(confirmButton.getAttribute('aria-busy')).toBe('true');
		fireEvent.click(confirmButton);
		expect(onconfirm).not.toHaveBeenCalled();
	});

	it('exposes the tone on the group', () => {
		render(ConfirmInline, {
			props: {
				question: 'Restart the server?',
				consequence: 'Uploads pause for a moment.',
				confirmLabel: 'Restart',
				tone: 'warn'
			}
		});
		expect(screen.getByRole('group').getAttribute('data-tone')).toBe('warn');
	});

	it('asks for the typed word and keeps the confirm button enabled', () => {
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				typedWord: 'DELETE'
			}
		});
		expect(screen.getByLabelText(t('confirm.type_word', { word: 'DELETE' }))).toBeTruthy();
		expect(screen.getByRole('button', { name: deleteLabel }).hasAttribute('disabled')).toBe(false);
	});

	it('shows an inline error and does not confirm when the word does not match', () => {
		const onconfirm = vi.fn();
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				typedWord: 'DELETE',
				onconfirm
			}
		});
		const field = screen.getByLabelText(t('confirm.type_word', { word: 'DELETE' }));
		fireEvent.input(field, { target: { value: 'delete' } });
		fireEvent.click(screen.getByRole('button', { name: deleteLabel }));
		expect(onconfirm).not.toHaveBeenCalled();
		expect(field.getAttribute('aria-invalid')).toBe('true');
		expect(screen.getByText(t('confirm.word_mismatch', { word: 'DELETE' }))).toBeTruthy();
	});

	it('confirms once the typed word matches and clears the error', () => {
		const onconfirm = vi.fn();
		render(ConfirmInline, {
			props: {
				question: deleteQuestion,
				consequence: deleteConsequence,
				confirmLabel: deleteLabel,
				typedWord: 'DELETE',
				onconfirm
			}
		});
		const field = screen.getByLabelText(t('confirm.type_word', { word: 'DELETE' }));
		fireEvent.input(field, { target: { value: 'nope' } });
		fireEvent.click(screen.getByRole('button', { name: deleteLabel }));
		fireEvent.input(field, { target: { value: 'DELETE' } });
		expect(screen.queryByText(t('confirm.word_mismatch', { word: 'DELETE' }))).toBeNull();
		fireEvent.click(screen.getByRole('button', { name: deleteLabel }));
		expect(onconfirm).toHaveBeenCalledTimes(1);
	});
});

describe('CopyField', () => {
	it('labels a read-only field with its value', () => {
		render(CopyField, { props: { label: 'Version tag', value: 'v2-abc' } });
		const field = screen.getByLabelText('Version tag') as HTMLInputElement;
		expect(field.value).toBe('v2-abc');
		expect(field.readOnly).toBe(true);
	});

	it('offers a copy button whose name contains Copy', () => {
		render(CopyField, { props: { label: 'Version tag', value: 'v2-abc' } });
		expect(screen.getByRole('button', { name: new RegExp(t('common.copy')) })).toBeTruthy();
	});

	it('adds the secret warning and links it to the field', () => {
		render(CopyField, { props: { label: 'Access key', value: 'k-123', secretOnce: true } });
		const field = screen.getByLabelText('Access key');
		const describedBy = field.getAttribute('aria-describedby');
		expect(describedBy).toBeTruthy();
		expect(document.getElementById(describedBy ?? '')?.textContent).toContain(t('facts.secret_once'));
	});

	it('shows no warning and no description by default', () => {
		render(CopyField, { props: { label: 'Version tag', value: 'v2-abc' } });
		expect(screen.queryByText(t('facts.secret_once'))).toBeNull();
		expect(screen.getByLabelText('Version tag').getAttribute('aria-describedby')).toBeNull();
	});
});

describe('ShortcutHint', () => {
	it('renders each key as a kbd element next to the label', () => {
		const { container } = render(ShortcutHint, { props: { keys: ['mod', 'K'], label: 'Open palette' } });
		expect(container.querySelectorAll('kbd')).toHaveLength(2);
		expect(screen.getByText('Open palette')).toBeTruthy();
	});

	it('renders only the keys when no label is given', () => {
		const { container } = render(ShortcutHint, { props: { keys: ['enter'] } });
		expect(container.querySelectorAll('kbd')).toHaveLength(1);
		expect(container.textContent?.trim()).toBe(container.querySelector('kbd')?.textContent?.trim());
	});
});

describe('DescriptionList', () => {
	const items: DescriptionItem[] = [
		{ term: 'Path', value: '/reports/q3.pdf', copy: '/reports/q3.pdf' },
		{ term: 'Type', value: snippetOf('File'), help: 'Files are stored once.' }
	];

	it('renders terms and values as a definition list', () => {
		const { container } = render(DescriptionList, { props: { items } });
		expect(container.querySelector('dl')).toBeTruthy();
		expect(screen.getAllByRole('term')).toHaveLength(2);
		expect(screen.getAllByRole('definition')).toHaveLength(2);
		expect(screen.getByText('/reports/q3.pdf')).toBeTruthy();
	});

	it('renders snippet values', () => {
		render(DescriptionList, { props: { items } });
		expect(screen.getByText('File')).toBeTruthy();
	});

	it('adds a copy button only to items with a copy value', () => {
		render(DescriptionList, { props: { items } });
		expect(screen.getAllByRole('button', { name: new RegExp(t('common.copy')) })).toHaveLength(1);
	});

	it('links help text to its value through aria-describedby', () => {
		render(DescriptionList, { props: { items } });
		const help = screen.getByText('Files are stored once.');
		const definition = help.closest('dd');
		expect(help.id).toBeTruthy();
		expect(definition?.getAttribute('aria-describedby')).toBe(help.id);
	});

	it('exposes the column count on the list', () => {
		const { container } = render(DescriptionList, { props: { items, columns: 2 } });
		expect(container.querySelector('dl')?.getAttribute('data-columns')).toBe('2');
	});
});

describe('AvatarLabel', () => {
	it('shows the name and secondary text beside the avatar', () => {
		render(AvatarLabel, { props: { name: 'Ada Lovelace', secondary: 'Owner' } });
		expect(screen.getByText('Ada Lovelace')).toBeTruthy();
		expect(screen.getByText('Owner')).toBeTruthy();
	});

	it('shows only the name when no secondary text is given', () => {
		render(AvatarLabel, { props: { name: 'Ada Lovelace' } });
		expect(screen.getByText('Ada Lovelace')).toBeTruthy();
		expect(screen.queryByText('Owner')).toBeNull();
	});

	it('hides the avatar from assistive technology', () => {
		const { container } = render(AvatarLabel, { props: { name: 'Ada Lovelace' } });
		expect(container.querySelector('[aria-hidden="true"]')).toBeTruthy();
	});
});
