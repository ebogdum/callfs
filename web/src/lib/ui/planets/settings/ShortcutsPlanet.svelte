<script lang="ts">
	import { t } from '$lib/copy/t';
	import type { KeyAlternative, KeymapOverrides } from '$lib/shortcuts/keymap';
	import { ShortcutManager, type ActiveShortcut } from '$lib/shortcuts/manager.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Kbd from '$lib/ui/atoms/Kbd.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Switch from '$lib/ui/atoms/Switch.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import SettingsCard from './SettingsCard.svelte';
	import type { ColumnDef } from '$lib/ui/types';

	const CLEARED_OVERRIDES = null as unknown as Record<string, string[]>;
	const MODIFIER_KEYS: readonly string[] = ['Shift', 'Control', 'Alt', 'Meta'];
	const KEY_NAMES: Record<string, string> = {
		Escape: 'esc',
		' ': 'space',
		ArrowUp: 'arrowup',
		ArrowDown: 'arrowdown',
		ArrowLeft: 'arrowleft',
		ArrowRight: 'arrowright'
	};

	function alternativesOf(overrides: Record<string, string[]>): KeymapOverrides {
		return Object.fromEntries(
			Object.entries(overrides).map(([id, keys]): [string, KeyAlternative[]] => [id, [keys]])
		);
	}

	function alternativeOf(event: KeyboardEvent): KeyAlternative {
		const key = KEY_NAMES[event.key] ?? event.key.toLowerCase();
		const tokens: string[] = [];
		if (event.metaKey || event.ctrlKey) {
			tokens.push('mod');
		}
		if (event.altKey) {
			tokens.push('alt');
		}
		if (event.shiftKey && (1 < key.length || /^[a-z]$/.test(key))) {
			tokens.push('shift');
		}
		tokens.push(key);
		return tokens;
	}

	function labelOf(id: string): string {
		return t(`personal.shortcut.${id}`);
	}

	const manager = new ShortcutManager();
	let recordingId = $state<string | null>(null);
	let conflict = $state<string | null>(null);
	const bindings = $derived(manager.effective());

	$effect(() => {
		const stored = preferences.value.keymapOverrides;
		if (null === stored) {
			manager.resetOverrides();
			return;
		}
		manager.setOverrides(alternativesOf(stored));
	});

	function startRecording(id: string): void {
		recordingId = id;
		conflict = null;
	}

	function cancelRecording(): void {
		recordingId = null;
		conflict = null;
	}

	function recordKey(event: KeyboardEvent): void {
		if (null === recordingId || true === MODIFIER_KEYS.includes(event.key) || event.isComposing) {
			return;
		}
		event.preventDefault();
		event.stopPropagation();
		if ('Escape' === event.key) {
			cancelRecording();
			return;
		}
		const id = recordingId;
		const keys = alternativeOf(event);
		const [firstConflict] = manager.conflictsFor(id, [keys]);
		if (undefined !== firstConflict) {
			conflict = t('personal.shortcuts.conflict', { name: labelOf(firstConflict) });
			return;
		}
		manager.setOverride(id, [keys]);
		preferences.update({ keymapOverrides: { ...preferences.value.keymapOverrides, [id]: keys } });
		cancelRecording();
	}

	function setSingleKey(enabled: boolean): void {
		preferences.update({ singleKeyShortcuts: enabled });
	}

	function resetAll(): void {
		const previous = $state.snapshot(preferences.value.keymapOverrides);
		manager.resetOverrides();
		preferences.update({ keymapOverrides: CLEARED_OVERRIDES });
		toasts.push({
			tone: 'ok',
			title: t('personal.shortcuts.reset.done'),
			action: {
				label: t('personal.undo'),
				run: () => {
					preferences.update({ keymapOverrides: previous });
				}
			}
		});
	}

	const columns: ColumnDef<ActiveShortcut>[] = [
		{ id: 'action', header: t('personal.shortcuts.column.action'), cell: actionCell, priority: 1 },
		{ id: 'scope', header: t('personal.shortcuts.column.scope'), cell: scopeCell },
		{ id: 'keys', header: t('personal.shortcuts.column.keys'), cell: keysCell, priority: 1 },
		{ id: 'change', header: t('personal.shortcuts.column.change'), cell: changeCell, priority: 1 }
	];
</script>

<svelte:window onkeydown={recordKey} />

{#snippet actionCell(row: ActiveShortcut)}
	<Text>{labelOf(row.id)}</Text>
{/snippet}

{#snippet scopeCell(row: ActiveShortcut)}
	<Text tone="muted">{t(`personal.scope.${row.scope}`)}</Text>
{/snippet}

{#snippet keysCell(row: ActiveShortcut)}
	<Cluster gap="2">
		{#each row.keys as alternative, index (index)}
			{#if 0 < index}<Text tone="muted">{t('personal.shortcuts.or')}</Text>{/if}
			<Kbd keys={alternative} />
		{/each}
	</Cluster>
{/snippet}

{#snippet changeCell(row: ActiveShortcut)}
	{#if row.id === recordingId}
		<Button variant="ghost" size="sm" onclick={cancelRecording}>{t('personal.shortcuts.cancel')}</Button>
	{:else}
		<Button
			variant="secondary"
			size="sm"
			aria-label={t('personal.shortcuts.change_for', { name: labelOf(row.id) })}
			onclick={() => startRecording(row.id)}>{t('personal.shortcuts.change')}</Button
		>
	{/if}
{/snippet}

<PageFrame title={t('personal.shortcuts.title')} job={t('personal.shortcuts.job')}>
	{#if 'loading' === preferences.status}
		<Skeleton shape="rect" height="6rem" />
		<Skeleton shape="rect" height="20rem" />
	{:else if null !== preferences.error && 'error' === preferences.status}
		<ErrorState variant="region" error={preferences.error} />
	{:else}
		{#if null !== preferences.error}
			<InlineAlert tone="danger" title={preferences.error.title} body={preferences.error.body} />
		{/if}
		<SettingsCard title={t('settings-screens.shortcuts.single.title')}>
			<Switch
				checked={preferences.value.singleKeyShortcuts}
				label={t('settings-screens.shortcuts.single.label')}
				description={t('personal.shortcuts.single.help')}
				onchange={setSingleKey}
			/>
		</SettingsCard>
		<SettingsCard title={t('personal.shortcuts.table.title')} description={t('settings-screens.shortcuts.table.desc')}>
			{#snippet actions()}
				<Button variant="secondary" iconStart="refresh" onclick={resetAll}>{t('personal.shortcuts.reset')}</Button>
			{/snippet}
			<div class="live" aria-live="polite">
				{#if null !== recordingId}
					<InlineAlert tone="info" title={t('personal.shortcuts.recording')} />
				{/if}
				{#if null !== conflict}
					<InlineAlert tone="warn" title={conflict} />
				{/if}
			</div>
			<DataTable
				rows={bindings}
				{columns}
				rowKey={(row: ActiveShortcut) => row.id}
				label={t('personal.shortcuts.table.title')}
				status="success"
			>
				{#snippet empty()}
					<Text tone="muted">{t('personal.shortcuts.empty')}</Text>
				{/snippet}
			</DataTable>
		</SettingsCard>
	{/if}
</PageFrame>

<style>
	.live {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.live:empty {
		display: none;
	}
</style>
