import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { ThemePrefs } from '$lib/theme/controller.svelte';
import { accentRamp } from '$lib/theme/ramp';
import ThemeEditor from './ThemeEditor.svelte';

const guard = vi.hoisted(() => ({ blockedSolid: '' }));

vi.mock('$lib/theme/ramp', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/theme/ramp')>();
	return {
		...actual,
		checkRampContrast: (ramp: readonly string[], mode: 'light' | 'dark') =>
			guard.blockedSolid === ramp[8] && 'light' === mode
				? { ok: false, failures: ['text-on-accent'] }
				: actual.checkRampContrast(ramp, mode)
	};
});

const BLOCKED_HUE = 200;

const basePrefs = (): ThemePrefs => ({
	theme: 'system',
	contrast: 'standard',
	density: 'default',
	motion: 'full',
	fontScale: 1,
	accentHue: 192,
	radius: 'medium'
});

const renderEditor = (prefs: ThemePrefs) => {
	const onreset = vi.fn();
	const onexport = vi.fn();
	const onimport = vi.fn();
	const view = render(ThemeEditor, { props: { prefs, onreset, onexport, onimport } });
	return { ...view, onreset, onexport, onimport };
};

const importButton = () => screen.getByRole('button', { name: t('theme.import.apply') });

const typeImport = (container: HTMLElement, text: string) => {
	const textarea = container.querySelector('textarea') as HTMLTextAreaElement;
	fireEvent.input(textarea, { target: { value: text } });
};

describe('ThemeEditor', () => {
	beforeEach(() => {
		guard.blockedSolid = accentRamp(BLOCKED_HUE, 'light')[8];
	});

	it('names the editor heading and labels every control', () => {
		renderEditor(basePrefs());

		expect(screen.getByRole('region', { name: t('theme.title') })).toBeTruthy();
		expect(screen.getByText(t('theme.mode.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.contrast.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.accent.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.radius.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.density.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.font_scale.label'))).toBeTruthy();
		expect(screen.getByText(t('theme.motion.label'))).toBeTruthy();
	});

	it('writes a chosen mode into prefs', () => {
		const prefs = basePrefs();
		renderEditor(prefs);

		fireEvent.click(screen.getByText(t('theme.mode.dark')));

		expect(prefs.theme).toBe('dark');
	});

	it('writes a chosen radius personality into prefs', () => {
		const prefs = basePrefs();
		renderEditor(prefs);

		fireEvent.click(screen.getByText(t('theme.radius.round')));

		expect(prefs.radius).toBe('round');
	});

	it('writes a chosen motion mode into prefs', () => {
		const prefs = basePrefs();
		renderEditor(prefs);

		fireEvent.click(screen.getByText(t('theme.motion.none')));

		expect(prefs.motion).toBe('none');
	});

	it('writes a chosen font scale into prefs', () => {
		const prefs = basePrefs();
		renderEditor(prefs);

		fireEvent.change(screen.getByLabelText(t('theme.font_scale.label')), { target: { value: '1.25' } });

		expect(prefs.fontScale).toBe(1.25);
	});

	it('refuses an imported accent hue that breaks contrast and names the failing pair', () => {
		const prefs = basePrefs();
		const { container, onimport } = renderEditor(prefs);

		typeImport(container, JSON.stringify({ ...basePrefs(), accentHue: BLOCKED_HUE }));
		fireEvent.click(importButton());

		const pair = t('theme.guard.pair', {
			mode: t('theme.mode.light'),
			pair: t('theme.pair.text-on-accent')
		});
		expect(screen.getByText(t('theme.guard.title'))).toBeTruthy();
		expect(screen.getByText(pair, { exact: false })).toBeTruthy();
		expect(prefs.accentHue).toBe(192);
		expect(onimport).not.toHaveBeenCalled();
	});

	it('accepts a valid import, applies it and emits it', () => {
		const prefs = basePrefs();
		const { container, onimport } = renderEditor(prefs);
		const imported = { ...basePrefs(), theme: 'dark', fontScale: 1.125 };

		typeImport(container, JSON.stringify(imported));
		fireEvent.click(importButton());

		expect(prefs.theme).toBe('dark');
		expect(prefs.fontScale).toBe(1.125);
		expect(onimport).toHaveBeenCalledWith(imported);
	});

	it('shows the import error for malformed JSON and emits nothing', () => {
		const { container, onimport } = renderEditor(basePrefs());

		typeImport(container, 'not json');
		fireEvent.click(importButton());

		expect(screen.getByText(t('theme.import.error'))).toBeTruthy();
		expect(onimport).not.toHaveBeenCalled();
	});

	it('shows the import error for JSON that is not a theme', () => {
		const { container, onimport } = renderEditor(basePrefs());

		typeImport(container, JSON.stringify({ theme: 'neon' }));
		fireEvent.click(importButton());

		expect(screen.getByText(t('theme.import.error'))).toBeTruthy();
		expect(onimport).not.toHaveBeenCalled();
	});

	it('emits the CSS snippet with both accent ramps', () => {
		const prefs = basePrefs();
		const { onexport } = renderEditor(prefs);

		fireEvent.click(screen.getByRole('button', { name: t('theme.export.css') }));

		expect(onexport).toHaveBeenCalledWith('css', expect.stringContaining(':root {'));
		expect(onexport).toHaveBeenCalledWith('css', expect.stringContaining(`--a-1: ${accentRamp(192, 'light')[0]};`));
		expect(onexport).toHaveBeenCalledWith('css', expect.stringContaining(`--a-12: ${accentRamp(192, 'dark')[11]};`));
	});

	it('emits the JSON export of the current prefs', () => {
		const prefs = basePrefs();
		const { onexport } = renderEditor(prefs);

		fireEvent.click(screen.getByRole('button', { name: t('theme.export.json') }));

		expect(onexport).toHaveBeenCalledWith('json', JSON.stringify(prefs, null, 2));
	});

	it('calls onreset from the reset button', () => {
		const { onreset } = renderEditor(basePrefs());

		fireEvent.click(screen.getByRole('button', { name: t('theme.reset.label') }));

		expect(onreset).toHaveBeenCalledTimes(1);
	});

	it('renders the preview controls and twelve swatches per accent ramp', () => {
		const { container } = renderEditor(basePrefs());

		expect(screen.getByRole('button', { name: t('theme.preview.button_primary') })).toBeTruthy();
		expect(screen.getByRole('table')).toBeTruthy();
		expect(screen.getByText(t('theme.preview.light_ramp'))).toBeTruthy();
		expect(container.querySelectorAll('.step')).toHaveLength(24);
	});
});
