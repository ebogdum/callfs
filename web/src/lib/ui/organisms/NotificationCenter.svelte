<script lang="ts" module>
	import type { Tone } from '../types';

	export interface NotificationEntry {
		id: number;
		at: number;
		tone: Tone;
		category: 'error' | 'job' | 'cluster' | 'event';
		title: string;
		body: string;
		requestId: string | null;
		read: boolean;
	}

	export const TOKENS = [
		'--notification-center-gap',
		'--notification-row-pad',
		'--notification-row-bg',
		'--notification-row-unread'
	] as const;

	const FILTER_CATEGORY: Record<string, NotificationEntry['category']> = {
		errors: 'error',
		jobs: 'job',
		cluster: 'cluster'
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import RelTime from '../atoms/RelTime.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import SegmentedControl from '../molecules/SegmentedControl.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		items: NotificationEntry[];
		onread: (id: number) => void;
		onclear: (id: number) => void;
		onopen: (entry: NotificationEntry) => void;
	}

	let { items, onread, onclear, onopen, class: className, ...rest }: Props = $props();

	let filter = $state('all');

	const filterOptions = [
		{ value: 'all', label: t('notifications.filter.all') },
		{ value: 'errors', label: t('notifications.filter.errors') },
		{ value: 'jobs', label: t('notifications.filter.jobs') },
		{ value: 'cluster', label: t('notifications.filter.cluster') }
	];

	const visible = $derived(
		items.filter((entry) => 'all' === filter || FILTER_CATEGORY[filter] === entry.category)
	);
	const unread = $derived(items.filter((entry) => false === entry.read));

	const markAllRead = () => {
		for (const entry of unread) {
			onread(entry.id);
		}
	};
</script>

<section {...rest} class={['notification-center', className]} aria-label={t('notifications.center.title')}>
	<Stack gap="4">
		<Cluster gap="2" justify="space-between">
			<SegmentedControl bind:value={filter} options={filterOptions} label={t('notifications.filter.label')} />
			<Button variant="ghost" size="sm" onclick={markAllRead}>{t('notifications.center.mark_all_read')}</Button>
		</Cluster>
		<p class="summary">{t('notifications.center.summary', { count: unread.length })}</p>
		{#if 0 === items.length}
			<EmptyState
				variant="cleared"
				icon="bell"
				title={t('notifications.center.empty_title')}
				body={t('notifications.center.empty_body')}
			/>
		{:else if 0 === visible.length}
			<EmptyState
				variant="no-results"
				icon="filter"
				title={t('notifications.center.filtered_empty_title')}
				body={t('notifications.center.filtered_empty_body')}
			/>
		{:else}
			<ol class="entries">
				{#each visible as entry (entry.id)}
					<li class="entry" data-tone={entry.tone} data-read={entry.read}>
						<Stack gap="2">
							<Cluster gap="2" align="start">
								<StatusGlyph tone={entry.tone} />
								<Stack gap="1">
									<Cluster gap="2" align="center">
										<p class="title">{entry.title}</p>
										{#if false === entry.read}
											<span class="new">{t('notifications.entry.new')}</span>
										{/if}
									</Cluster>
									<p class="body">{entry.body}</p>
									<Cluster gap="2" align="center">
										<RelTime at={entry.at} />
										{#if null !== entry.requestId}
											<span class="request-id">{t('notifications.entry.request_id')} {entry.requestId}</span>
											<CopyButton
												value={entry.requestId}
												label={t('notifications.entry.copy_request_id')}
												variant="button"
											/>
										{/if}
									</Cluster>
								</Stack>
							</Cluster>
							<Cluster gap="2">
								{#if false === entry.read}
									<Button variant="ghost" size="sm" onclick={() => onread(entry.id)}>
										{t('notifications.entry.mark_read')}
									</Button>
								{/if}
								<Button variant="ghost" size="sm" onclick={() => onopen(entry)}>
									{t('notifications.entry.open')}
								</Button>
								<Button variant="ghost" size="sm" iconStart="x" onclick={() => onclear(entry.id)}>
									{t('notifications.entry.clear')}
								</Button>
							</Cluster>
						</Stack>
					</li>
				{/each}
			</ol>
		{/if}
	</Stack>
</section>

<style>
	.notification-center {
		display: grid;
		min-inline-size: 0;
	}

	.summary {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
	}

	.entries {
		display: grid;
		gap: var(--notification-center-gap, var(--space-3));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.entry {
		padding: var(--notification-row-pad, var(--space-3));
		background: var(--notification-row-bg, var(--surface-raised));
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
	}

	.entry[data-read='false'] {
		background: var(--notification-row-unread, var(--surface-selected));
	}

	.title {
		margin: 0;
		font-weight: 600;
		overflow-wrap: anywhere;
	}

	.body {
		margin: 0;
		color: var(--text-muted);
		overflow-wrap: anywhere;
	}

	.new {
		padding-inline: var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		color: var(--accent-text);
		font-size: var(--text-caption-size);
	}

	.request-id {
		color: var(--text-muted);
		font-family: var(--font-mono);
		font-size: var(--text-caption-size);
		overflow-wrap: anywhere;
	}
</style>
