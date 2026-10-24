<script lang="ts" module>
	export interface SetupItem {
		id: string;
		label: string;
		done: boolean;
		href: string;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../../atoms/Button.svelte';
	import ProgressBar from '../../atoms/ProgressBar.svelte';
	import Text from '../../atoms/Text.svelte';
	import Icon from '../../quarks/Icon.svelte';
	import VisuallyHidden from '../../quarks/VisuallyHidden.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		items: SetupItem[];
		ondismiss: () => void;
		class?: string;
	}

	let { items, ondismiss, class: className, ...rest }: Props = $props();

	const doneCount = $derived(items.filter((item) => item.done).length);
</script>

<div class={['setup-checklist', className]} {...rest}>
	<div class="summary">
		<div class="summary-row">
			<Text tone="muted" size="sm" tabular>{t('overview.setup.progress', { done: doneCount, total: items.length })}</Text>
			<Button variant="ghost" size="sm" onclick={ondismiss}>{t('overview.setup.dismiss')}</Button>
		</div>
		<ProgressBar value={doneCount} max={items.length} label={t('overview.setup.title')} tone="ok" />
	</div>
	<ul class="items">
		{#each items as item (item.id)}
			<li>
				<a class="item" href={item.href} data-done={item.done}>
					<span class="mark" aria-hidden="true">
						{#if item.done}
							<Icon name="check" size="sm" />
						{/if}
					</span>
					<span class="label">{item.label}</span>
					<VisuallyHidden>{item.done ? t('overview.setup.done') : t('overview.setup.todo')}</VisuallyHidden>
					<Icon name="chevron-right" size="sm" class="go" />
				</a>
			</li>
		{/each}
	</ul>
</div>

<style>
	.setup-checklist {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.summary {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.items {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, calc(var(--space-4) * 18)), 1fr));
		gap: var(--space-0-5) var(--space-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.item {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-3);
		min-block-size: var(--control-h);
		padding-block: var(--space-1-5);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		color: var(--text);
		text-decoration: none;
	}

	@media (hover: hover) {
		.item:hover {
			background: var(--surface-hover);
		}
	}

	.item:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.mark {
		display: grid;
		place-items: center;
		inline-size: var(--space-5);
		block-size: var(--space-5);
		border: var(--hairline) solid var(--border-strong);
		border-radius: var(--radius-full);
		color: var(--text-on-accent);
	}

	.item[data-done='true'] .mark {
		border-color: var(--ok);
		background: var(--ok);
	}

	.label {
		min-inline-size: 0;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.item[data-done='true'] .label {
		color: var(--text-muted);
	}

	.item :global(.go) {
		color: var(--text-faint);
	}

	.summary-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
	}
</style>
