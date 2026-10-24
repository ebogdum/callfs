<script lang="ts" module>
	export type HeadingLevel = 1 | 2 | 3 | 4;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Text, { type TextSize } from './Text.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		level: HeadingLevel;
		size?: TextSize;
		children: Snippet;
	}

	let { level, size, class: className, children, ...rest }: Props = $props();

	const defaultSizes: Record<HeadingLevel, TextSize> = {
		1: 'title',
		2: 'title-sm',
		3: 'body',
		4: 'body'
	};
	const headingSize = $derived(size ?? defaultSizes[level]);
</script>

<Text
	as={`h${level}`}
	size={headingSize}
	weight={3 <= level ? 600 : undefined}
	balance
	class={['heading', className]}
	{...rest}
>
	{@render children()}
</Text>
