import {
	getConfig,
	patchRuntime,
	save as saveChanges,
	schema,
	validate as validateChanges,
	type SaveResult
} from '$lib/api/admin/config';
import { restart } from '$lib/api/admin/status';
import type { ConfigSchema, ConfigView, Result, ValidateResult } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { formatBytes } from '$lib/format/bytes';
import { formatDuration, parseGoDuration } from '$lib/format/duration';
import { formatNumber } from '$lib/format/number';
import { toasts } from '$lib/stores/toasts.svelte';
import type { ChoiceOption } from '$lib/ui/molecules/RadioGroup.svelte';
import type { SettingMeta, SettingSource } from '$lib/ui/organisms/SettingRow.svelte';
import type { UiError } from '$lib/ui/types';
import { entryFor, type SettingsEntry } from './settings-map';

export interface SettingsSnapshot {
	view: ConfigView;
	schema: ConfigSchema;
}

type ConfigKeyState = ConfigView['keys'][number];

export async function loadSettings(): Promise<Result<SettingsSnapshot, UiError>> {
	const [view, shape] = await Promise.all([getConfig(), schema()]);
	if (!view.ok) {
		return view;
	}
	if (!shape.ok) {
		return shape;
	}
	return { ok: true, value: { view: view.value, schema: shape.value } };
}

export function sameValue(left: unknown, right: unknown): boolean {
	return JSON.stringify(left) === JSON.stringify(right);
}

function countOf(value: unknown): number {
	if (Array.isArray(value)) {
		return value.length;
	}
	if (null === value || 'object' !== typeof value) {
		return 0;
	}
	return Object.keys(value).length;
}

export function describeValue(entry: SettingsEntry, value: unknown, isSet: boolean): string {
	const notSet = t('settings.value.not_set');
	if ('secret' === entry.type) {
		return isSet ? t('settings.value.set') : notSet;
	}
	if ('bool' === entry.type) {
		return true === value ? t('settings.value.on') : t('settings.value.off');
	}
	if ('enum' === entry.type) {
		return 'string' === typeof value && '' !== value ? t(`settings.option.${entry.key}.${value}`) : notSet;
	}
	if ('duration' === entry.type && 'string' === typeof value) {
		const parsed = parseGoDuration(value);
		if (!parsed.ok) {
			return value;
		}
		if (entry.zeroLabel && 0 === parsed.value) {
			return t(`settings.zero.${entry.key}`);
		}
		return formatDuration(parsed.value, 'long');
	}
	if ('size' === entry.type && 'number' === typeof value) {
		if (entry.zeroLabel && 0 === value) {
			return t(`settings.zero.${entry.key}`);
		}
		return formatBytes(value).text;
	}
	if ('number' === entry.type && 'number' === typeof value) {
		return formatNumber(value);
	}
	if ('map' === entry.type || 'list' === entry.type) {
		return t('settings.value.count', { count: countOf(value) });
	}
	return 'string' === typeof value && '' !== value ? value : notSet;
}

export function metaFor(
	entry: SettingsEntry,
	state: ConfigKeyState | undefined,
	value: unknown,
	schemaMin: number | null,
	schemaMax: number | null
): SettingMeta {
	const isSet = true === state?.set;
	const source: SettingSource = 'env' === entry.apply ? 'env' : (state?.source ?? 'default');
	const options: ChoiceOption[] | undefined = entry.options?.map((option) => ({
		value: option,
		label: t(`settings.option.${entry.key}.${option}`),
		recommended: entry.recommended === option
	}));
	return {
		key: entry.key,
		type: entry.type,
		label: t(entry.labelKey),
		effect: t(entry.effectKey),
		restart: 'restart' === entry.apply || 'env' === entry.apply ? 'restart' : 'runtime',
		source,
		valueText: describeValue(entry, value, isSet),
		defaultValue: entry.defaultValue,
		defaultText: describeValue(entry, entry.defaultValue, false),
		options,
		min: schemaMin ?? entry.min,
		max: schemaMax ?? entry.max,
		isSet
	};
}

export class SettingsDraft {
	snapshot = $state<SettingsSnapshot | null>(null);
	baseline = $state<Record<string, unknown>>({});
	keyStates = $state<Record<string, ConfigKeyState>>({});
	edits = $state<Record<string, unknown>>({});
	pending = $state<string[]>([]);
	applied = $state<string[]>([]);
	revision = $state('');
	validation = $state<ValidateResult | null>(null);
	saving = $state(false);
	saveError = $state<UiError | null>(null);
	readonly dirtyKeys = $derived(Object.keys(this.edits));
	readonly allowRemoteRestart = $derived(true === this.baseline['ui.allow_remote_restart']);

	load(snapshot: SettingsSnapshot): void {
		const baseline: Record<string, unknown> = {};
		const keyStates: Record<string, ConfigKeyState> = {};
		for (const state of snapshot.view.keys) {
			baseline[state.path] = state.value;
			keyStates[state.path] = state;
		}
		this.snapshot = snapshot;
		this.baseline = baseline;
		this.keyStates = keyStates;
		this.pending = [...snapshot.view.pending];
		this.edits = {};
		this.applied = [];
		this.validation = null;
		this.saveError = null;
	}

	baselineOf(entry: SettingsEntry): unknown {
		return Object.hasOwn(this.baseline, entry.key) ? this.baseline[entry.key] : entry.defaultValue;
	}

	draftOf(entry: SettingsEntry): unknown {
		if (Object.hasOwn(this.edits, entry.key)) {
			return this.edits[entry.key];
		}
		return this.baselineOf(entry);
	}

	isSet(key: string): boolean {
		return Object.hasOwn(this.edits, key) || true === this.keyStates[key]?.set;
	}

	isLocked(key: string): boolean {
		return 'env' === this.keyStates[key]?.source;
	}

	isEnvEntry(entry: SettingsEntry): boolean {
		return 'env' === entry.apply || this.isLocked(entry.key);
	}

	async setValue(entry: SettingsEntry, value: unknown): Promise<void> {
		if (this.isEnvEntry(entry)) {
			return;
		}
		if ('runtime' === entry.apply) {
			await this.autosave(entry, value);
			return;
		}
		this.stage(entry, value);
	}

	stage(entry: SettingsEntry, value: unknown): void {
		const isEmptySecret = 'secret' === entry.type && (null === value || '' === value);
		const dropEdit = isEmptySecret || (!('secret' === entry.type) && sameValue(value, this.baseline[entry.key]));
		const hasEdit = Object.hasOwn(this.edits, entry.key);
		if (dropEdit && !hasEdit) {
			return;
		}
		if (!dropEdit && hasEdit && sameValue(value, this.edits[entry.key])) {
			return;
		}
		const next = { ...this.edits };
		if (dropEdit) {
			delete next[entry.key];
		} else {
			next[entry.key] = value;
		}
		this.edits = next;
	}

	discard(keys: readonly string[]): void {
		const next = { ...this.edits };
		for (const key of keys) {
			delete next[key];
		}
		this.edits = next;
	}

	applyPreset(values: Record<string, unknown>): void {
		for (const [key, value] of Object.entries(values)) {
			const entry = entryFor(key);
			if (undefined !== entry) {
				void this.setValue(entry, value);
			}
		}
	}

	async autosave(entry: SettingsEntry, value: unknown): Promise<void> {
		const previous = this.baselineOf(entry);
		const outcome = await patchRuntime({ [entry.key]: value });
		if (!outcome.ok) {
			toasts.push({ tone: 'danger', title: outcome.error.title, body: outcome.error.body });
			return;
		}
		this.baseline = { ...this.baseline, [entry.key]: value };
		this.applied = [...new Set([...this.applied, entry.key])];
		toasts.push({
			tone: 'ok',
			title: t('settings.saved.applies_now'),
			action: { label: t('settings.undo'), run: () => void this.undo(entry, previous) }
		});
	}

	async undo(entry: SettingsEntry, previous: unknown): Promise<void> {
		const outcome = await patchRuntime({ [entry.key]: previous });
		if (!outcome.ok) {
			toasts.push({ tone: 'danger', title: outcome.error.title, body: outcome.error.body });
			return;
		}
		this.baseline = { ...this.baseline, [entry.key]: previous };
		this.applied = this.applied.filter((key) => entry.key !== key);
		toasts.push({ tone: 'info', title: t('settings.undo.done') });
	}

	async validate(): Promise<Result<ValidateResult, UiError>> {
		const outcome = await validateChanges({ ...this.edits });
		this.validation = outcome.ok ? outcome.value : null;
		return outcome;
	}

	async save(): Promise<Result<SaveResult, UiError>> {
		const changes = { ...this.edits };
		this.saving = true;
		this.saveError = null;
		const outcome = await saveChanges(changes, this.revision);
		this.saving = false;
		if (!outcome.ok) {
			this.saveError = outcome.error;
			return outcome;
		}
		const baseline = { ...this.baseline };
		const keyStates = { ...this.keyStates };
		for (const [key, value] of Object.entries(changes)) {
			if ('secret' === entryFor(key)?.type) {
				keyStates[key] = { ...keyStates[key], set: true };
			} else {
				baseline[key] = value;
			}
		}
		this.revision = outcome.value.revision;
		this.baseline = baseline;
		this.keyStates = keyStates;
		this.pending = [...new Set([...this.pending, ...outcome.value.restart_required])];
		this.edits = {};
		this.validation = null;
		return outcome;
	}

	async restartNow(): Promise<void> {
		const outcome = await restart(t('settings.restart.reason'));
		if (!outcome.ok) {
			toasts.push({ tone: 'danger', title: outcome.error.title, body: outcome.error.body });
			return;
		}
		toasts.push({ tone: 'info', title: t('settings.restart.started') });
	}
}
