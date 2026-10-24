<script lang="ts" module>
	import { formatDuration, parseGoDuration } from '$lib/format/duration';

	const DURATION_UNITS = ['ms', 's', 'm', 'h'] as const;
	type DurationUnitCode = (typeof DURATION_UNITS)[number];

	const UNIT_MS: Record<DurationUnitCode, number> = { ms: 1, s: 1000, m: 60 * 1000, h: 60 * 60 * 1000 };
	const UNIT_COPY: Record<DurationUnitCode, string> = {
		ms: 'inputs.duration.unit.ms',
		s: 'inputs.duration.unit.s',
		m: 'inputs.duration.unit.m',
		h: 'inputs.duration.unit.h'
	};

	function durationMs(text: string): number | null {
		const parsed = parseGoDuration(text);
		return parsed.ok ? parsed.value : null;
	}

	function readDuration(text: string, units: DurationUnitCode[]): { amount: number | null; unit: DurationUnitCode } {
		const smallest = units.reduce((a, b) => (UNIT_MS[a] <= UNIT_MS[b] ? a : b));
		const parsed = parseGoDuration(text);
		if (!parsed.ok) {
			return { amount: null, unit: smallest };
		}
		const exact = [...units]
			.sort((a, b) => UNIT_MS[b] - UNIT_MS[a])
			.find((code) => 0 === parsed.value % UNIT_MS[code]);
		const unit = exact ?? smallest;
		return { amount: parsed.value / UNIT_MS[unit], unit };
	}

	function toText(amount: number | null, unit: DurationUnitCode): string {
		return null === amount ? '' : `${amount}${unit}`;
	}

	function resolveUnit(text: string, fallback: DurationUnitCode): DurationUnitCode {
		const match = DURATION_UNITS.find((code) => code === text);
		return undefined === match ? fallback : match;
	}
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Label from '../atoms/Label.svelte';
	import NumberInput from '../atoms/NumberInput.svelte';
	import Select from '../atoms/Select.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		value?: string;
		label: string;
		min?: string;
		max?: string;
		units?: DurationUnitCode[];
	}

	let {
		value = $bindable(''),
		label,
		min,
		max,
		units = ['ms', 's', 'm', 'h'],
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const amountId = `${uid}-amount`;
	const helpId = `${uid}-help`;
	const errorId = `${uid}-error`;

	const initial = readDuration(value, units);
	let amount = $state<number | null>(initial.amount);
	let unit = $state<DurationUnitCode>(initial.unit);

	const unitOptions = $derived(units.map((code) => ({ value: code, label: t(UNIT_COPY[code]) })));
	const currentMs = $derived(durationMs(toText(amount, unit)));
	const minMs = $derived(undefined === min ? null : durationMs(min));
	const maxMs = $derived(undefined === max ? null : durationMs(max));

	const problem = $derived.by((): string | null => {
		if (null !== currentMs && null !== minMs && currentMs < minMs) {
			return t('inputs.duration.below_min', { value: formatDuration(minMs, 'long') });
		}
		if (null !== currentMs && null !== maxMs && currentMs > maxMs) {
			return t('inputs.duration.above_max', { value: formatDuration(maxMs, 'long') });
		}
		return null;
	});

	const describedBy = $derived(
		[null !== currentMs ? helpId : null, null !== problem ? errorId : null].filter(
			(id): id is string => null !== id
		).join(' ') || undefined
	);

	$effect(() => {
		const external = value;
		untrack(() => {
			if (durationMs(external) !== durationMs(toText(amount, unit))) {
				const next = readDuration(external, units);
				amount = next.amount;
				unit = next.unit;
			}
		});
	});

	$effect(() => {
		const next = toText(amount, unit);
		untrack(() => {
			if (durationMs(next) !== durationMs(value)) {
				value = next;
			}
		});
	});
</script>

<Stack gap="1-5" class={className} {...rest}>
	<Label for={amountId}>{label}</Label>
	<Stack gap="1">
		<Cluster gap="2">
			<NumberInput
				id={amountId}
				class="duration-amount"
				bind:value={amount}
				step={1}
				aria-invalid={null === problem ? undefined : 'true'}
				aria-describedby={describedBy}
			/>
			<Select
				class="duration-unit"
				value={unit}
				options={unitOptions}
				aria-label={`${label}, ${t('inputs.unit')}`}
				onchange={(event) => {
					unit = resolveUnit(event.currentTarget.value, unit);
				}}
			/>
		</Cluster>
		{#if null !== problem}
			<FieldError id={errorId} message={problem} />
		{/if}
		{#if null !== currentMs}
			<HelpText id={helpId}>{formatDuration(currentMs, 'long')}</HelpText>
		{/if}
	</Stack>
</Stack>

<style>
	:global(.duration-amount) {
		flex: 1 1 auto;
		min-inline-size: 0;
	}

	:global(.duration-unit) {
		flex: 0 0 auto;
	}
</style>
