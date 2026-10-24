<script lang="ts" module>
	import type { UiError } from '../types';

	export type TransferState =
		| 'queued'
		| 'uploading'
		| 'downloading'
		| 'paused'
		| 'stalled'
		| 'verifying'
		| 'done'
		| 'failed'
		| 'cancelled'
		| 'conflict';

	export type TransferTab = 'active' | 'queued' | 'failed' | 'done';

	export interface TransferItem {
		id: string;
		name: string;
		destination: string;
		state: TransferState;
		done: number;
		total: number | null;
		partsDone: number | null;
		partsTotal: number | null;
		bytesPerSecond: number;
		eta: number | null;
		resumedAt: number | null;
		error: UiError | null;
	}

	export interface TrayAggregate {
		done: number;
		total: number | null;
		bytesPerSecond: number;
		eta: number | null;
	}

	export const TOKENS = [
		'--transfer-tray-bg',
		'--transfer-tray-border',
		'--transfer-tray-radius',
		'--transfer-tray-pad',
		'--transfer-tray-width'
	] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatDuration } from '$lib/format/duration';
	import { formatNumber } from '$lib/format/number';
	import Badge from '../atoms/Badge.svelte';
	import Button from '../atoms/Button.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import ProgressCell from '../cells/ProgressCell.svelte';
	import Tabs from '../molecules/Tabs.svelte';
	import { announce } from '../quarks/announce';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import type { StatusKey } from '../types';

	type ItemAction = 'pause' | 'resume' | 'cancel' | 'retry';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'onpause' | 'onresume' | 'oncancel'> {
		items: TransferItem[];
		aggregate: TrayAggregate;
		collapsed?: boolean;
		onpause?: (id: string) => void;
		onresume?: (id: string) => void;
		oncancel?: (id: string) => void;
		onretry?: (id: string) => void;
		onretryfailed?: () => void;
		onclear?: () => void;
	}

	const BODY_ID = 'transfer-tray-body';
	const TABS: readonly TransferTab[] = ['active', 'queued', 'failed', 'done'];
	const TAB_LABELS: Record<TransferTab, string> = {
		active: 'transfers.tab.active',
		queued: 'transfers.tab.queued',
		failed: 'transfers.tab.failed',
		done: 'transfers.tab.done'
	};
	const TAB_STATES: Record<TransferTab, readonly TransferState[]> = {
		active: ['uploading', 'downloading', 'paused', 'stalled', 'verifying'],
		queued: ['queued', 'conflict'],
		failed: ['failed'],
		done: ['done', 'cancelled']
	};
	const CLEARABLE_STATES: readonly TransferState[] = ['done', 'cancelled', 'failed'];
	const ACTION_LABELS: Record<ItemAction, string> = {
		pause: 'transfers.action.pause',
		resume: 'transfers.action.resume',
		cancel: 'transfers.action.cancel',
		retry: 'transfers.action.retry'
	};
	const ACTIONS_BY_STATE: Record<TransferState, readonly ItemAction[]> = {
		queued: ['cancel'],
		uploading: ['pause', 'cancel'],
		downloading: ['pause', 'cancel'],
		paused: ['resume', 'cancel'],
		stalled: ['retry', 'cancel'],
		verifying: ['cancel'],
		done: [],
		failed: ['retry'],
		cancelled: [],
		conflict: []
	};

	let {
		items,
		aggregate,
		collapsed = $bindable(false),
		onpause,
		onresume,
		oncancel,
		onretry,
		onretryfailed,
		onclear,
		class: className,
		...rest
	}: Props = $props();

	let tab = $state<TransferTab>('active');
	let previousFinished: number | null = null;

	const isTransferTab = (value: string): value is TransferTab => (TABS as readonly string[]).includes(value);

	const chooseTab = (next: string): void => {
		if (isTransferTab(next)) {
			tab = next;
		}
	};

	const itemsIn = (id: TransferTab): TransferItem[] => items.filter((item) => TAB_STATES[id].includes(item.state));

	const visibleItems = $derived(itemsIn(tab));
	const tabItems = $derived(TABS.map((id) => ({ id, label: t(TAB_LABELS[id]), count: itemsIn(id).length })));
	const unfinishedCount = $derived(itemsIn('active').length + itemsIn('queued').length);
	const hasFailed = $derived(0 < itemsIn('failed').length);
	const hasClearable = $derived(items.some((item) => CLEARABLE_STATES.includes(item.state)));
	const finishedCount = $derived(items.filter((item) => 'done' === item.state).length);

	const percent = $derived(
		null === aggregate.total || 0 === aggregate.total
			? 0
			: Math.min(100, Math.round((aggregate.done / aggregate.total) * 100))
	);
	const summary = $derived(
		t('transfers.tray.summary', { count: formatNumber(unfinishedCount), percent: formatNumber(percent) })
	);

	$effect(() => {
		const current = finishedCount;
		if (null !== previousFinished && current > previousFinished) {
			announce(t('transfers.announce.finished', { count: current - previousFinished }), 'polite');
		}
		previousFinished = current;
	});

	const toggle = (): void => {
		collapsed = !collapsed;
	};

	const statusOf = (item: TransferItem): StatusKey => `transfer.${item.state}`;

	const actionsFor = (item: TransferItem): readonly ItemAction[] => ACTIONS_BY_STATE[item.state];

	const runAction = (action: ItemAction, id: string): void => {
		if ('pause' === action) {
			onpause?.(id);
		} else if ('resume' === action) {
			onresume?.(id);
		} else if ('cancel' === action) {
			oncancel?.(id);
		} else {
			onretry?.(id);
		}
	};

	const actionName = (action: ItemAction, name: string): string =>
		t('transfers.action.named', { action: t(ACTION_LABELS[action]), name });
</script>

<section
	class={['transfer-tray', className]}
	aria-label={t('transfers.tray.title')}
	data-state={collapsed ? 'collapsed' : 'open'}
	{...rest}
>
	<Cluster gap="2" justify="space-between">
		<Button variant="ghost" aria-expanded={!collapsed} aria-controls={BODY_ID} onclick={toggle}>
			<span>{t('transfers.tray.title')}</span>
			<Badge tone="accent" variant="solid">{summary}</Badge>
		</Button>
	</Cluster>
	<div id={BODY_ID} class="body" hidden={collapsed}>
		<Stack gap="3">
			<Tabs
				bind:value={() => tab, chooseTab}
				items={tabItems}
				label={t('transfers.tray.tabs')}
			/>
			<div role="tabpanel" aria-label={t(TAB_LABELS[tab])}>
				{#if 0 === items.length}
					<Stack gap="1">
						<p class="empty-title">{t('transfers.tray.empty.title')}</p>
						<p class="muted">{t('transfers.tray.empty.body')}</p>
					</Stack>
				{:else if 0 === visibleItems.length}
					<p class="muted">{t('transfers.tab.empty')}</p>
				{:else}
					<ul class="list">
						{#each visibleItems as item (item.id)}
							<li class="item" data-state={item.state}>
								<Stack gap="1">
									<Cluster gap="2">
										<span class="name">{item.name}</span>
										<StatusPill status={statusOf(item)} size="sm" />
									</Cluster>
									<span class="muted">{item.destination}</span>
									<ProgressCell
										done={item.done}
										total={item.total}
										state={statusOf(item)}
										bytesPerSecond={item.bytesPerSecond}
									/>
									<Cluster gap="2">
										{#if null !== item.partsTotal}
											<span class="muted">
												{t('transfers.item.parts', {
													done: formatNumber(item.partsDone),
													total: formatNumber(item.partsTotal)
												})}
											</span>
										{/if}
										{#if null !== item.resumedAt}
											<span class="muted">
												{t('transfers.item.resumed', { size: formatBytes(item.resumedAt).text })}
											</span>
										{/if}
										{#if 0 < item.bytesPerSecond}
											<span class="muted">
												{t('transfers.item.speed', { speed: formatBytes(item.bytesPerSecond).text })}
											</span>
										{/if}
										{#if null !== item.eta}
											<span class="muted">
												{t('transfers.item.timeLeft', { time: formatDuration(item.eta, 'short') })}
											</span>
										{/if}
									</Cluster>
									{#if 'stalled' === item.state}
										<p class="warning">{t('transfers.item.stalled')}</p>
									{/if}
									{#if null !== item.error}
										<p class="warning">{item.error.body}</p>
									{/if}
									{#if 0 < actionsFor(item).length}
										<Cluster gap="2">
											{#each actionsFor(item) as action (action)}
												<Button
													size="sm"
													variant="ghost"
													aria-label={actionName(action, item.name)}
													onclick={() => runAction(action, item.id)}
												>
													{t(ACTION_LABELS[action])}
												</Button>
											{/each}
										</Cluster>
									{/if}
								</Stack>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
			{#if hasFailed || hasClearable}
				<Cluster gap="2">
					{#if hasFailed}
						<Button size="sm" onclick={() => onretryfailed?.()}>{t('transfers.action.retryFailed')}</Button>
					{/if}
					{#if hasClearable}
						<Button size="sm" variant="ghost" onclick={() => onclear?.()}>
							{t('transfers.action.clearFinished')}
						</Button>
					{/if}
				</Cluster>
			{/if}
		</Stack>
	</div>
</section>

<style>
	.transfer-tray {
		display: grid;
		gap: var(--space-3);
		justify-self: end;
		inline-size: min(var(--transfer-tray-width, var(--drawer-w)), 100%);
		padding: var(--transfer-tray-pad, var(--card-pad));
		background: var(--transfer-tray-bg, var(--surface-raised));
		border: var(--hairline) solid var(--transfer-tray-border, var(--border-subtle));
		border-radius: var(--transfer-tray-radius, var(--radius-lg));
		box-shadow: var(--shadow-xs);
		color: var(--text);
		font-family: var(--font-sans);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.list {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.item {
		min-inline-size: 0;
	}

	.name {
		min-inline-size: 0;
		overflow-wrap: anywhere;
		font-weight: var(--text-body-weight);
	}

	.muted {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		overflow-wrap: anywhere;
	}

	.warning {
		color: var(--warn-text);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		overflow-wrap: anywhere;
	}

	.empty-title {
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
	}
</style>
