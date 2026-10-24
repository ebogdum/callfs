import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import AlertDialog from './AlertDialog.svelte';
import Dialog from './Dialog.svelte';
import Drawer from './Drawer.svelte';

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));
const markupSnippet = (markup: string) => createRawSnippet(() => ({ render: () => markup }));

afterEach(() => {
	cleanup();
});

describe('Dialog', () => {
	it('names the dialog by its title and describes it', () => {
		render(Dialog, {
			props: { open: true, title: 'Rename file', description: 'Pick a new name.', children: textSnippet('Body') }
		});
		const dialog = screen.getByRole('dialog', { name: 'Rename file' });
		expect(dialog).toHaveAccessibleDescription('Pick a new name.');
	});

	it('exposes the size as a data attribute', () => {
		render(Dialog, { props: { open: true, title: 'Share', size: 'lg', children: textSnippet('Body') } });
		expect(document.querySelector('.dialog')?.getAttribute('data-size')).toBe('lg');
	});

	it('renders the footer snippet', () => {
		render(Dialog, {
			props: { open: true, title: 'Share', children: textSnippet('Body'), footer: textSnippet('Footer copy') }
		});
		expect(screen.getByText('Footer copy')).toBeInTheDocument();
	});

	it('closes from the Close button', async () => {
		render(Dialog, { props: { open: true, title: 'Share', children: textSnippet('Body') } });
		await fireEvent.click(screen.getByRole('button', { name: t('u092overlays.close') }));
		await waitFor(() => {
			expect(screen.queryByRole('dialog')).toBeNull();
		});
	});

	it('closes on Escape when dismissible', async () => {
		render(Dialog, { props: { open: true, title: 'Share', children: textSnippet('Body') } });
		await fireEvent.keyDown(document, { key: 'Escape' });
		await waitFor(() => {
			expect(screen.queryByRole('dialog')).toBeNull();
		});
	});

	it('ignores Escape and disables Close when not dismissible', async () => {
		render(Dialog, { props: { open: true, title: 'Edit rule', dismissible: false, children: textSnippet('Body') } });
		await fireEvent.keyDown(document, { key: 'Escape' });
		expect(screen.getByRole('dialog', { name: 'Edit rule' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('u092overlays.close') })).toHaveAttribute('aria-disabled', 'true');
	});

	it('moves focus to the first field when it opens', async () => {
		render(Dialog, {
			props: { open: true, title: 'Rename', children: markupSnippet('<label>Name <input name="name" /></label>') }
		});
		await waitFor(() => {
			expect(screen.getByRole('textbox', { name: 'Name' })).toHaveFocus();
		});
	});

	it('moves focus to the first non-danger footer action when there is no field', async () => {
		render(Dialog, {
			props: {
				open: true,
				title: 'Remove',
				children: textSnippet('Body'),
				footer: markupSnippet(
					'<div><button type="button" data-tone="danger">Remove</button><button type="button">Keep</button></div>'
				)
			}
		});
		await waitFor(() => {
			expect(screen.getByRole('button', { name: 'Keep' })).toHaveFocus();
		});
	});
});

describe('AlertDialog', () => {
	it('names the alert by its title and describes it with the body', () => {
		render(AlertDialog, {
			props: {
				open: true,
				title: 'Delete report.pdf for good?',
				body: 'This cannot be undone.',
				confirmLabel: 'Delete 1 file',
				onconfirm: vi.fn()
			}
		});
		expect(screen.getByRole('alertdialog', { name: 'Delete report.pdf for good?' })).toHaveAccessibleDescription(
			'This cannot be undone.'
		);
	});

	it('moves focus to Cancel when it opens', async () => {
		render(AlertDialog, {
			props: { open: true, title: 'Delete', body: 'Gone.', confirmLabel: 'Delete', onconfirm: vi.fn() }
		});
		await waitFor(() => {
			expect(screen.getByRole('button', { name: t('u092overlays.cancel') })).toHaveFocus();
		});
	});

	it('uses the danger variant for the danger tone', () => {
		render(AlertDialog, {
			props: { open: true, title: 'Delete', body: 'Gone.', confirmLabel: 'Delete', onconfirm: vi.fn() }
		});
		expect(screen.getByRole('button', { name: 'Delete' })).toHaveAttribute('data-variant', 'danger');
	});

	it('uses the primary variant for the warn tone', () => {
		render(AlertDialog, {
			props: {
				open: true,
				title: 'Move to trash?',
				body: 'You can restore it.',
				confirmLabel: 'Move to trash',
				tone: 'warn',
				onconfirm: vi.fn()
			}
		});
		expect(screen.getByRole('button', { name: 'Move to trash' })).toHaveAttribute('data-variant', 'primary');
	});

	it('runs onconfirm once and closes when no typed word is required', async () => {
		const onconfirm = vi.fn();
		render(AlertDialog, {
			props: {
				open: true,
				title: 'Move to trash?',
				body: 'You can restore it.',
				confirmLabel: 'Move to trash',
				tone: 'warn',
				onconfirm
			}
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Move to trash' }));
		expect(onconfirm).toHaveBeenCalledTimes(1);
		await waitFor(() => {
			expect(screen.queryByRole('alertdialog')).toBeNull();
		});
	});

	it('keeps the dialog open and shows the mismatch error when the typed word is wrong', async () => {
		const onconfirm = vi.fn();
		render(AlertDialog, {
			props: {
				open: true,
				title: 'Delete all under /team?',
				body: 'This cannot be undone.',
				confirmLabel: 'Delete',
				typedWord: 'DELETE',
				onconfirm
			}
		});
		const input = screen.getByRole('textbox', { name: t('u092overlays.type_word', { word: 'DELETE' }) });
		await fireEvent.input(input, { target: { value: 'delete' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
		expect(onconfirm).not.toHaveBeenCalled();
		expect(screen.getByRole('alertdialog')).toBeInTheDocument();
		expect(input).toHaveAttribute('aria-invalid', 'true');
		expect(screen.getByText(t('u092overlays.type_mismatch', { word: 'DELETE' }))).toBeInTheDocument();
	});

	it('runs onconfirm when the typed word matches', async () => {
		const onconfirm = vi.fn();
		render(AlertDialog, {
			props: {
				open: true,
				title: 'Delete all under /team?',
				body: 'This cannot be undone.',
				confirmLabel: 'Delete',
				typedWord: 'DELETE',
				onconfirm
			}
		});
		await fireEvent.input(screen.getByRole('textbox', { name: t('u092overlays.type_word', { word: 'DELETE' }) }), {
			target: { value: 'DELETE' }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
		expect(onconfirm).toHaveBeenCalledTimes(1);
	});

	it('closes without confirming when Cancel is pressed', async () => {
		const onconfirm = vi.fn();
		render(AlertDialog, {
			props: { open: true, title: 'Delete', body: 'Gone.', confirmLabel: 'Delete', onconfirm }
		});
		await fireEvent.click(screen.getByRole('button', { name: t('u092overlays.cancel') }));
		expect(onconfirm).not.toHaveBeenCalled();
		await waitFor(() => {
			expect(screen.queryByRole('alertdialog')).toBeNull();
		});
	});

	it('closes on Escape without confirming', async () => {
		const onconfirm = vi.fn();
		render(AlertDialog, {
			props: { open: true, title: 'Delete', body: 'Gone.', confirmLabel: 'Delete', onconfirm }
		});
		await fireEvent.keyDown(document, { key: 'Escape' });
		expect(onconfirm).not.toHaveBeenCalled();
		await waitFor(() => {
			expect(screen.queryByRole('alertdialog')).toBeNull();
		});
	});
});

describe('Drawer', () => {
	it('names the panel by its title and defaults to the end side and default width', () => {
		render(Drawer, { props: { open: true, title: 'Details', children: textSnippet('Body') } });
		expect(screen.getByRole('dialog', { name: 'Details' })).toBeInTheDocument();
		expect(document.querySelector('.drawer')?.getAttribute('data-side')).toBe('end');
		expect(document.querySelector('.drawer')?.getAttribute('data-width')).toBe('default');
	});

	it('exposes side and width as data attributes', () => {
		render(Drawer, {
			props: { open: true, title: 'Edit link', side: 'start', width: 'wide', children: textSnippet('Body') }
		});
		expect(document.querySelector('.drawer')?.getAttribute('data-side')).toBe('start');
		expect(document.querySelector('.drawer')?.getAttribute('data-width')).toBe('wide');
	});

	it('renders the scrim only when modal', () => {
		render(Drawer, { props: { open: true, title: 'Details', modal: true, children: textSnippet('Body') } });
		expect(document.querySelector('.scrim')).not.toBeNull();
	});

	it('renders no scrim when non-modal', () => {
		render(Drawer, { props: { open: true, title: 'Details', children: textSnippet('Body') } });
		expect(document.querySelector('.scrim')).toBeNull();
	});

	it('renders the footer snippet', () => {
		render(Drawer, {
			props: { open: true, title: 'Details', children: textSnippet('Body'), footer: textSnippet('Footer copy') }
		});
		expect(screen.getByText('Footer copy')).toBeInTheDocument();
	});

	it('closes from the Close button', async () => {
		render(Drawer, { props: { open: true, title: 'Details', children: textSnippet('Body') } });
		await fireEvent.click(screen.getByRole('button', { name: t('u092overlays.close') }));
		await waitFor(() => {
			expect(screen.queryByRole('dialog')).toBeNull();
		});
	});

	it('ignores Escape and disables Close while dirty', async () => {
		render(Drawer, { props: { open: true, title: 'Edit link', dirty: true, children: textSnippet('Body') } });
		await fireEvent.keyDown(document, { key: 'Escape' });
		expect(screen.getByRole('dialog', { name: 'Edit link' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('u092overlays.close') })).toHaveAttribute('aria-disabled', 'true');
	});

	it('exposes the sheet grip as a separator and moves it with the keyboard', async () => {
		render(Drawer, { props: { open: true, title: 'Details', children: textSnippet('Body') } });
		const grip = screen.getByLabelText(t('u092overlays.resize_panel'));
		expect(grip).toHaveAttribute('aria-valuenow', '50');
		await fireEvent.keyDown(grip, { key: 'ArrowUp' });
		expect(grip).toHaveAttribute('aria-valuenow', '90');
		await fireEvent.keyDown(grip, { key: 'Home' });
		expect(grip).toHaveAttribute('aria-valuenow', '50');
	});
});
