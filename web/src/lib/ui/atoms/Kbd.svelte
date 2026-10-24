<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		keys: string[];
	}

	let { keys, class: className, ...rest }: Props = $props();

	const isMac = 'undefined' !== typeof navigator && /Mac|iPhone|iPad|iPod/.test(navigator.userAgent);

	const modifierLabels = new Map<string, { mac: string; other: string }>([
		['mod', { mac: '⌘', other: t('text.key_ctrl') }],
		['shift', { mac: '⇧', other: t('text.key_shift') }],
		['alt', { mac: '⌥', other: t('text.key_alt') }],
		['enter', { mac: '↵', other: '↵' }]
	]);

	function keyLabel(key: string): string {
		const label = modifierLabels.get(key);
		if (undefined === label) {
			return key;
		}
		return isMac ? label.mac : label.other;
	}
</script>

<span class={['kbd-group', className]} {...rest}>
	{#each keys as key, index (index)}
		<kbd>{keyLabel(key)}</kbd>
	{/each}
</span>

<style>
	.kbd-group {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}

	kbd {
		min-inline-size: var(--space-4);
		padding: var(--space-0-5) var(--space-1);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-xs);
		background: var(--bg-subtle);
		color: var(--text-muted);
		font-family: var(--font-mono);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		text-align: center;
		white-space: nowrap;
	}
</style>
