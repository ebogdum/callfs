<script lang="ts" module>
	export type IndicatorState = 'live' | 'stale' | 'paused' | 'offline';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatRelative, formatTimestamp } from '$lib/format/time';
	import { t } from '$lib/copy/t';
	import Button from '$lib/ui/atoms/Button.svelte';
	import StatusGlyph from '$lib/ui/quarks/StatusGlyph.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		updatedAt: number | null;
		intervalMs: number;
		state: IndicatorState;
		onrefresh: () => void;
	}

	type BadgeKey = 'offline' | 'paused' | 'stale';

	const BADGES = {
		offline: { tone: 'danger', word: 'u080molecules.stale.offline' },
		paused: { tone: 'muted', word: 'u080molecules.stale.paused' },
		stale: { tone: 'warn', word: 'u080molecules.stale.word' }
	} as const;

	const badgeFor = (indicatorState: IndicatorState, overdue: boolean): BadgeKey | null => {
		if ('offline' === indicatorState) return 'offline';
		if ('paused' === indicatorState) return 'paused';
		if ('stale' === indicatorState || overdue) return 'stale';
		return null;
	};

	let {
		updatedAt,
		intervalMs,
		state: indicatorState,
		onrefresh,
		class: className,
		...rest
	}: Props = $props();

	let now = $state(Date.now());

	$effect(() => {
		const timer = setInterval(() => {
			now = Date.now();
		}, 1000);
		return () => clearInterval(timer);
	});

	const overdue = $derived(null !== updatedAt && intervalMs * 2 < now - updatedAt);
	const badgeKey = $derived(badgeFor(indicatorState, overdue));
	const JUST_NOW_MS = 5000;
	const sinceUpdate = $derived(null === updatedAt ? 0 : Math.max(0, now - updatedAt));
	const updatedText = $derived(
		null === updatedAt || JUST_NOW_MS > sinceUpdate
			? t('u080molecules.stale.just_now')
			: t('u080molecules.stale.updated', { when: formatRelative(now - sinceUpdate, now) })
	);
</script>

<div class={['stale-indicator', className]} {...rest}>
	<Cluster gap="2" align="center">
		{#if null === updatedAt}
			<span class="updated">{t('u080molecules.stale.not_yet')}</span>
		{:else}
			<time class="updated" datetime={new Date(updatedAt).toISOString()} title={formatTimestamp(updatedAt)}>
				{updatedText}
			</time>
		{/if}
		{#if null !== badgeKey}
			<span class="badge" data-tone={BADGES[badgeKey].tone}>
				<StatusGlyph tone={BADGES[badgeKey].tone} />
				<span>{t(BADGES[badgeKey].word)}</span>
			</span>
		{/if}
		<Button variant="ghost" size="sm" iconStart="refresh" onclick={onrefresh}>
			{#snippet children()}{t('u080molecules.stale.refresh')}{/snippet}
		</Button>
	</Cluster>
</div>

<style>
	.stale-indicator {
		display: block;
		color: var(--stale-indicator-fg, var(--text-muted));
		font-size: var(--text-sm-size);
	}

	.updated {
		font-variant-numeric: tabular-nums;
	}

	.badge {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-weight: 600;
	}

	.badge[data-tone='warn'] {
		color: var(--stale-indicator-warn, var(--warn-text));
	}

	.badge[data-tone='danger'] {
		color: var(--stale-indicator-danger, var(--danger-text));
	}
</style>
