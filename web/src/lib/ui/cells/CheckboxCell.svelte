<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Checkbox from '../atoms/Checkbox.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children' | 'onchange'> {
		checked: boolean;
		label: string;
		onchange?: (checked: boolean) => void;
		class?: string;
	}

	let { checked, label, onchange, class: className, ...rest }: Props = $props();

	const setChecked = (next: boolean) => {
		onchange?.(next);
	};
</script>

<div class={['checkbox-cell', className]} {...rest}>
	<Checkbox bind:checked={() => checked, setChecked} aria-label={label} />
</div>

<style>
	.checkbox-cell {
		display: inline-flex;
		align-items: center;
	}
</style>
