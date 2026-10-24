<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		copy?: boolean;
		children: Snippet;
	}

	type CopyState = 'idle' | 'copied' | 'failed';

	let { copy = false, class: className, children, ...rest }: Props = $props();

	let element: HTMLElement | undefined = $state();
	let copyState: CopyState = $state('idle');

	const copyLabel = $derived.by(() => {
		if ('copied' === copyState) {
			return t('common.copied');
		}
		if ('failed' === copyState) {
			return t('text.copy_failed');
		}
		return t('common.copy');
	});

	function copyValue(): void {
		const text = element?.textContent ?? '';
		if (!navigator.clipboard) {
			copyState = 'failed';
			return;
		}
		navigator.clipboard.writeText(text).then(
			() => {
				copyState = 'copied';
			},
			() => {
				copyState = 'failed';
			}
		);
	}

	$effect(() => {
		if ('idle' === copyState) {
			return;
		}
		const timer = setTimeout(() => {
			copyState = 'idle';
		}, 1600);
		return () => {
			clearTimeout(timer);
		};
	});
</script>

<span class={['code', className]} data-copy-state={copyState} {...rest}>
	<code bind:this={element}>{@render children()}</code>
	{#if copy}
		<button type="button" class="copy" onclick={copyValue}>
			<Icon name={'copied' === copyState ? 'check' : 'copy'} size="xs" />
			<span>{copyLabel}</span>
		</button>
	{/if}
</span>

<style>
	.code {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		padding: var(--space-0-5) var(--space-1);
		border-radius: var(--radius-xs);
		background: var(--code-bg);
		color: var(--text);
	}

	code {
		min-inline-size: 0;
		overflow-wrap: anywhere;
		font-family: var(--font-mono);
		font-size: inherit;
	}

	.copy {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-block-size: var(--target-min);
		min-inline-size: var(--target-min);
		padding-inline: var(--space-1);
		border: 0;
		border-radius: var(--radius-xs);
		background: transparent;
		color: var(--text-muted);
		font: inherit;
		cursor: pointer;
	}

	.copy:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.copy:hover {
			color: var(--text);
		}
	}

	.copy:active {
		color: var(--text);
	}
</style>
