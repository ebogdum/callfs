<script lang="ts" module>
	export const TOKENS = ['--hairline-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		orientation?: 'horizontal' | 'vertical';
		tone?: 'subtle' | 'default';
		decorative?: boolean;
		class?: string;
	}

	let {
		orientation = 'horizontal',
		tone = 'default',
		decorative = true,
		class: className = '',
		...rest
	}: Props = $props();
</script>

<div
	class="hairline {className}"
	data-orientation={orientation}
	data-tone={tone}
	role={decorative ? undefined : 'separator'}
	aria-hidden={decorative ? 'true' : undefined}
	aria-orientation={decorative ? undefined : orientation}
	{...rest}
></div>

<style>
	.hairline {
		flex: none;
		background: var(--hairline-color, var(--border-strong));
	}

	.hairline[data-tone='subtle'] {
		background: var(--hairline-color, var(--border-subtle));
	}

	.hairline[data-orientation='horizontal'] {
		inline-size: 100%;
		block-size: var(--hairline);
	}

	.hairline[data-orientation='vertical'] {
		inline-size: var(--hairline);
		block-size: 100%;
		align-self: stretch;
	}
</style>
