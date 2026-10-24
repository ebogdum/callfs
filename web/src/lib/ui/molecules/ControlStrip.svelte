<script lang="ts" module>
	export const TOKENS = ['--control-strip-bg', '--control-strip-fg', '--control-strip-border', '--control-strip-mark'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		node: string;
		home: string;
		compact?: boolean;
		onback: () => void;
		class?: string;
	}

	let { node, home, compact = false, onback, class: className, ...rest }: Props = $props();
</script>

<section
	class={['control-strip', className]}
	data-compact={compact ? 'true' : undefined}
	aria-label={t('nodes.strip.label')}
	aria-live="polite"
	{...rest}
>
	<span class="mark" aria-hidden="true"><Icon name="server" size="sm" /></span>
	<span class="text">
		<span class="title">{t('nodes.strip.title', { node })}</span>
		{#if false === compact}
			<span class="body">{t('nodes.strip.body')}</span>
		{/if}
	</span>
	<span class="action">
		<Button variant="secondary" size="sm" iconStart="arrow-left" onclick={onback}>
			{compact ? t('nodes.strip.back_short') : t('nodes.strip.back', { node: home })}
		</Button>
	</span>
</section>

<style>
	.control-strip {
		position: sticky;
		inset-block-start: var(--topbar-h);
		z-index: var(--z-sticky);
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-block-size: var(--touch-target);
		padding-block: var(--space-2);
		padding-inline: var(--gutter);
		border-block-end: var(--hairline) solid var(--control-strip-border, var(--warn-border));
		background: var(--control-strip-bg, var(--warn-soft));
		color: var(--control-strip-fg, var(--warn-text));
	}

	.mark {
		display: grid;
		flex: none;
		place-items: center;
		inline-size: var(--space-6);
		block-size: var(--space-6);
		border-radius: var(--radius-sm);
		background: var(--control-strip-mark, var(--warn));
		color: var(--text-on-accent);
	}

	.text {
		display: flex;
		flex: 1 1 auto;
		flex-wrap: wrap;
		align-items: baseline;
		column-gap: var(--space-3);
		row-gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.title {
		min-inline-size: 0;
		overflow: hidden;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.body {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.action {
		display: inline-flex;
		flex: none;
	}

	.control-strip[data-compact='true'] {
		gap: var(--space-2);
	}
</style>
