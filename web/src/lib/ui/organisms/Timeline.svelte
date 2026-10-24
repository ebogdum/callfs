<script lang="ts" module>
	import type { Tone } from '../types';

	export interface TimelineItem {
		at: string;
		actor?: string;
		title: string;
		detail?: string;
		tone: Tone;
		href?: string;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { base } from '$app/paths';
	import { t } from '$lib/copy/t';
	import Link from '../atoms/Link.svelte';
	import RelTime from '../atoms/RelTime.svelte';
	import Text from '../atoms/Text.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLOListElement>, 'class'> {
		items: TimelineItem[];
		class?: string;
	}

	let { items, class: className, ...rest }: Props = $props();

	const sorted = $derived([...items].sort((a, b) => Date.parse(b.at) - Date.parse(a.at)));
</script>

<ol class={['timeline', className]} aria-label={t('u106logs.timeline.label')} {...rest}>
	{#each sorted as item, index (index)}
		<li class="entry">
			<span class="track">
				<StatusGlyph tone={item.tone} />
			</span>
			<div class="content">
				<div class="head">
					{#if undefined === item.href}
						<Text weight={600}>{item.title}</Text>
					{:else}
						<Link href={`${base}${item.href}`}>{item.title}</Link>
					{/if}
					<RelTime at={item.at} />
				</div>
				{#if undefined !== item.actor}
					<Text tone="muted" size="sm">{t('u106logs.timeline.by', { actor: item.actor })}</Text>
				{/if}
				{#if undefined !== item.detail}
					<Text tone="muted" size="sm">{item.detail}</Text>
				{/if}
			</div>
		</li>
	{:else}
		<li class="empty">
			<Text tone="muted">{t('u106logs.timeline.empty')}</Text>
		</li>
	{/each}
</ol>

<style>
	.timeline {
		display: grid;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.entry {
		position: relative;
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: var(--space-3);
		padding-block: var(--space-3);
	}

	.track {
		position: relative;
		display: grid;
		justify-items: center;
		align-content: start;
	}

	.entry:not(:last-child) .track::after {
		content: '';
		position: absolute;
		inset-block-start: var(--space-6);
		inset-block-end: calc(-1 * var(--space-3));
		inset-inline-start: 50%;
		inline-size: var(--hairline);
		background: var(--border);
	}

	.content {
		display: grid;
		gap: var(--space-1);
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2);
	}

	.empty {
		padding-block: var(--space-3);
	}
</style>
