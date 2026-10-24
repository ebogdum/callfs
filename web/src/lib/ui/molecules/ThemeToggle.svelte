<script lang="ts" module>
	export const TOKENS = [
		'--theme-toggle-radius',
		'--theme-toggle-border',
		'--theme-toggle-fg',
		'--theme-toggle-selected'
	] as const;

	export type ThemeMode = 'system' | 'light' | 'dark';

	const THEME_MODES: readonly string[] = ['system', 'light', 'dark'];

	const isThemeMode = (candidate: string): candidate is ThemeMode =>
		-1 !== THEME_MODES.indexOf(candidate);
</script>

<script lang="ts">
	import { ToggleGroup } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import Icon from '$lib/ui/quarks/Icon.svelte';

	interface Props {
		value: ThemeMode;
		class?: string;
	}

	const OPTIONS = [
		{ value: 'system', icon: 'monitor', label: 'u080molecules.theme.system' },
		{ value: 'light', icon: 'sun', label: 'u080molecules.theme.light' },
		{ value: 'dark', icon: 'moon', label: 'u080molecules.theme.dark' }
	] as const;

	let { value = $bindable(), class: className }: Props = $props();

	const select = (next: string) => {
		if (isThemeMode(next)) value = next;
	};
</script>

<ToggleGroup.Root
	type="single"
	bind:value={() => value, select}
	aria-label={t('u080molecules.theme.group')}
	class="theme-toggle {className ?? ''}"
>
	{#each OPTIONS as option (option.value)}
		<ToggleGroup.Item value={option.value} class="theme-option">
			<Icon name={option.icon} size="sm" />
			<span>{t(option.label)}</span>
		</ToggleGroup.Item>
	{/each}
</ToggleGroup.Root>

<style>
	:global(.theme-toggle) {
		display: inline-flex;
		padding: var(--space-0-5);
		border: var(--hairline) solid var(--theme-toggle-border, var(--border-control));
		border-radius: var(--theme-toggle-radius, var(--radius-md));
	}

	:global(.theme-toggle) :global(.theme-option) {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		block-size: var(--control-h);
		min-inline-size: var(--target-min);
		padding-inline: var(--control-px);
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--theme-toggle-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		cursor: pointer;
	}

	:global(.theme-toggle) :global(.theme-option[data-state='on']) {
		background: var(--theme-toggle-selected, var(--surface-selected));
		color: var(--text);
	}

	:global(.theme-toggle) :global(.theme-option:active) {
		background: var(--surface-pressed);
	}

	:global(.theme-toggle) :global(.theme-option:focus-visible) {
		outline: var(--focus-width) solid var(--focus-ring-color);
		outline-offset: var(--space-0-5);
	}

	@media (hover: hover) {
		:global(.theme-toggle) :global(.theme-option:hover) {
			background: var(--surface-hover);
		}
	}
</style>
