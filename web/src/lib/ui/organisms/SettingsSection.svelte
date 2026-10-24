<script lang="ts" module>
	export const TOKENS = ['--settings-save-bar-bg', '--settings-save-bar-inset'] as const;

	export type SettingsSectionMode = 'autosave' | 'transactional';

	export interface SettingsPreset {
		id: string;
		label: string;
		description?: string;
		recommended?: boolean;
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Heading from '../atoms/Heading.svelte';
	import Text from '../atoms/Text.svelte';
	import RadioGroup, { type ChoiceOption } from '../molecules/RadioGroup.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import Surface from '../quarks/Surface.svelte';

	const CUSTOM_PRESET = 'custom';
	const UNDO_WINDOW_MS = 10000;

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		title: string;
		description?: string;
		rows: Snippet;
		presets?: SettingsPreset[];
		activePreset?: string;
		mode: SettingsSectionMode;
		dirtyCount: number;
		saved?: boolean;
		onapplypreset?: (id: string) => void;
		onsave?: () => void;
		ondiscard?: () => void;
		onundo?: () => void;
		class?: string;
	}

	let {
		title,
		description,
		rows,
		presets = [],
		activePreset = $bindable(CUSTOM_PRESET),
		mode,
		dirtyCount,
		saved = false,
		onapplypreset,
		onsave,
		ondiscard,
		onundo,
		class: className,
		...rest
	}: Props = $props();

	const headingId = $props.id();

	const presetOptions: ChoiceOption[] = $derived([
		...presets.map((preset) => ({
			value: preset.id,
			label: preset.label,
			description: preset.description,
			recommended: preset.recommended,
		})),
		{ value: CUSTOM_PRESET, label: t('settingsui.presets.custom') },
	]);

	let undoOpen = $state(false);

	$effect(() => {
		if (saved && 'autosave' === mode) {
			undoOpen = true;
			const timer = setTimeout(() => {
				undoOpen = false;
			}, UNDO_WINDOW_MS);
			return () => clearTimeout(timer);
		}
		undoOpen = false;
	});

	function currentPreset(): string {
		return activePreset;
	}

	function selectPreset(next: string): void {
		activePreset = next;
		if (CUSTOM_PRESET !== next) {
			onapplypreset?.(next);
		}
	}
</script>

<section class={['settings-section', className]} aria-labelledby={headingId} {...rest}>
	<Surface pad="card" radius="lg">
		<Stack gap="4">
			<Stack gap="1-5">
				<Heading level={2} size="title-sm" id={headingId}>{title}</Heading>
				{#if description}
					<Text tone="muted" size="sm">{description}</Text>
				{/if}
			</Stack>
			{#if 0 < presets.length}
				<RadioGroup bind:value={currentPreset, selectPreset} options={presetOptions} label={t('settingsui.presets.label')} />
			{/if}
			<Stack gap="4">
				{@render rows()}
			</Stack>
			{#if 'autosave' === mode && saved}
				<div role="status">
					<Cluster gap="2" align="center">
						<Text tone="ok" size="sm">{t('settingsui.autosave.saved')}</Text>
						{#if undoOpen}
							<Button variant="ghost" size="sm" onclick={() => onundo?.()}>{t('settingsui.autosave.undo')}</Button>
						{/if}
					</Cluster>
				</div>
			{/if}
			{#if 'transactional' === mode && 0 < dirtyCount}
				<div class="save-bar" role="region" aria-label={t('settingsui.save.region')}>
					<Cluster gap="2" justify="space-between" align="center">
						<Text>{t('settingsui.save.count', { count: dirtyCount })}</Text>
						<Cluster gap="2" align="center">
							<Button variant="ghost" onclick={() => ondiscard?.()}>{t('settingsui.save.discard')}</Button>
							<Button variant="primary" onclick={() => onsave?.()}>{t('settingsui.save.review')}</Button>
						</Cluster>
					</Cluster>
				</div>
			{/if}
		</Stack>
	</Surface>
</section>

<style>
	.settings-section {
		min-inline-size: 0;
	}

	.save-bar {
		position: sticky;
		inset-block-end: var(--settings-save-bar-inset, var(--space-4));
		padding: var(--space-3);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--settings-save-bar-bg, var(--surface-overlay));
	}
</style>
