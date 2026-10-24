<script lang="ts" module>
	const SIZE_UNITS = ['B', 'kB', 'MB', 'GB'] as const;
	type SizeUnitCode = (typeof SIZE_UNITS)[number];

	const DISPLAY_SCALE = 10;

	const UNIT_BYTES: Record<SizeUnitCode, number> = { B: 1, kB: 1000, MB: 1000 * 1000, GB: 1000 * 1000 * 1000 };

	function readSize(bytes: number, units: SizeUnitCode[]): { amount: number; unit: SizeUnitCode } {
		const smallest = units.reduce((a, b) => (UNIT_BYTES[a] <= UNIT_BYTES[b] ? a : b));
		const largest =
			[...units]
				.sort((a, b) => UNIT_BYTES[b] - UNIT_BYTES[a])
				.find((code) => bytes >= UNIT_BYTES[code]) ?? smallest;
		return { amount: Math.round((bytes / UNIT_BYTES[largest]) * DISPLAY_SCALE) / DISPLAY_SCALE, unit: largest };
	}

	function sameShown(bytes: number, amount: number | null, unit: SizeUnitCode, units: SizeUnitCode[]): boolean {
		const shown = readSize(bytes, units);
		return shown.amount === amount && shown.unit === unit;
	}

	function toBytes(amount: number | null, unit: SizeUnitCode): number {
		return null === amount ? 0 : Math.round(amount * UNIT_BYTES[unit]);
	}

	function resolveUnit(text: string, fallback: SizeUnitCode): SizeUnitCode {
		const match = SIZE_UNITS.find((code) => code === text);
		return undefined === match ? fallback : match;
	}
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Label from '../atoms/Label.svelte';
	import NumberInput from '../atoms/NumberInput.svelte';
	import Select from '../atoms/Select.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		value?: number;
		label: string;
		units?: SizeUnitCode[];
	}

	let { value = $bindable(0), label, units = ['B', 'kB', 'MB', 'GB'], class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const amountId = `${uid}-amount`;

	const initial = readSize(value, units);
	let amount = $state<number | null>(initial.amount);
	let unit = $state<SizeUnitCode>(initial.unit);

	const unitOptions = $derived(units.map((code) => ({ value: code, label: code })));

	$effect(() => {
		const external = value;
		untrack(() => {
			if (external !== toBytes(amount, unit) && !sameShown(external, amount, unit, units)) {
				const next = readSize(external, units);
				amount = next.amount;
				unit = next.unit;
			}
		});
	});

	$effect(() => {
		const next = toBytes(amount, unit);
		untrack(() => {
			if (next !== value && !sameShown(value, amount, unit, units)) {
				value = next;
			}
		});
	});
</script>

<Stack gap="1-5" class={className} {...rest}>
	<Label for={amountId}>{label}</Label>
	<Cluster gap="2">
		<NumberInput id={amountId} class="size-amount" bind:value={amount} min={0} step={1} />
		<Select
			class="size-unit"
			value={unit}
			options={unitOptions}
			aria-label={`${label}, ${t('inputs.unit')}`}
			onchange={(event) => {
				unit = resolveUnit(event.currentTarget.value, unit);
			}}
		/>
	</Cluster>
</Stack>

<style>
	:global(.size-amount) {
		flex: 1 1 auto;
		min-inline-size: 0;
	}

	:global(.size-unit) {
		flex: 0 0 auto;
	}
</style>
