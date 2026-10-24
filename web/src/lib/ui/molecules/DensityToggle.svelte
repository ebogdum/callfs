<script lang="ts" module>
	export const TOKENS = ['--density-toggle-bg', '--density-toggle-selected-bg'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import type { Density } from '../types';
	import SegmentedControl from './SegmentedControl.svelte';

	const DENSITIES: readonly Density[] = ['compact', 'default', 'comfortable'];

	type Props = Omit<HTMLAttributes<HTMLDivElement>, 'children'> & { id?: string } & {
		value?: Density;
		class?: string;
	};

	let { value = $bindable('default'), class: className, ...rest }: Props = $props();

	const options = $derived(DENSITIES.map((density) => ({ value: density, label: t(`tablecontrols.density.${density}`) })));

	const toDensity = (candidate: string): Density | undefined =>
		DENSITIES.find((density) => density === candidate);

	function choose(next: string): void {
		const density = toDensity(next);
		if (undefined !== density) {
			value = density;
		}
	}
</script>

<SegmentedControl
	bind:value={() => value, choose}
	{options}
	label={t('tablecontrols.density.label')}
	class="density-toggle {className ?? ''}"
	{...rest}
/>
