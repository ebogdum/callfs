<script lang="ts" module>
	import type { Unit } from '../types';

	export const TOKENS = ['--stat-tile-label', '--stat-tile-value', '--stat-tile-help'] as const;

	const SUFFIXES: Partial<Record<Unit, string>> = {
		bytes_per_s: '/s',
		per_s: '/s',
		percent: '%',
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { StatusKey } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Bytes from '../atoms/Bytes.svelte';
	import Delta from '../atoms/Delta.svelte';
	import Duration from '../atoms/Duration.svelte';
	import Link from '../atoms/Link.svelte';
	import Num from '../atoms/Num.svelte';
	import Percent from '../atoms/Percent.svelte';
	import Skeleton from '../atoms/Skeleton.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Surface from '../quarks/Surface.svelte';
	import Sparkline from './Sparkline.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		label: string;
		value: number | null;
		unit: Unit;
		base?: { num: number; den: number };
		delta?: { value: number; goodWhen: 'up' | 'down' | 'none'; period: string };
		status?: StatusKey;
		sparkline?: (number | null)[];
		href?: string;
		help?: string;
		loading?: boolean;
		class?: string;
	}

	let { label, value, unit, base, delta, status, sparkline, href, help, loading = false, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const labelId = `${uid}-label`;
	const helpId = `${uid}-help`;

	const valueKind = $derived<'bytes' | 'duration' | 'number'>(
		'bytes' === unit || 'bytes_per_s' === unit ? 'bytes' : 'ms' === unit || 's' === unit ? 'duration' : 'number'
	);
	const durationMs = $derived(null === value ? 0 : 's' === unit ? value * 1000 : value);
	const suffix = $derived(SUFFIXES[unit] ?? '');
</script>

<Surface
	pad="card"
	radius="lg"
	as="section"
	class={['stat-tile', className]}
	data-state={null !== value ? 'success' : loading ? 'loading' : 'empty'}
	aria-busy={null === value && loading ? 'true' : undefined}
	aria-labelledby={labelId}
	aria-describedby={help && null !== value ? helpId : undefined}
	{...rest}
>
	<div class="head">
		<p class="label" id={labelId}>{label}</p>
		{#if status}
			<StatusPill {status} size="sm" />
		{/if}
	</div>
	{#if null === value && loading}
		<Skeleton shape="text" width="60%" height="var(--text-display-line)" />
	{:else if null === value}
		<p class="empty">{help ?? t('u077molecules.no_value')}</p>
	{:else}
		<Cluster gap="1" align="baseline" wrap={false} class="stat-tile-value-line">
			{#if 'bytes' === valueKind}
				<Bytes {value} class="value" />
			{:else if 'duration' === valueKind}
				<Duration value={durationMs} style="short" class="value" />
			{:else}
				<Num {value} style={'percent' === unit ? 'decimal' : 'int'} class="value" />
			{/if}
			{#if '' !== suffix}<span class="suffix">{suffix}</span>{/if}
		</Cluster>
		{#if (base && 0 < base.den) || delta}
			<div class="meta">
				{#if base && 0 < base.den}
					<Cluster gap="1" class="stat-tile-base">
						<span>{t('u077molecules.base', { total: formatNumber(base.den, 'int') })}</span>
						<Percent num={base.num} den={base.den} style="rate-only" />
					</Cluster>
				{/if}
				{#if delta}
					<Delta value={delta.value} {unit} goodWhen={delta.goodWhen} period={delta.period} />
				{/if}
			</div>
		{/if}
		{#if help}
			<p class="help" id={helpId}>{help}</p>
		{/if}
	{/if}
	{#if sparkline && null !== value}
		<div class="spark"><Sparkline values={sparkline} tone="accent" height="md" /></div>
	{/if}
	{#if href}
		<div class="foot"><Link {href}>{t('u077molecules.view_details')}</Link></div>
	{/if}
</Surface>

<style>
	:global(.stat-tile) {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		block-size: 100%;
		min-inline-size: 0;
	}

	.head {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-1-5);
		min-block-size: var(--space-5);
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-3);
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.spark {
		--sparkline-w: 100%;
		--sparkline-h: var(--space-8);
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		justify-content: flex-end;
		padding-block-start: var(--space-2);
	}

	.foot {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		align-items: flex-start;
		justify-content: flex-end;
		padding-block-start: var(--space-1);
	}

	.spark + .foot {
		flex-grow: 0;
	}

	.label {
		margin: 0;
		color: var(--stat-tile-label, var(--text-muted));
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
	}

	:global(.stat-tile-value-line),
	:global(.stat-tile-base) {
		font-variant-numeric: tabular-nums;
	}

	:global(.stat-tile-value-line) :global(.value) {
		color: var(--stat-tile-value, var(--text));
		font-size: var(--text-display-size);
		line-height: var(--text-display-line);
		font-weight: var(--text-display-weight);
		letter-spacing: var(--text-display-tracking);
	}

	.suffix {
		color: var(--text-muted);
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
	}

	:global(.stat-tile-base) {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.empty {
		display: flex;
		align-items: center;
		min-block-size: var(--text-display-line);
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		text-wrap: pretty;
	}

	.help {
		margin: 0;
		color: var(--stat-tile-help, var(--text-muted));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		text-wrap: pretty;
	}
</style>
