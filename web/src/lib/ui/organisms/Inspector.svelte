<script lang="ts" module>
	export type InspectorTabId = 'details' | 'placement' | 'durability' | 'links' | 'permissions' | 'activity';

	export interface InspectorTab {
		id: InspectorTabId;
		count?: number;
	}

	export const TOKENS = ['--inspector-bg', '--inspector-border', '--inspector-pad', '--inspector-gap'] as const;
</script>

<script lang="ts">
	import type { Snippet, ComponentProps } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ServerInfo, SplitFileDetail, Stat } from '$lib/api/types';
	import type { MenuItem, QueryStatus } from '../types';
	import Button from '../atoms/Button.svelte';
	import IconButton from '../atoms/IconButton.svelte';
	import ToggleButton from '../atoms/ToggleButton.svelte';
	import Heading from '../atoms/Heading.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Menu from '../molecules/Menu.svelte';
	import Tabs from '../molecules/Tabs.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import InspectorDetails from './inspector/InspectorDetails.svelte';
	import InspectorPlacement from './inspector/InspectorPlacement.svelte';
	import InspectorDurability from './inspector/InspectorDurability.svelte';
	import InspectorActivity from './inspector/InspectorActivity.svelte';
	import { t } from '$lib/copy/t';
	import { isHidden } from '$lib/format/path';

	type ActivityItems = ComponentProps<typeof InspectorActivity>['events'];
	type LockInfo = NonNullable<ComponentProps<typeof InspectorPlacement>['lock']>;
	type CacheState = NonNullable<ComponentProps<typeof InspectorPlacement>['cache']>;

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		entity: Stat | null;
		tabs: InspectorTab[];
		value?: string;
		pinned?: boolean;
		onclose?: () => void;
		ondownload?: () => void;
		actions: MenuItem[];
		server?: ServerInfo | null;
		lock?: LockInfo | null;
		cache?: CacheState | null;
		erasure?: SplitFileDetail | null;
		onverify?: () => void;
		onrepair?: () => void;
		busy?: boolean;
		events?: ActivityItems;
		activityStatus?: QueryStatus;
		links?: Snippet;
		permissions?: Snippet;
		class?: string;
	}

	let {
		entity,
		tabs,
		value = $bindable('details'),
		pinned = $bindable(false),
		onclose,
		ondownload,
		actions,
		server = null,
		lock = null,
		cache = null,
		erasure = null,
		onverify,
		onrepair,
		busy = false,
		events = [],
		activityStatus = 'success',
		links,
		permissions,
		class: className,
		...rest
	}: Props = $props();

	const tabItems = $derived(tabs.map((tab) => ({ ...tab, label: t(`inspector.tab.${tab.id}`) })));
	const activeTab = $derived(tabs.find((tab) => tab.id === value) ?? tabs[0]);
	const activeId: InspectorTabId = $derived(activeTab?.id ?? 'details');

	function handleKeydown(event: KeyboardEvent): void {
		if ('Escape' === event.key) {
			event.preventDefault();
			onclose?.();
		}
	}
</script>

<aside class={['inspector', className]} aria-label={t('inspector.label')} onkeydown={handleKeydown} {...rest}>
	{#if null === entity}
		<EmptyState variant="cleared" icon="inspector" title={t('inspector.empty.title')} body={t('inspector.empty.body')} />
	{:else}
		<header class="header">
			<div class="title">
				<Heading level={2} size="title-sm">{entity.name}</Heading>
				<div class="pills">
					{#if entity.locked}<StatusPill status="file.locked" size="sm" />{/if}
					{#if entity.erasure_coded}<StatusPill status="file.split" size="sm" />{/if}
					{#if !entity.is_local}<StatusPill status="file.remote" size="sm" />{/if}
					{#if '' === entity.owner}<StatusPill status="file.no_owner" size="sm" />{/if}
					{#if 'file' === entity.type && '' === entity.etag}<StatusPill status="file.no_tag" size="sm" />{/if}
					{#if isHidden(entity.path)}<StatusPill status="file.hidden" size="sm" />{/if}
				</div>
			</div>
			<div class="actions">
				{#if 'file' === entity.type}
					<Button variant="primary" size="sm" iconStart="download" onclick={ondownload}>
						{t('inspector.action.download')}
					</Button>
				{/if}
				<ToggleButton bind:pressed={pinned} label={t('inspector.pin')} icon="pin" />
				<Menu items={actions} label={t('inspector.actions.more')}>
					{#snippet trigger(props)}
						<IconButton icon="more" label={t('inspector.actions.more')} {...props} />
					{/snippet}
				</Menu>
				<IconButton icon="x" label={t('common.close')} onclick={onclose} />
			</div>
		</header>
		<Tabs bind:value items={tabItems} label={t('inspector.tabs.label')} size="sm" />
		<div class="body">
			<Scroll axis="y" label={t(`inspector.tab.${activeId}`)}>
				<div class="panel" role="tabpanel" aria-label={t(`inspector.tab.${activeId}`)}>
					{#if 'details' === activeId}
						<InspectorDetails stat={entity} />
					{:else if 'placement' === activeId}
						<InspectorPlacement stat={entity} {server} {lock} {cache} />
					{:else if 'durability' === activeId}
						<InspectorDurability {erasure} {onverify} {onrepair} {busy} />
					{:else if 'links' === activeId}
						{@render links?.()}
					{:else if 'permissions' === activeId}
						{@render permissions?.()}
					{:else}
						<InspectorActivity events={events} status={activityStatus} />
					{/if}
				</div>
			</Scroll>
		</div>
	{/if}
</aside>

<style>
	.inspector {
		display: flex;
		flex-direction: column;
		gap: var(--inspector-gap, var(--space-4));
		box-sizing: border-box;
		block-size: 100%;
		min-inline-size: 0;
		min-block-size: 0;
		padding: var(--inspector-pad, var(--space-4));
		background: var(--inspector-bg, var(--surface-raised));
		border-inline-start: var(--hairline) solid var(--inspector-border, var(--border-subtle));
	}

	.header {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--space-2);
	}

	.title {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	.pills {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
	}

	.actions {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}

	.body {
		display: flex;
		flex: 1 1 auto;
		min-block-size: 0;
	}

	.body > :global(*) {
		flex: 1 1 auto;
		min-inline-size: 0;
	}

	.panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding-block: var(--space-1);
	}
</style>
