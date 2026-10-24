<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { t } from '$lib/copy/t';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import HelpText from '$lib/ui/atoms/HelpText.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Switch from '$lib/ui/atoms/Switch.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import Banner from '$lib/ui/molecules/Banner.svelte';
	import ConfirmInline from '$lib/ui/molecules/ConfirmInline.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ApiEquivalent from '$lib/ui/organisms/ApiEquivalent.svelte';
	import ConfigDiff from '$lib/ui/organisms/ConfigDiff.svelte';
	import Drawer from '$lib/ui/organisms/Drawer.svelte';
	import SettingRow, { type SettingRowStatus } from '$lib/ui/organisms/SettingRow.svelte';
	import SettingsSection, { type SettingsPreset } from '$lib/ui/organisms/SettingsSection.svelte';
	import {
		SETTINGS_GROUPS,
		entriesFor,
		entryFor,
		riskApplies,
		sectionsFor,
		type SettingsEntry,
		type SettingsGroup,
		type SettingsSectionDef
	} from './settings-map';
	import { SettingsDraft, loadSettings, metaFor, sameValue } from './settings-draft.svelte';

	interface Props {
		group: string;
	}

	let { group }: Props = $props();

	const CUSTOM_PRESET = 'custom';

	const isGroup = (value: string): value is SettingsGroup => SETTINGS_GROUPS.some((item) => item === value);

	const settingsQuery = createQuery({
		key: ['settings', 'config'],
		fetch: () => loadSettings(),
		staleMs: 60_000,
		revalidateOnFocus: false
	});

	const draft = new SettingsDraft();

	let reviewOpen = $state(false);
	let summaryElement = $state<HTMLElement | null>(null);

	$effect(() => {
		const result = settingsQuery.data;
		if (undefined !== result && result.ok) {
			const snapshot = result.value;
			untrack(() => draft.load(snapshot));
		}
	});

	const validGroup = $derived(isGroup(group));
	const sections = $derived(isGroup(group) ? sectionsFor(group) : []);
	const groupEntries = $derived(isGroup(group) ? entriesFor(group) : []);
	const pendingCount = $derived(draft.pending.length);
	const validationErrors = $derived(draft.validation?.errors ?? []);
	const loadFailure = $derived.by(() => {
		const result = settingsQuery.data;
		return undefined !== result && !result.ok ? result.error : null;
	});
	const validated = $derived(null !== draft.validation && draft.validation.valid);
	const restartKeys = $derived(draft.dirtyKeys.filter((key) => 'restart' === entryFor(key)?.apply));
	const secretKeys = $derived(draft.dirtyKeys.filter((key) => 'secret' === entryFor(key)?.type));
	const confirmWords = $derived(
		[...new Set(draft.dirtyKeys.map((key) => entryFor(key)?.confirmWord).filter((word): word is string => undefined !== word))]
	);
	const beforeChanges = $derived(Object.fromEntries(draft.dirtyKeys.map((key) => [key, draft.baseline[key]])));
	const afterChanges = $derived(Object.fromEntries(draft.dirtyKeys.map((key) => [key, draft.edits[key]])));
	const requestBody = $derived({ changes: { ...draft.edits }, expected_revision: draft.revision });

	function valueNow(key: string): unknown {
		const entry = entryFor(key);
		return undefined === entry ? undefined : draft.draftOf(entry);
	}

	function visible(entry: SettingsEntry): boolean {
		const dependency = entry.dependsOn;
		return undefined === dependency || sameValue(valueNow(dependency.key), dependency.equals);
	}

	function rowsOf(sectionId: string): SettingsEntry[] {
		return groupEntries.filter((entry) => sectionId === entry.section && visible(entry));
	}

	function allRowsOf(sectionId: string): SettingsEntry[] {
		return groupEntries.filter((entry) => sectionId === entry.section);
	}

	function modeOf(sectionId: string): 'autosave' | 'transactional' {
		return allRowsOf(sectionId).some((entry) => 'restart' === entry.apply || 'auth_reload' === entry.apply)
			? 'transactional'
			: 'autosave';
	}

	function dirtyCountOf(sectionId: string): number {
		return allRowsOf(sectionId).filter((entry) => draft.dirtyKeys.includes(entry.key)).length;
	}

	function keysOf(sectionId: string): string[] {
		return allRowsOf(sectionId).map((entry) => entry.key);
	}

	function presetsOf(section: SettingsSectionDef): SettingsPreset[] | undefined {
		return section.presets?.map((preset) => ({
			id: preset.id,
			label: t(`settings.preset.${section.id}.${preset.id}`),
			recommended: preset.recommended
		}));
	}

	function activePresetOf(section: SettingsSectionDef): string {
		const match = section.presets?.find((preset) =>
			Object.entries(preset.values).every(([key, value]) => sameValue(value, valueNow(key)))
		);
		return match?.id ?? CUSTOM_PRESET;
	}

	function applyPreset(section: SettingsSectionDef, presetId: string): void {
		const preset = section.presets?.find((item) => presetId === item.id);
		if (undefined !== preset) {
			draft.applyPreset(preset.values);
		}
	}

	function errorOf(key: string): string | undefined {
		return validationErrors.find((error) => key === error.path)?.message;
	}

	function statusOf(key: string): SettingRowStatus {
		if (undefined !== errorOf(key)) {
			return 'invalid';
		}
		if (draft.dirtyKeys.includes(key)) {
			return draft.saving ? 'saving' : 'dirty';
		}
		if (draft.pending.includes(key)) {
			return 'pending-restart';
		}
		if (draft.applied.includes(key)) {
			return 'runtime-applied';
		}
		return 'clean';
	}

	function metaOf(entry: SettingsEntry) {
		const schemaKey = draft.snapshot?.schema.keys.find((item) => entry.key === item.path);
		return metaFor(entry, draft.keyStates[entry.key], draft.baselineOf(entry), schemaKey?.min ?? null, schemaKey?.max ?? null);
	}

	function riskOf(entry: SettingsEntry): boolean {
		return riskApplies(entry, draft.draftOf(entry), draft.isSet(entry.key), draft.dirtyKeys.includes(entry.key));
	}

	function noteParams(entry: SettingsEntry): { n: number } {
		const value = draft.draftOf(entry);
		return { n: 'number' === typeof value ? value : 0 };
	}

	function errorLabel(path: string): string {
		const entry = entryFor(path);
		return undefined === entry ? path : t(entry.labelKey);
	}

	async function reviewChanges(): Promise<void> {
		const outcome = await draft.validate();
		if (!outcome.ok) {
			toasts.push({ tone: 'danger', title: outcome.error.title, body: outcome.error.body });
			return;
		}
		if (outcome.value.valid) {
			reviewOpen = true;
			return;
		}
		await tick();
		summaryElement?.focus();
	}

	async function saveDraft(): Promise<void> {
		const outcome = await draft.save();
		if (!outcome.ok) {
			return;
		}
		reviewOpen = false;
		toasts.push({
			tone: 'ok',
			title: t(0 < outcome.value.restart_required.length ? 'settings.saved.restart' : 'settings.saved.applies_now')
		});
		invalidate(['settings']);
	}

	function retry(): void {
		void settingsQuery.refetch();
	}
</script>

{#if validGroup}
	<PageFrame title={t(`settings.page.${group}`)} job={t(`settings-screens.group.job.${group}`)}>
		{#snippet secondary()}
			<Switch
				checked={preferences.value.showTechnical}
				label={t('settingsui.technical.show')}
				onchange={(next: boolean) => preferences.update({ showTechnical: next })}
			/>
			{#if 'logging' === group}
				<Button variant="secondary" iconStart="logs" onclick={() => void goto(`${base}/activity/logs`)}>
					{t('settings-screens.group.logs')}
				</Button>
			{/if}
		{/snippet}

		{#if undefined !== settingsQuery.data && settingsQuery.data.ok}
			{#if 0 < pendingCount}
				<Banner
					tone="warn"
					title={t('settings.pending.title', { count: pendingCount })}
					body={t('settings.pending.body')}
					action={draft.allowRemoteRestart ? { label: t('settings.restart.now'), run: () => void draft.restartNow() } : undefined}
				/>
			{/if}

			{#if 0 < validationErrors.length}
				<div bind:this={summaryElement} tabindex="-1">
					<InlineAlert tone="danger" title={t('settings.validation.title', { count: validationErrors.length })} body={t('settings.validation.body')}>
						{#snippet actions()}
							<Stack gap="1">
								{#each validationErrors as error (error.path)}
									<Link href={`#setting-${error.path}`}>{errorLabel(error.path)}</Link>
								{/each}
							</Stack>
						{/snippet}
					</InlineAlert>
				</div>
			{/if}

			{#each sections as section (section.id)}
				{#if 0 < rowsOf(section.id).length}
					<SettingsSection
						title={t(`settings.section.${section.id}`)}
						mode={modeOf(section.id)}
						dirtyCount={dirtyCountOf(section.id)}
						presets={presetsOf(section)}
						activePreset={activePresetOf(section)}
						onapplypreset={(id: string) => applyPreset(section, id)}
						onsave={() => void reviewChanges()}
						ondiscard={() => draft.discard(keysOf(section.id))}
					>
						{#snippet rows()}
							<div class="rows">
								{#each rowsOf(section.id) as entry (entry.key)}
									<div class="row">
										<SettingRow
											id={`setting-${entry.key}`}
											meta={metaOf(entry)}
											value={draft.baselineOf(entry)}
											bind:draft={() => draft.draftOf(entry), (next: unknown) => void draft.setValue(entry, next)}
											status={statusOf(entry.key)}
											error={errorOf(entry.key)}
											onreset={() => void draft.setValue(entry, entry.defaultValue)}
											technical={preferences.value.showTechnical}
										/>
										{#if entry.hasNote}
											<HelpText id={`setting-${entry.key}-note`}>{t(`settings.note.${entry.key}`, noteParams(entry))}</HelpText>
										{/if}
										{#if riskOf(entry)}
											<InlineAlert tone="warn" title={t(entry.riskKey ?? '')} />
										{/if}
									</div>
								{/each}
							</div>
						{/snippet}
					</SettingsSection>
				{/if}
			{/each}
		{:else if null !== loadFailure}
			<ErrorState variant="page" error={loadFailure} onretry={retry} />
		{:else}
			<Skeleton shape="rect" height="16rem" />
			<Skeleton shape="rect" height="10rem" />
		{/if}
	</PageFrame>
{:else}
	<EmptyState variant="not-available" icon="settings" title={t('settings.unknown.title')} body={t('settings.unknown.body')} />
{/if}

<Drawer bind:open={reviewOpen} title={t('settings.review.title', { count: draft.dirtyKeys.length })} width="form">
	<Stack gap="6">
		{#if draft.saveError}
			<InlineAlert tone="danger" title={draft.saveError.title} body={draft.saveError.body} />
		{/if}
		{#if validated}
			<InlineAlert tone="ok" title={t('settings.review.valid')} />
		{/if}
		<InlineAlert
			tone={0 < restartKeys.length ? 'warn' : 'info'}
			title={t(0 < restartKeys.length ? 'settings.review.restart' : 'settings.review.applies_now', { count: restartKeys.length })}
		/>
		<ConfigDiff before={beforeChanges} after={afterChanges} secretKeys={secretKeys} restartKeys={restartKeys} />
		<ApiEquivalent request={{ method: 'PUT', url: '/v1/admin/config', body: requestBody }} />
	</Stack>
	{#snippet footer()}
		{#if 0 < confirmWords.length}
			<ConfirmInline
				question={t('settings.review.confirm.question')}
				consequence={t('settings.review.confirm.body')}
				confirmLabel={t('settings.review.save', { count: draft.dirtyKeys.length })}
				typedWord={confirmWords.join(' ')}
				busy={draft.saving}
				onconfirm={() => void saveDraft()}
				oncancel={() => (reviewOpen = false)}
			/>
		{:else}
			<Button variant="primary" loading={draft.saving} onclick={() => void saveDraft()}>
				{t('settings.review.save', { count: draft.dirtyKeys.length })}
			</Button>
		{/if}
	{/snippet}
</Drawer>

<style>
	.rows {
		display: flex;
		flex-direction: column;
	}

	.row {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-block: var(--space-5);
	}

	.row:first-child {
		padding-block-start: 0;
	}

	.row:last-child {
		padding-block-end: 0;
	}

	.row + .row {
		border-block-start: var(--hairline) solid var(--border-subtle);
	}
</style>
