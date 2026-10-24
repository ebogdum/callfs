<script lang="ts" module>
	export const TOKENS = ['--theme-preview-bg', '--theme-preview-border'] as const;
	export type ThemeExportFormat = 'css' | 'json';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ThemePrefs } from '$lib/theme/controller.svelte';
	import { accentRamp, checkRampContrast, type ColorMode } from '$lib/theme/ramp';
	import { formatNumber } from '$lib/format/number';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Input from '../atoms/Input.svelte';
	import Meter from '../atoms/Meter.svelte';
	import Select from '../atoms/Select.svelte';
	import Slider from '../atoms/Slider.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Textarea from '../atoms/Textarea.svelte';
	import DensityToggle from '../molecules/DensityToggle.svelte';
	import FormField from '../molecules/FormField.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';
	import SegmentedControl from '../molecules/SegmentedControl.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Grid from '../quarks/Grid.svelte';
	import Stack from '../quarks/Stack.svelte';
	import Surface from '../quarks/Surface.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		prefs: ThemePrefs;
		onreset?: () => void;
		onexport?: (format: ThemeExportFormat, text: string) => void;
		onimport?: (prefs: ThemePrefs) => void;
		class?: string;
	}

	type ParseResult = { ok: true; value: ThemePrefs } | { ok: false; error: string };

	const THEME_MODES = ['system', 'light', 'dark'] as const;
	const CONTRAST_MODES = ['standard', 'more'] as const;
	const DENSITIES = ['compact', 'default', 'comfortable'] as const;
	const MOTIONS = ['full', 'reduced', 'none'] as const;
	const RADII = ['sharp', 'medium', 'round'] as const;
	const FONT_SCALES = [0.875, 1, 1.125, 1.25] as const;
	const COLOR_MODES: readonly ColorMode[] = ['light', 'dark'];
	const ID = {
		mode: 'theme-mode',
		contrast: 'theme-contrast',
		accent: 'theme-accent',
		radius: 'theme-radius',
		density: 'theme-density',
		fontScale: 'theme-font-scale',
		motion: 'theme-motion',
		previewName: 'theme-preview-name',
		importText: 'theme-import'
	};

	let { prefs = $bindable(), onreset, onexport, onimport, class: className, ...rest }: Props = $props();

	let importText = $state('');
	let importError = $state<string | null>(null);
	let refusal = $state<{ hue: number; pairs: string[] } | null>(null);

	function isOneOf<T extends string>(options: readonly T[], value: unknown): value is T {
		return options.some((option) => option === value);
	}

	function isFontScale(value: number): value is (typeof FONT_SCALES)[number] {
		return FONT_SCALES.some((option) => option === value);
	}

	function isThemePrefs(value: unknown): value is ThemePrefs {
		if ('object' !== typeof value || null === value) {
			return false;
		}
		const record = value as Record<string, unknown>;
		return (
			isOneOf(THEME_MODES, record.theme) &&
			isOneOf(CONTRAST_MODES, record.contrast) &&
			isOneOf(DENSITIES, record.density) &&
			isOneOf(MOTIONS, record.motion) &&
			isOneOf(RADII, record.radius) &&
			'number' === typeof record.fontScale &&
			isFontScale(record.fontScale) &&
			'number' === typeof record.accentHue &&
			0 <= record.accentHue &&
			360 >= record.accentHue
		);
	}

	function parseThemePrefs(text: string): ParseResult {
		let raw: unknown;
		try {
			raw = JSON.parse(text);
		} catch {
			return { ok: false, error: t('theme.import.error') };
		}
		if (!isThemePrefs(raw)) {
			return { ok: false, error: t('theme.import.error') };
		}
		return { ok: true, value: raw };
	}

	function refusedPairs(hue: number): string[] {
		return COLOR_MODES.flatMap((mode) =>
			checkRampContrast(accentRamp(hue, mode), mode).failures.map((pair) =>
				t('theme.guard.pair', { mode: t(`theme.mode.${mode}`), pair: t(`theme.pair.${pair}`) })
			)
		);
	}

	function chooseHue(hue: number): void {
		const pairs = refusedPairs(hue);
		if (0 < pairs.length) {
			refusal = { hue, pairs };
			return;
		}
		refusal = null;
		prefs.accentHue = hue;
	}

	function chooseFontScale(next: string): void {
		const scale = Number(next);
		if (isFontScale(scale)) {
			prefs.fontScale = scale;
		}
	}

	function cssSnippet(next: ThemePrefs): string {
		const lightLines = accentRamp(next.accentHue, 'light').map((hex, index) => `  --a-${index + 1}: ${hex};`);
		const darkLines = accentRamp(next.accentHue, 'dark').map((hex, index) => `  --a-${index + 1}: ${hex};`);
		return [
			':root {',
			`  --font-scale: ${next.fontScale};`,
			...lightLines,
			'}',
			":root[data-theme='dark'] {",
			...darkLines,
			'}'
		].join('\n');
	}

	function exportCss(): void {
		onexport?.('css', cssSnippet(prefs));
	}

	function exportJson(): void {
		onexport?.('json', JSON.stringify(prefs, null, 2));
	}

	function applyImport(): void {
		const parsed = parseThemePrefs(importText);
		if (!parsed.ok) {
			importError = parsed.error;
			return;
		}
		importError = null;
		const pairs = refusedPairs(parsed.value.accentHue);
		if (0 < pairs.length) {
			refusal = { hue: parsed.value.accentHue, pairs };
			return;
		}
		refusal = null;
		Object.assign(prefs, parsed.value);
		onimport?.(parsed.value);
	}

	const rampRows = $derived([
		{ label: 'theme.preview.light_ramp', colors: accentRamp(prefs.accentHue, 'light') },
		{ label: 'theme.preview.dark_ramp', colors: accentRamp(prefs.accentHue, 'dark') }
	]);
	const modeOptions = THEME_MODES.map((value) => ({ value, label: t(`theme.mode.${value}`) }));
	const contrastOptions = CONTRAST_MODES.map((value) => ({ value, label: t(`theme.contrast.${value}`) }));
	const radiusOptions = RADII.map((value) => ({ value, label: t(`theme.radius.${value}`) }));
	const motionOptions = MOTIONS.map((value) => ({ value, label: t(`theme.motion.${value}`) }));
	const fontScaleOptions = FONT_SCALES.map((scale) => ({
		value: String(scale),
		label: t('theme.font_scale.percent', { value: formatNumber(scale * 100, 'decimal') })
	}));
	const formatAccent = (hue: number): string => t('theme.accent.degrees', { hue: formatNumber(hue, 'int') });
</script>

<section class={['theme-editor', className]} aria-label={t('theme.title')} {...rest}>
	<Stack gap="6">
		<p class="help">{t('theme.description')}</p>

		<Surface pad="card">
			<Grid min="16" gap="4">
				<FormField label={t('theme.mode.label')} for={ID.mode}>
					{#snippet control()}
						<SegmentedControl id={ID.mode} bind:value={prefs.theme} options={modeOptions} label={t('theme.mode.label')} />
					{/snippet}
				</FormField>
				<FormField label={t('theme.contrast.label')} for={ID.contrast}>
					{#snippet control()}
						<SegmentedControl id={ID.contrast} bind:value={prefs.contrast} options={contrastOptions} label={t('theme.contrast.label')} />
					{/snippet}
				</FormField>
				<div class="hue">
					<Slider
						id={ID.accent}
						bind:value={() => prefs.accentHue, chooseHue}
						min={0}
						max={360}
						step={1}
						label={t('theme.accent.label')}
						format={formatAccent}
					/>
					<p class="hue-help">{t('theme.accent.help')}</p>
				</div>
				<FormField label={t('theme.radius.label')} for={ID.radius}>
					{#snippet control()}
						<SegmentedControl id={ID.radius} bind:value={prefs.radius} options={radiusOptions} label={t('theme.radius.label')} />
					{/snippet}
				</FormField>
				<FormField label={t('theme.density.label')} for={ID.density}>
					{#snippet control()}
						<DensityToggle id={ID.density} bind:value={prefs.density} />
					{/snippet}
				</FormField>
				<FormField label={t('theme.font_scale.label')} for={ID.fontScale}>
					{#snippet control()}
						<Select id={ID.fontScale} bind:value={() => String(prefs.fontScale), chooseFontScale} options={fontScaleOptions} />
					{/snippet}
				</FormField>
				<FormField label={t('theme.motion.label')} for={ID.motion}>
					{#snippet control()}
						<SegmentedControl id={ID.motion} bind:value={prefs.motion} options={motionOptions} label={t('theme.motion.label')} />
					{/snippet}
				</FormField>
			</Grid>
			{#if null !== refusal}
				<InlineAlert
					tone="danger"
					title={t('theme.guard.title')}
					body={t('theme.guard.body', { hue: formatNumber(refusal.hue, 'int'), pairs: refusal.pairs.join('; ') })}
				/>
			{/if}
		</Surface>

		<Surface pad="card" class="preview">
			<Stack gap="4">
				<Stack gap="1">
					<h3 class="heading">{t('theme.preview.title')}</h3>
					<p class="help">{t('theme.preview.note')}</p>
				</Stack>
				<Stack gap="2">
					{#each rampRows as row (row.label)}
						<Stack gap="1">
							<span class="caption">{t(row.label)}</span>
							<Cluster gap="1" wrap={false}>
								{#each row.colors as color, index (index)}
									<span class="step" style:background-color={color} aria-hidden="true"></span>
								{/each}
							</Cluster>
						</Stack>
					{/each}
				</Stack>
				<Cluster gap="2">
					<Button variant="primary">{t('theme.preview.button_primary')}</Button>
					<Button variant="secondary">{t('theme.preview.button_secondary')}</Button>
					<Button variant="ghost">{t('theme.preview.button_ghost')}</Button>
					<Button variant="danger">{t('theme.preview.button_danger')}</Button>
				</Cluster>
				<Cluster gap="2">
					<StatusPill status="health.ok" />
					<StatusPill status="member.follower" />
				</Cluster>
				<table class="sample">
					<thead>
						<tr>
							<th scope="col">{t('theme.preview.table_name')}</th>
							<th scope="col">{t('theme.preview.table_size')}</th>
						</tr>
					</thead>
					<tbody>
						<tr>
							<td>{t('theme.preview.table_row_one')}</td>
							<td><StatusPill status="health.ok" /></td>
						</tr>
						<tr>
							<td>{t('theme.preview.table_row_two')}</td>
							<td><StatusPill status="member.follower" /></td>
						</tr>
					</tbody>
				</table>
				<FormField label={t('theme.preview.field_label')} for={ID.previewName} help={t('theme.preview.field_help')}>
					{#snippet control()}
						<Input id={ID.previewName} placeholder={t('theme.preview.field_label')} />
					{/snippet}
				</FormField>
				<Meter value={62} max={100} unit="percent" label={t('theme.preview.meter')} />
			</Stack>
		</Surface>

		<Surface pad="card">
			<Stack gap="4">
				<Cluster gap="2">
					<Button variant="secondary" onclick={exportCss}>{t('theme.export.css')}</Button>
					<Button variant="secondary" onclick={exportJson}>{t('theme.export.json')}</Button>
					<Button variant="ghost" onclick={() => onreset?.()}>{t('theme.reset.label')}</Button>
				</Cluster>
				<FormField label={t('theme.import.label')} for={ID.importText} error={importError ?? undefined}>
					{#snippet control()}
						<Textarea id={ID.importText} bind:value={importText} rows={4} />
					{/snippet}
				</FormField>
				<Cluster gap="2">
					<Button variant="secondary" onclick={applyImport}>{t('theme.import.apply')}</Button>
				</Cluster>
			</Stack>
		</Surface>
	</Stack>
</section>

<style>
	.heading {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
	}

	.help {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.hue {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
		min-inline-size: 0;
	}

	.hue-help {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.caption {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
	}

	.step {
		display: block;
		inline-size: var(--space-6);
		block-size: var(--space-4);
		border-radius: var(--radius-xs);
	}

	.sample {
		inline-size: 100%;
		border-collapse: separate;
		border-spacing: 0;
		font-size: var(--table-text);
	}

	.sample th,
	.sample td {
		padding-inline: var(--cell-px);
		block-size: var(--row-h);
		text-align: start;
		border-block-end: var(--space-px) solid var(--border-subtle);
	}

	.sample th {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		font-weight: var(--text-caption-weight);
	}

	.theme-editor :global(.preview) {
		background: var(--theme-preview-bg, var(--surface));
		border-color: var(--theme-preview-border, var(--border-subtle));
	}
</style>
