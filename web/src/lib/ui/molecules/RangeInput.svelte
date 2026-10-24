<script lang="ts" module>
	type RangePreset = 'first' | 'last' | 'custom';
	type RangeFault = 'outside' | 'order';

	const MEGABYTE = 1000 * 1000;

	const rangeFor = (preset: RangePreset, size: number): { from: number; to: number } | null => {
		if ('first' === preset) {
			return { from: 0, to: Math.max(0, Math.min(MEGABYTE, size) - 1) };
		}
		if ('last' === preset) {
			return { from: Math.max(0, size - MEGABYTE), to: Math.max(0, size - 1) };
		}
		return null;
	};

	const faultFor = (from: number, to: number | null, size: number): RangeFault | null => {
		if (0 > from || from >= size) {
			return 'outside';
		}
		if (null === to) {
			return null;
		}
		if (to >= size) {
			return 'outside';
		}
		return from > to ? 'order' : null;
	};
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLFieldsetAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Label from '../atoms/Label.svelte';
	import NumberInput from '../atoms/NumberInput.svelte';
	import Select from '../atoms/Select.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLFieldsetAttributes, 'class'> {
		size: number;
		from: number;
		to: number | null;
		label: string;
		class?: string;
	}

	let { size, from = $bindable(), to = $bindable(), label, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const presetId = `${uid}-preset`;
	const fromId = `${uid}-from`;
	const toId = `${uid}-to`;
	const helpId = `${uid}-help`;
	const errorId = `${uid}-error`;

	const presetOptions = [
		{ value: 'first', label: t('u082molecules.range.first_mb') },
		{ value: 'last', label: t('u082molecules.range.last_mb') },
		{ value: 'custom', label: t('u082molecules.range.custom') },
	];

	let preset = $state<RangePreset>('custom');
	let fromDraft = $state<number | null>(from);
	let toDraft = $state<number | null>(to);

	const fault = $derived(faultFor(from, to, size));
	const errorText = $derived.by(() => {
		if ('outside' === fault) {
			return t('u082molecules.range.outside', { size: formatBytes(size).text });
		}
		if ('order' === fault) {
			return t('u082molecules.range.order');
		}
		return null;
	});
	const describedBy = $derived(null === errorText ? helpId : errorId);

	function presetFrom(text: string): RangePreset {
		if ('first' === text) {
			return 'first';
		}
		if ('last' === text) {
			return 'last';
		}
		return 'custom';
	}

	function applyPreset(next: RangePreset): void {
		preset = next;
		const range = rangeFor(next, size);
		if (null === range) {
			return;
		}
		fromDraft = range.from;
		toDraft = range.to;
	}

	function markCustom(): void {
		preset = 'custom';
	}

	$effect(() => {
		const next = fromDraft ?? 0;
		untrack(() => {
			if (next !== from) {
				from = next;
			}
		});
	});

	$effect(() => {
		const external = from;
		untrack(() => {
			if (external !== (fromDraft ?? 0)) {
				fromDraft = external;
			}
		});
	});

	$effect(() => {
		const next = toDraft;
		untrack(() => {
			if (next !== to) {
				to = next;
			}
		});
	});

	$effect(() => {
		const external = to;
		untrack(() => {
			if (external !== toDraft) {
				toDraft = external;
			}
		});
	});
</script>

<fieldset class={['range-input', className]} data-state={null === errorText ? 'idle' : 'error'} {...rest}>
	<legend>{label}</legend>
	<Stack gap="2">
		<Stack gap="1-5">
			<Label for={presetId}>{t('u082molecules.range.preset')}</Label>
			<Select
				id={presetId}
				value={preset}
				options={presetOptions}
				onchange={(event) => applyPreset(presetFrom(event.currentTarget.value))}
			/>
		</Stack>
		<Cluster gap="2" align="end" wrap>
			<Stack gap="1-5" class="range-field">
				<Label for={fromId}>{t('u082molecules.range.from')}</Label>
				<NumberInput
					id={fromId}
					bind:value={fromDraft}
					min={0}
					step={1}
					unit={t('u082molecules.range.unit')}
					aria-invalid={null === errorText ? undefined : 'true'}
					aria-describedby={describedBy}
					oninput={markCustom}
				/>
			</Stack>
			<Stack gap="1-5" class="range-field">
				<Label for={toId}>{t('u082molecules.range.to')}</Label>
				<NumberInput
					id={toId}
					bind:value={toDraft}
					min={0}
					step={1}
					unit={t('u082molecules.range.unit')}
					aria-invalid={null === errorText ? undefined : 'true'}
					aria-describedby={describedBy}
					oninput={markCustom}
				/>
			</Stack>
		</Cluster>
		{#if null === errorText}
			<HelpText id={helpId}>{t('u082molecules.range.help')}</HelpText>
		{:else}
			<FieldError id={errorId} message={errorText} />
		{/if}
	</Stack>
</fieldset>

<style>
	.range-input {
		min-inline-size: 0;
		margin: 0;
		padding: 0;
		border: 0;
	}

	.range-input > legend {
		padding: 0;
	}

	:global(.range-field) {
		flex: 1 1 var(--range-input-field-basis, auto);
		min-inline-size: 0;
	}
</style>
