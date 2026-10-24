<script lang="ts">
	import { toUiError } from '$lib/api/errors';
	import { eventHistory } from '$lib/api/admin/telemetry';
	import { myEvents } from '$lib/api/session';
	import type { Event as ActivityEvent, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { UiError } from '$lib/ui/types';
	import Button from '../../atoms/Button.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import EventFeed from '../../organisms/EventFeed.svelte';
	import ActivityRange, { activityRange, rangeKey, rangeSince } from './ActivityRange.svelte';

	interface EventPage {
		items: ActivityEvent[];
	}

	const EVENT_LIMIT = 500;
	const POLL_MS = 5000;
	const LIVE_TOPICS = [
		'file.*',
		'link.*',
		'cluster.*',
		'server.*',
		'config.*',
		'job.*',
		'health.*',
		'finding.*',
		'security.*'
	];

	const range = activityRange();

	function isTypingTarget(target: EventTarget | null): boolean {
		return (
			target instanceof HTMLElement &&
			(['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) || true === target.isContentEditable)
		);
	}

	function onKeydown(event: KeyboardEvent): void {
		if (
			't' === event.key &&
			false === event.metaKey &&
			false === event.ctrlKey &&
			false === event.altKey &&
			false === isTypingTarget(event.target) &&
			undefined !== range.value.preset
		) {
			range.value = { ...range.value, live: !range.value.live };
		}
	}

	function togglePause(): void {
		if (paused) {
			frozen = null;
			paused = false;
			return;
		}
		frozen = latest;
		paused = true;
	}

	function showNew(): void {
		frozen = null;
		paused = false;
	}

	let selectedGroups = $state<string[]>([]);
	let paused = $state(false);
	let frozen = $state<ActivityEvent[] | null>(null);

	const live = $derived(range.value.live);

	async function loadEvents(): Promise<Result<EventPage, UiError>> {
		const since = rangeSince(range.value);
		if (true === session.isAdmin) {
			const result = await eventHistory({ since });
			return result.ok
				? result
				: { ok: false, error: toUiError(result.error, { action: 'activity.events.action.load' }) };
		}
		return myEvents({ since, limit: EVENT_LIMIT });
	}

	const eventsQuery = createQuery<Result<EventPage, UiError>>(() => ({
		key: ['events', session.isAdmin ? 'admin' : 'own', rangeKey(range.value)],
		fetch: loadEvents,
		refetchMs: live ? POLL_MS : undefined,
		liveTopics: LIVE_TOPICS,
		isEmpty: (result) => result.ok && 0 === result.value.items.length,
		isPartial: (result) => result.ok && EVENT_LIMIT <= result.value.items.length
	}));

	const loadError = $derived(
		eventsQuery.error ?? (eventsQuery.data?.ok === false ? eventsQuery.data.error : null)
	);
	const latest = $derived(eventsQuery.data?.ok ? eventsQuery.data.value.items : []);
	const frozenIds = $derived.by(() => new Set((frozen ?? []).map((event) => event.id)));
	const source = $derived.by(() => (null === frozen ? latest : frozen));
	const newCount = $derived.by(() =>
		null === frozen ? 0 : latest.filter((event) => false === frozenIds.has(event.id)).length
	);
	const indicatorState = $derived.by(() =>
		null !== loadError ? 'offline' : paused || false === live ? 'paused' : 'live'
	);
</script>

<svelte:window onkeydown={onKeydown} />

<PageFrame title={t('activity.events.title')} job={t('activity.events.job')}>
	{#snippet status()}
		<StaleIndicator
			updatedAt={eventsQuery.updatedAt}
			intervalMs={POLL_MS}
			state={indicatorState}
			onrefresh={() => eventsQuery.refetch()}
		/>
	{/snippet}
	{#snippet secondary()}
		{#if live || paused}
			<Button variant="secondary" iconStart={paused ? 'play' : 'pause'} onclick={togglePause}>
				{paused ? t('activity.events.resume') : t('activity.events.pause')}
			</Button>
		{/if}
	{/snippet}
	{#snippet toolbar()}
		<ActivityRange store={range} />
	{/snippet}

	{#if 'loading' === eventsQuery.status}
		<Stack gap="2">
			<Skeleton shape="rect" height="var(--space-10)" />
			<Skeleton shape="rect" height="var(--space-10)" />
			<Skeleton shape="rect" height="var(--space-10)" />
		</Stack>
	{:else if null !== loadError && 0 === source.length}
		<ErrorState error={loadError} variant="region" onretry={() => eventsQuery.refetch()} />
	{:else if 0 === source.length}
		<EmptyState
			variant="no-results"
			icon="activity"
			title={t('activity.events.empty.title')}
			body={t('activity.events.empty.body')}
		/>
	{:else}
		<Stack gap="4">
			{#if null !== loadError}
				<InlineAlert tone="danger" title={loadError.title} body={loadError.body} />
			{/if}
			{#if 'partial' === eventsQuery.status}
				<InlineAlert tone="info" title={t('activity.events.partial', { count: EVENT_LIMIT })} />
			{/if}
			{#if 0 < newCount}
				<div class="new">
					<Button size="sm" variant="primary" iconStart="arrow-up" onclick={showNew}>
						{t('activity.events.newCount', { count: newCount })}
					</Button>
				</div>
			{/if}
			<div class="feed">
				<EventFeed events={source} bind:types={selectedGroups} live={live && false === paused} />
			</div>
		</Stack>
	{/if}
</PageFrame>

<style>
	.new {
		display: flex;
		justify-content: center;
	}

	.feed {
		padding: var(--card-pad);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		min-inline-size: 0;
	}
</style>
