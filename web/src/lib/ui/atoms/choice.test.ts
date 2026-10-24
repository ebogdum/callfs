import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { RadioGroup } from 'bits-ui';
import { createRawSnippet, mount, unmount } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import Checkbox from './Checkbox.svelte';
import Radio from './Radio.svelte';
import Slider from './Slider.svelte';
import Switch from './Switch.svelte';

type RadioProps = {
	value: string;
	label: string;
	description?: string;
	recommended?: boolean;
};

function renderRadioInGroup(props: RadioProps, groupValue: string) {
	const children = createRawSnippet(() => ({
		render: () => '<div></div>',
		setup: (element: Element) => {
			const instance = mount(Radio, { target: element, props });
			return () => {
				void unmount(instance);
			};
		}
	}));
	return render(RadioGroup.Root, { props: { value: groupValue, children } });
}

afterEach(() => {
	cleanup();
	vi.useRealTimers();
});

describe('Checkbox', () => {
	it('names the checkbox with its visible label', () => {
		render(Checkbox, { props: { checked: false, label: 'Select report.pdf' } });
		const box = screen.getByRole('checkbox', { name: 'Select report.pdf' });
		expect(box.getAttribute('aria-checked')).toBe('false');
	});

	it('reports the mixed state when indeterminate', () => {
		render(Checkbox, { props: { checked: false, indeterminate: true, label: 'Select all' } });
		const box = screen.getByRole('checkbox', { name: 'Select all' });
		expect(box.getAttribute('aria-checked')).toBe('mixed');
	});

	it('toggles on box click and on label click', async () => {
		render(Checkbox, { props: { checked: false, label: 'Select report.pdf' } });
		const box = screen.getByRole('checkbox', { name: 'Select report.pdf' });
		await fireEvent.click(box);
		expect(box.getAttribute('aria-checked')).toBe('true');
		await fireEvent.click(screen.getByText('Select report.pdf'));
		expect(box.getAttribute('aria-checked')).toBe('false');
	});

	it('shows the check glyph only while checked', () => {
		const { unmount: unmountCheckbox } = render(Checkbox, { props: { checked: true, label: 'Keep copy' } });
		const box = screen.getByRole('checkbox', { name: 'Keep copy' });
		expect(box.querySelector('svg')).not.toBeNull();
		unmountCheckbox();
		render(Checkbox, { props: { checked: false, label: 'Drop copy' } });
		expect(screen.getByRole('checkbox', { name: 'Drop copy' }).querySelector('svg')).toBeNull();
	});

	it('links description and disabled reason through aria-describedby', () => {
		render(Checkbox, {
			props: {
				checked: false,
				label: 'Allow restart',
				description: 'Lets admins restart the server.',
				disabledReason: 'Only the owner can change this.'
			}
		});
		const box = screen.getByRole('checkbox', { name: 'Allow restart' });
		const describedBy = box.getAttribute('aria-describedby') ?? '';
		const describedText = describedBy
			.split(' ')
			.map((id) => document.getElementById(id)?.textContent)
			.join('|');
		expect(describedText).toContain('Lets admins restart the server.');
		expect(describedText).toContain('Only the owner can change this.');
		expect((box as HTMLButtonElement).disabled).toBe(true);
	});
});

describe('Switch', () => {
	it('is a switch named by its label', () => {
		render(Switch, { props: { checked: false, label: 'Keep a backup copy' } });
		const control = screen.getByRole('switch', { name: 'Keep a backup copy' });
		expect(control.getAttribute('aria-checked')).toBe('false');
	});

	it('flips on click and reports the new state to onchange', async () => {
		const onchange = vi.fn();
		render(Switch, { props: { checked: false, label: 'Keep a backup copy', onchange } });
		const control = screen.getByRole('switch', { name: 'Keep a backup copy' });
		await fireEvent.click(control);
		expect(onchange).toHaveBeenCalledWith(true);
		expect(control.getAttribute('aria-checked')).toBe('true');
	});

	it('marks itself busy and shows a small spinner while saving', async () => {
		vi.useFakeTimers();
		const { container } = render(Switch, { props: { checked: true, label: 'Trash', saving: true } });
		const control = screen.getByRole('switch', { name: 'Trash' });
		expect(control.getAttribute('aria-busy')).toBe('true');
		expect(container.querySelector('.spinner')).toBeNull();
		await vi.advanceTimersByTimeAsync(150);
		expect(container.querySelector('.spinner')).not.toBeNull();
	});

	it('links the description through aria-describedby', () => {
		render(Switch, {
			props: { checked: false, label: 'Trash', description: 'Keeps deleted files for 30 days.' }
		});
		const control = screen.getByRole('switch', { name: 'Trash' });
		const describedBy = control.getAttribute('aria-describedby') ?? '';
		expect(document.getElementById(describedBy)?.textContent).toBe('Keeps deleted files for 30 days.');
	});
});

describe('Radio', () => {
	it('names the radio with its label and reflects the group value', () => {
		renderRadioInGroup({ value: 'local', label: 'Local disk' }, 'local');
		const radio = screen.getByRole('radio', { name: 'Local disk' });
		expect(radio.getAttribute('aria-checked')).toBe('true');
	});

	it('selects the radio on click', async () => {
		renderRadioInGroup({ value: 'bucket', label: 'S3 bucket' }, 'local');
		const radio = screen.getByRole('radio', { name: 'S3 bucket' });
		expect(radio.getAttribute('aria-checked')).toBe('false');
		await fireEvent.click(radio);
		expect(radio.getAttribute('aria-checked')).toBe('true');
	});

	it('shows the recommended word beside the label when recommended', () => {
		renderRadioInGroup({ value: 'postgres', label: 'Postgres', recommended: true }, 'postgres');
		expect(screen.getByText(t('common.recommended'))).toBeTruthy();
	});

	it('omits the recommended word when not recommended', () => {
		renderRadioInGroup({ value: 'sqlite', label: 'SQLite' }, 'postgres');
		expect(screen.queryByText(t('common.recommended'))).toBeNull();
	});

	it('links the description through aria-describedby', () => {
		renderRadioInGroup({ value: 'aes', label: 'AES256', description: 'Encrypts files at rest.' }, 'aes');
		const radio = screen.getByRole('radio', { name: 'AES256' });
		const describedBy = radio.getAttribute('aria-describedby') ?? '';
		expect(document.getElementById(describedBy)?.textContent).toBe('Encrypts files at rest.');
	});
});

describe('Slider', () => {
	const formatFiles = (n: number) => `${n} files`;

	it('is a slider named by its label with the numeric value', () => {
		render(Slider, { props: { value: 5, min: 0, max: 10, step: 1, label: 'Files per page' } });
		const thumb = screen.getByRole('slider', { name: 'Files per page' });
		expect(thumb.getAttribute('aria-valuenow')).toBe('5');
	});

	it('shows the formatted value and exposes it as aria-valuetext', () => {
		render(Slider, {
			props: { value: 5, min: 0, max: 10, step: 1, label: 'Files per page', format: formatFiles }
		});
		expect(screen.getByText('5 files')).toBeTruthy();
		const thumb = screen.getByRole('slider', { name: 'Files per page' });
		expect(thumb.getAttribute('aria-valuetext')).toBe('5 files');
	});

	it('steps with ArrowRight and ArrowLeft', async () => {
		render(Slider, { props: { value: 5, min: 0, max: 10, step: 1, label: 'Files per page' } });
		const thumb = screen.getByRole('slider', { name: 'Files per page' });
		await fireEvent.keyDown(thumb, { key: 'ArrowRight' });
		expect(thumb.getAttribute('aria-valuenow')).toBe('6');
		await fireEvent.keyDown(thumb, { key: 'ArrowLeft' });
		expect(thumb.getAttribute('aria-valuenow')).toBe('5');
	});

	it('jumps to the ends with Home and End', async () => {
		render(Slider, { props: { value: 5, min: 0, max: 10, step: 1, label: 'Files per page' } });
		const thumb = screen.getByRole('slider', { name: 'Files per page' });
		await fireEvent.keyDown(thumb, { key: 'End' });
		expect(thumb.getAttribute('aria-valuenow')).toBe('10');
		await fireEvent.keyDown(thumb, { key: 'Home' });
		expect(thumb.getAttribute('aria-valuenow')).toBe('0');
	});
});
