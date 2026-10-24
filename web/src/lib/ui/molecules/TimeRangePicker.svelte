<script lang="ts" module>
	export type TimePreset = '15m' | '1h' | '6h' | '24h' | '7d';

	export interface TimeRange {
		from: number;
		to: number;
		preset?: string;
		live: boolean;
	}

	const PRESET_MS: Record<TimePreset, number> = {
		'15m': 900_000,
		'1h': 3_600_000,
		'6h': 21_600_000,
		'24h': 86_400_000,
		'7d': 604_800_000
	};

	const PRESET_KEYS: Record<TimePreset, string> = {
		'15m': 'u076molecules.preset_15m',
		'1h': 'u076molecules.preset_1h',
		'6h': 'u076molecules.preset_6h',
		'24h': 'u076molecules.preset_24h',
		'7d': 'u076molecules.preset_7d'
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Checkbox from '../atoms/Checkbox.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import Switch from '../atoms/Switch.svelte';
	import DateTimeInput from './DateTimeInput.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		value: TimeRange;
		presets?: TimePreset[];
		compare?: boolean;
	}

	let {
		value = $bindable({ from: 0, to: 0, live: false }),
		presets = ['15m', '1h', '6h', '24h', '7d'],
		compare = $bindable(false),
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const errorId = `${uid}-error`;
	const orderInvalid = $derived(value.to <= value.from);

	function applyPreset(preset: TimePreset) {
		const to = Date.now();
		value = { from: to - PRESET_MS[preset], to, preset, live: value.live };
	}

	function setFrom(from: number) {
		value = { from, to: value.to, live: value.live };
	}

	function setTo(to: number) {
		value = { from: value.from, to, live: value.live };
	}

	function setLive(live: boolean) {
		value = { ...value, live };
	}
</script>

<div class={['time-range', className]} role="group" aria-label={t('u076molecules.timerange_label')} {...rest}>
	<div class="segmented presets" role="group" aria-label={t('u076molecules.timerange_presets')}>
		{#each presets as preset (preset)}
			<button
				type="button"
				class="segment"
				data-state={value.preset === preset ? 'on' : 'off'}
				aria-pressed={value.preset === preset}
				onclick={() => applyPreset(preset)}>{t(PRESET_KEYS[preset])}</button
			>
		{/each}
	</div>
	<div class="range">
		<div class="field">
			<DateTimeInput bind:value={() => value.from, setFrom} label={t('u076molecules.timerange_from')} />
		</div>
		<div class="field">
			<DateTimeInput bind:value={() => value.to, setTo} label={t('u076molecules.timerange_to')} />
		</div>
	</div>
	<div aria-live="polite">
		{#if orderInvalid}
			<FieldError id={errorId} message={t('u076molecules.timerange_error_order')} />
		{/if}
	</div>
	<div class="options">
		<Switch
			bind:checked={() => value.live, setLive}
			label={t('u076molecules.timerange_live')}
			description={t('u076molecules.timerange_live_help')}
		/>
		<Checkbox bind:checked={compare} label={t('u076molecules.timerange_compare')} />
	</div>
</div>

<style>
	.time-range {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.presets {
		align-self: flex-start;
	}

	.range {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		max-inline-size: var(--form-max);
	}

	.field {
		flex: 1 1 calc(var(--space-4) * 14);
		min-inline-size: 0;
	}

	.options {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		max-inline-size: var(--form-max);
	}
</style>
