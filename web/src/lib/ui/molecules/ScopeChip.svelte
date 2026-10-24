<script lang="ts" module>
	export const TOKENS = [
		'--scope-chip-radius',
		'--scope-chip-fg',
		'--scope-chip-bg',
		'--scope-chip-border'
	] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '$lib/ui/quarks/Icon.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		server: string;
		member?: string;
		leader?: boolean;
	}

	let { server, member, leader = false, class: className, ...rest }: Props = $props();
</script>

<span class={['scope-chip', className]} data-leader={leader ? 'true' : undefined} {...rest}>
	<Cluster as="span" gap="1" align="center">
		<span>{t('u080molecules.scope.server', { server })}</span>
		{#if member}
			<span aria-hidden="true">{t('u080molecules.scope.separator')}</span>
			<span>{t('u080molecules.scope.member', { member })}</span>
		{/if}
		{#if leader}
			<span aria-hidden="true">{t('u080molecules.scope.separator')}</span>
			<span class="leader">
				<Icon name="leader" size="xs" />
				<span>{t('u080molecules.scope.leader')}</span>
			</span>
		{/if}
	</Cluster>
</span>

<style>
	.scope-chip {
		display: inline-flex;
		max-inline-size: 100%;
		padding-inline: var(--space-2);
		padding-block: var(--space-0-5);
		border: var(--hairline) solid var(--scope-chip-border, var(--border));
		border-radius: var(--scope-chip-radius, var(--radius-full));
		background: var(--scope-chip-bg, var(--surface-sunken));
		color: var(--scope-chip-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		overflow-wrap: anywhere;
	}

	.leader {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--scope-chip-fg, var(--accent-text));
		font-weight: 600;
	}
</style>
