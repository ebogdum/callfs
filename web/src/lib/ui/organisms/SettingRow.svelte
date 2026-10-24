<script lang="ts" module>
	import type { ChoiceOption } from '../molecules/RadioGroup.svelte';

	export const TOKENS = ['--setting-row-fg'] as const;

	export type SettingType = 'bool' | 'enum' | 'number' | 'duration' | 'size' | 'text' | 'secret' | 'map' | 'list' | 'path';
	export type SettingRestart = 'restart' | 'runtime';
	export type SettingSource = 'default' | 'file' | 'env' | 'runtime';
	export type SettingRowStatus = 'clean' | 'dirty' | 'invalid' | 'saving' | 'saved' | 'pending-restart' | 'runtime-applied';

	export interface SettingMeta {
		key: string;
		type: SettingType;
		label: string;
		effect: string;
		restart: SettingRestart;
		source: SettingSource;
		valueText: string;
		defaultValue: unknown;
		defaultText: string;
		options?: ChoiceOption[];
		min?: number;
		max?: number;
		step?: number;
		unit?: string;
		isSet?: boolean;
		optional?: boolean;
		limitLabel?: string;
	}

	const OWN_HELP_TYPES: SettingType[] = ['bool', 'enum', 'number', 'text', 'map'];
	const OPTIONAL_SIZE_START = 64 * 1024 * 1024;

	const SOURCE_COPY: Record<SettingSource, string> = {
		default: 'settingsui.source.default',
		file: 'settingsui.source.file',
		env: 'settingsui.source.env',
		runtime: 'settingsui.source.runtime',
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Badge from '../atoms/Badge.svelte';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Input from '../atoms/Input.svelte';
	import NumberInput from '../atoms/NumberInput.svelte';
	import Switch from '../atoms/Switch.svelte';
	import Text from '../atoms/Text.svelte';
	import DurationInput from '../molecules/DurationInput.svelte';
	import EnumField from '../molecules/EnumField.svelte';
	import FormField from '../molecules/FormField.svelte';
	import KeyValueEditor from '../molecules/KeyValueEditor.svelte';
	import ListEditor from '../molecules/ListEditor.svelte';
	import PathInput from '../molecules/PathInput.svelte';
	import SecretInput from '../molecules/SecretInput.svelte';
	import SizeInput from '../molecules/SizeInput.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Dot from '../quarks/Dot.svelte';
	import Icon from '../quarks/Icon.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		meta: SettingMeta;
		value: unknown;
		draft: unknown;
		status: SettingRowStatus;
		error?: string;
		onreset?: () => void;
		ondocs?: (key: string) => void;
		technical?: boolean;
		class?: string;
	}

	let { meta, value, draft = $bindable(), status, error, onreset, ondocs, technical = false, class: className, ...rest }: Props = $props();

	const id = $props.id();
	const controlId = $derived(`${id}-control`);
	const errorId = $derived(`${id}-error`);
	const helpId = $derived(`${id}-help`);

	const isChanged = $derived(JSON.stringify(draft) !== JSON.stringify(value));
	const isDefault = $derived(JSON.stringify(draft) === JSON.stringify(meta.defaultValue));
	const isLocked = $derived('env' === meta.source);
	const hasOwnHelp = $derived(OWN_HELP_TYPES.includes(meta.type));
	const envName = $derived(`CALLFS_${meta.key.toUpperCase().replaceAll('.', '__')}`);
	const restartsNow = $derived('restart' === meta.restart);

	function setDraft(next: unknown): void {
		draft = next;
	}

	function asBoolean(): boolean {
		return true === draft;
	}

	function asText(): string {
		return 'string' === typeof draft ? draft : '';
	}

	function asNumber(): number | null {
		return 'number' === typeof draft ? draft : null;
	}

	function asBytes(): number {
		return 'number' === typeof draft ? draft : 0;
	}

	function asItems(): string[] {
		return Array.isArray(draft) ? draft.filter((item): item is string => 'string' === typeof item) : [];
	}

	function asEntries(): Record<string, string> {
		if (null === draft || 'object' !== typeof draft || Array.isArray(draft)) {
			return {};
		}
		return Object.fromEntries(Object.entries(draft).filter((entry): entry is [string, string] => 'string' === typeof entry[1]));
	}
</script>

<div class={['setting-row', className]} data-status={status} data-source={meta.source} data-type={meta.type} {...rest}>
	<Stack gap="2">
		{#if isLocked}
			<Cluster gap="2" align="center">
				<Icon name="lock" size="sm" />
				<Text tone="muted" size="sm">{t('settingsui.env.locked', { name: envName })}</Text>
			</Cluster>
			<Text>{meta.valueText}</Text>
		{:else if 'bool' === meta.type}
			<Switch bind:checked={asBoolean, setDraft} label={meta.label} description={meta.effect} saving={'saving' === status} />
		{:else if 'enum' === meta.type}
			<EnumField bind:value={asText, setDraft} label={meta.label} help={meta.effect} options={meta.options ?? []} />
		{:else if 'number' === meta.type}
			<FormField label={meta.label} for={controlId} help={meta.effect}>
				{#snippet control(context)}
					<NumberInput
						id={controlId}
						aria-describedby={context.describedBy}
						bind:value={asNumber, setDraft}
						min={meta.min}
						max={meta.max}
						step={meta.step}
						unit={meta.unit}
					/>
				{/snippet}
			</FormField>
		{:else if 'text' === meta.type}
			<FormField label={meta.label} for={controlId} help={meta.effect}>
				{#snippet control(context)}
					<Input id={controlId} aria-describedby={context.describedBy} bind:value={asText, setDraft} mono />
				{/snippet}
			</FormField>
		{:else if 'map' === meta.type}
			<FormField label={meta.label} for={controlId} help={meta.effect}>
				{#snippet control(context)}
					<KeyValueEditor
						aria-describedby={context.describedBy}
						bind:entries={asEntries, setDraft}
						keyLabel={t('settingsui.map.key')}
						valueLabel={t('settingsui.map.value')}
						addLabel={t('settingsui.map.add')}
					/>
				{/snippet}
			</FormField>
		{:else if 'duration' === meta.type}
			<DurationInput label={meta.label} min={'number' === typeof meta.min ? `${meta.min}s` : undefined} bind:value={asText, setDraft} />
		{:else if 'size' === meta.type && meta.optional}
			<Switch checked={null !== draft && undefined !== draft} label={meta.limitLabel ?? meta.label} onchange={(on: boolean) => setDraft(on ? OPTIONAL_SIZE_START : null)} />
			{#if null !== draft && undefined !== draft}
				<SizeInput label={meta.label} bind:value={asBytes, setDraft} />
			{/if}
		{:else if 'size' === meta.type}
			<SizeInput label={meta.label} bind:value={asBytes, setDraft} />
		{:else if 'secret' === meta.type}
			<SecretInput label={meta.label} isSet={meta.isSet ?? false} bind:value={asText, setDraft} generate />
		{:else if 'path' === meta.type}
			<PathInput label={meta.label} bind:value={asText, setDraft} />
		{:else}
			<ListEditor
				label={meta.label}
				itemLabel={t('settingsui.list.item')}
				addLabel={t('settingsui.list.add')}
				bind:items={asItems, setDraft}
			/>
		{/if}

		{#if !isLocked && !hasOwnHelp}
			<HelpText id={helpId}>{meta.effect}</HelpText>
		{/if}

		{#if 'invalid' === status && error}
			<FieldError id={errorId} message={error} />
		{/if}

		<div class="row-status" role="status" aria-live="polite">
			{#if 'saving' === status}
				<Text tone="muted" size="sm">{t('settingsui.status.saving')}</Text>
			{:else if 'saved' === status}
				<Text tone="ok" size="sm">{t('settingsui.status.saved')}</Text>
			{:else if 'pending-restart' === status}
				<Text tone="warn" size="sm">{t('settingsui.status.pending_restart')}</Text>
			{:else if 'runtime-applied' === status}
				<Text tone="ok" size="sm">{t('settingsui.status.runtime_applied')}</Text>
			{/if}
		</div>

		<div class="row-meta">
			<div class="row-facts">
				{#if isChanged}
					<span class="changed"><Dot tone="accent" />{t('settingsui.status.changed')}</span>
				{/if}
				<Badge tone={'env' === meta.source ? 'info' : 'neutral'}>{t(SOURCE_COPY[meta.source])}</Badge>
				<Badge tone={restartsNow ? 'warn' : 'ok'}>{restartsNow ? t('settingsui.restart.needed') : t('settingsui.restart.applies')}</Badge>
				<span class="fact">{t('settingsui.value.current', { value: meta.valueText })}</span>
				<span class="fact">{t('settingsui.value.default', { value: meta.defaultText })}</span>
			</div>
			<div class="row-actions">
				{#if !isDefault && !isLocked}
					<Button variant="ghost" size="sm" onclick={() => onreset?.()}>{t('settingsui.reset')}</Button>
				{/if}
				{#if ondocs}
					<Button variant="ghost" size="sm" onclick={() => ondocs?.(meta.key)}>{t('settingsui.docs')}</Button>
				{/if}
			</div>
		</div>

		{#if technical}
			<Cluster gap="2" align="center">
				<Text tone="muted" size="sm">{t('settingsui.technical.key')}</Text>
				<Text mono size="sm">{meta.key}</Text>
				<CopyButton value={meta.key} label={t('settingsui.technical.copy_key')} />
			</Cluster>
			{#if isLocked}
				<Cluster gap="2" align="center">
					<Text tone="muted" size="sm">{t('settingsui.technical.variable')}</Text>
					<Text mono size="sm">{envName}</Text>
					<CopyButton value={envName} label={t('settingsui.technical.copy_variable')} />
				</Cluster>
			{/if}
		{/if}
	</Stack>
</div>

<style>
	.setting-row {
		min-inline-size: 0;
		color: var(--setting-row-fg, var(--text));
	}

	.row-status:empty {
		display: none;
	}

	.row-meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
		padding-block-start: var(--space-1);
	}

	.row-facts,
	.row-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.row-actions {
		gap: var(--space-1);
	}

	.fact,
	.changed {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		overflow-wrap: anywhere;
	}

	.changed {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--accent-text);
		font-weight: var(--text-control-weight);
	}
</style>
