<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/copy/t';
	import { migrate, preferences, type Preferences } from '$lib/stores/preferences.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { ThemePrefs } from '$lib/theme/controller.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ThemeEditor from '$lib/ui/organisms/ThemeEditor.svelte';

	const THEME_FIELDS = ['theme', 'contrast', 'density', 'motion', 'fontScale', 'accentHue', 'radius'] as const;

	function themeOf(source: Preferences): ThemePrefs {
		return {
			theme: source.theme,
			contrast: source.contrast,
			density: source.density,
			motion: source.motion,
			fontScale: source.fontScale,
			accentHue: source.accentHue,
			radius: source.radius
		};
	}

	function sameTheme(left: ThemePrefs, right: ThemePrefs): boolean {
		return THEME_FIELDS.every((field) => left[field] === right[field]);
	}

	function commit(previous: ThemePrefs, next: ThemePrefs): void {
		preferences.update(next);
		toasts.push({
			tone: 'ok',
			title: t('personal.appearance.saved'),
			action: {
				label: t('personal.undo'),
				run: () => {
					preferences.update(previous);
				}
			}
		});
	}

	function resetTheme(): void {
		draft = themeOf(migrate(undefined));
	}

	function exportTheme(format: 'css' | 'json', text: string): void {
		const url = URL.createObjectURL(new Blob([text], { type: 'css' === format ? 'text/css' : 'application/json' }));
		const link = document.createElement('a');
		link.href = url;
		link.download = `callfs-theme.${format}`;
		link.click();
		URL.revokeObjectURL(url);
	}

	let draft = $state<ThemePrefs>(themeOf(preferences.value));

	$effect(() => {
		const next = $state.snapshot(draft);
		untrack(() => {
			const current = themeOf(preferences.value);
			if (true === sameTheme(current, next)) {
				return;
			}
			commit(current, next);
		});
	});

	$effect(() => {
		const stored = themeOf(preferences.value);
		untrack(() => {
			if (true === sameTheme(draft, stored)) {
				return;
			}
			draft = stored;
		});
	});
</script>

<PageFrame title={t('personal.appearance.title')} job={t('personal.appearance.job')}>
	{#if 'loading' === preferences.status}
		<Skeleton shape="rect" height="24rem" />
	{:else if null !== preferences.error && 'error' === preferences.status}
		<ErrorState variant="region" error={preferences.error} />
	{:else}
		{#if null !== preferences.error}
			<InlineAlert tone="danger" title={preferences.error.title} body={preferences.error.body} />
		{/if}
		<ThemeEditor bind:prefs={draft} onreset={resetTheme} onexport={exportTheme} />
	{/if}
</PageFrame>
