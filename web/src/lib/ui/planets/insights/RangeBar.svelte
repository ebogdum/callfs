<script lang="ts" module>
	import { getContext } from 'svelte';
	import { presetValue, TimeRangeStore, type TimeRangePreset, type TimeRangeValue } from '$lib/stores/time-range.svelte';

	const RANGE_CONTEXT = 'callfs.timeRange';
	const PRESETS: readonly TimeRangePreset[] = ['15m', '1h', '6h', '24h', '7d'];

	export function insightsRange(): TimeRangeStore {
		return getContext<TimeRangeStore | undefined>(RANGE_CONTEXT) ?? new TimeRangeStore(presetValue('24h', Date.now(), false));
	}

	function presetOf(raw: string | undefined): TimeRangePreset | undefined {
		return PRESETS.find((preset) => preset === raw);
	}

	export function rangeValueOf(next: { from: number; to: number; preset?: string; live: boolean }): TimeRangeValue {
		const preset = presetOf(next.preset);
		return undefined === preset ? { from: next.from, to: next.to, live: false } : presetValue(preset, Date.now(), next.live);
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
	}

	let { store }: Props = $props();

	const options = PRESETS.map((preset) => ({ value: preset, label: t(`insights.range.${preset}`) }));
	const isCustom = $derived(undefined === store.value.preset);
	const selected = $derived(store.value.preset ?? '');
	const canLive = $derived(undefined !== store.value.preset);

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

<div class="range-bar">
	<SegmentedControl label={t('insights.range.label')} {options} bind:value={() => selected, choose} />
	<Popover label={t('insights.range.custom_label')} wide>
		{#snippet trigger(props)}
			<Button {...props} variant={isCustom ? 'primary' : 'ghost'} size="sm" iconStart="calendar">
				{isCustom ? t('insights.range.custom_on') : t('insights.range.custom')}
			</Button>
		{/snippet}
		{#snippet content()}
			<TimeRangePicker bind:value={() => store.value, (next) => (store.value = rangeValueOf(next))} />
		{/snippet}
	</Popover>
	<ToggleButton
		icon={store.value.live ? 'pause' : 'play'}
		label={t('insights.range.live')}
		title={store.value.live ? t('insights.range.live_on') : t('insights.range.live_off')}
		disabled={!canLive}
		bind:pressed={() => store.value.live, setLive}
	/>
</div>

<style>
	.range-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}
</style>
