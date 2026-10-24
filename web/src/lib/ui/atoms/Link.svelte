<script lang="ts" module>
	export type LinkTone = 'default' | 'muted';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAnchorAttributes {
		href: string;
		external?: boolean;
		tone?: LinkTone;
		children: Snippet;
	}

	let { href, external = false, tone = 'default', class: className, children, ...rest }: Props = $props();
</script>

<a
	class={['link', className]}
	{href}
	data-tone={tone}
	target={external ? '_blank' : undefined}
	rel={external ? 'noopener noreferrer' : undefined}
	{...rest}
>
	{@render children()}{#if external}
		<Icon name="external" size="xs" />
		<VisuallyHidden>{t('common.opens_new_tab')}</VisuallyHidden>
	{/if}
</a>

<style>
	.link {
		display: inline-flex;
		align-items: baseline;
		gap: var(--space-1);
		color: var(--text-link);
		text-decoration: none;
		border-radius: var(--radius-xs);
		outline: var(--focus-width) solid transparent;
	}

	.link[data-tone='muted'] {
		color: var(--text-muted);
	}

	p > .link {
		text-decoration: underline;
	}

	.link:focus-visible {
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.link:hover {
			text-decoration: underline;
		}
	}

	.link:focus-visible {
		text-decoration: underline;
	}
</style>
