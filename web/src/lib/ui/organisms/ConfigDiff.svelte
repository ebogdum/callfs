<script lang="ts" module>
	export const TOKENS = ['--config-diff-gap', '--config-diff-removed-fg', '--config-diff-added-fg'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Badge from '../atoms/Badge.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		before: Record<string, unknown>;
		after: Record<string, unknown>;
		secretKeys: string[];
		restartKeys?: string[];
		class?: string;
	}

	let { before, after, secretKeys, restartKeys = [], class: className, ...rest }: Props = $props();

	const changedKeys = $derived(
		[...new Set([...Object.keys(before), ...Object.keys(after)])]
			.filter((key) => JSON.stringify(before[key]) !== JSON.stringify(after[key]))
			.sort(),
	);

	function describeValue(value: unknown): string {
		if (undefined === value || null === value) {
			return t('settingsui.diff.unset');
		}
		if ('string' === typeof value) {
			return value;
		}
		if ('number' === typeof value || 'boolean' === typeof value) {
			return String(value);
		}
		return JSON.stringify(value);
	}
</script>

<div class={['config-diff', className]} {...rest}>
	{#if 0 === changedKeys.length}
		<Text tone="muted">{t('settingsui.diff.none')}</Text>
	{:else}
		<ul class="diff-list">
			{#each changedKeys as key (key)}
				<li class="diff-row">
					<Stack gap="1">
						<Cluster gap="2" align="center">
							<Text mono size="sm">{key}</Text>
							<Badge tone={restartKeys.includes(key) ? 'warn' : 'ok'}>
								{restartKeys.includes(key) ? t('settingsui.restart.needed') : t('settingsui.restart.applies')}
							</Badge>
						</Cluster>
						{#if secretKeys.includes(key)}
							<Text size="sm">{t('settingsui.diff.replaced')}</Text>
						{:else}
							<Text size="sm">{t('settingsui.diff.changed', { from: describeValue(before[key]), to: describeValue(after[key]) })}</Text>
							<Stack gap="1">
								<Text mono size="sm"><span class="removed"><span aria-hidden="true">−</span> {describeValue(before[key])}</span></Text>
								<Text mono size="sm"><span class="added"><span aria-hidden="true">+</span> {describeValue(after[key])}</span></Text>
							</Stack>
						{/if}
					</Stack>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.config-diff {
		min-inline-size: 0;
	}

	.diff-list {
		display: grid;
		gap: var(--config-diff-gap, var(--space-4));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.diff-row {
		min-inline-size: 0;
	}

	.removed {
		color: var(--config-diff-removed-fg, var(--danger-text));
	}

	.added {
		color: var(--config-diff-added-fg, var(--ok-text));
	}
</style>
