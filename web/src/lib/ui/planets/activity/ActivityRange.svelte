<script lang="ts" module>
	import { getContext } from 'svelte';
	import {
		presetValue,
		TimeRangeStore,
		type TimeRangePreset,
		type TimeRangeValue
	} from '$lib/stores/time-range.svelte';

	const RANGE_CONTEXT = 'callfs.timeRange';
	const PRESETS: readonly TimeRangePreset[] = ['15m', '1h', '6h', '24h', '7d'];
	const CUSTOM = 'custom';
	const DEFAULT_PRESET: TimeRangePreset = '24h';

	export function activityRange(): TimeRangeStore {
		return (
			getContext<TimeRangeStore | undefined>(RANGE_CONTEXT) ??
			new TimeRangeStore(presetValue(DEFAULT_PRESET, Date.now(), true))
		);
	}

	export function rangeKey(value: TimeRangeValue): string {
		return undefined === value.preset ? `${value.from}-${value.to}` : value.preset;
	}

	export function rangeSince(value: TimeRangeValue): string {
		const from = undefined === value.preset ? value.from : presetValue(value.preset, Date.now(), value.live).from;
		return new Date(from).toISOString();
	}

	export function rangeUntil(value: TimeRangeValue): string | undefined {
		return undefined === value.preset ? new Date(value.to).toISOString() : undefined;
	}

	function presetOf(raw: string | undefined): TimeRangePreset | undefined {
		return PRESETS.find((preset) => preset === raw);
	}

	function toRangeValue(next: { from: number; to: number; preset?: string; live: boolean }): TimeRangeValue {
		const preset = presetOf(next.preset);
		return undefined === preset
			? { from: next.from, to: next.to, live: false }
			: presetValue(preset, Date.now(), next.live);
	}
</script>

<script lang="ts">
	import { t } from '$lib/copy/t';
	import Button from '../../atoms/Button.svelte';
	import ToggleButton from '../../atoms/ToggleButton.svelte';
	import Popover from '../../molecules/Popover.svelte';
	import SegmentedControl from '../../molecules/SegmentedControl.svelte';
	import TimeRangePicker from '../../molecules/TimeRangePicker.svelte';

	interface Props {
		store: TimeRangeStore;
		showLive?: boolean;
	}

	let { store, showLive = true }: Props = $props();

	const options = PRESETS.map((preset) => ({ value: preset, label: t(`activity.range.${preset}`) }));
	const selected = $derived(store.value.preset ?? CUSTOM);
	const isCustom = $derived(undefined === store.value.preset);
	const canLive = $derived(false === isCustom);

	function choose(next: string): void {
		const preset = presetOf(next);
		if (undefined !== preset) {
			store.value = presetValue(preset, Date.now(), store.value.live);
		}
	}

	function setLive(live: boolean): void {
		const preset = store.value.preset;
		if (undefined !== preset) {
			store.value = presetValue(preset, Date.now(), live);
		}
	}
</script>

<div class="activity-range">
	<SegmentedControl label={t('activity.range.label')} {options} bind:value={() => selected, choose} />
	<Popover label={t('activity.range.customLabel')} wide>
		{#snippet trigger(props)}
			<Button {...props} variant={isCustom ? 'primary' : 'secondary'} size="sm" iconStart="calendar">
				{isCustom ? t('activity.range.customOn') : t('activity.range.custom')}
			</Button>
		{/snippet}
		{#snippet content()}
			<TimeRangePicker bind:value={() => store.value, (next) => (store.value = toRangeValue(next))} />
		{/snippet}
	</Popover>
	{#if showLive}
		<ToggleButton
			icon={store.value.live ? 'pause' : 'play'}
			label={t('activity.range.live')}
			title={store.value.live ? t('activity.range.liveOn') : t('activity.range.liveOff')}
			disabled={!canLive}
			bind:pressed={() => store.value.live, setLive}
		/>
	{/if}
</div>

<style>
	.activity-range {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}
</style>
