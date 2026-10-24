<script lang="ts">
	import type { LucideProps } from './icons';
	import type { IconName } from '../types';
	import { icons, iconPixels, type IconSize } from './icons';

	interface Props extends Omit<LucideProps, 'role' | 'aria-label' | 'size' | 'strokeWidth' | 'children'> {
		name: IconName;
		size?: IconSize;
		label?: string;
		strokeWidth?: number;
	}

	let { name, size = 'md', label, strokeWidth = 1.5, class: className, ...rest }: Props = $props();

	const Glyph = $derived(icons[name]);
	const named = $derived(undefined !== label && '' !== label);
</script>

<Glyph
	size={iconPixels[size]}
	{strokeWidth}
	class={className ? `icon ${className}` : 'icon'}
	role={named ? 'img' : undefined}
	aria-label={named ? label : undefined}
	aria-hidden={named ? undefined : 'true'}
	{...rest}
/>
