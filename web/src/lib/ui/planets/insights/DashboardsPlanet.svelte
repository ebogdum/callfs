<script lang="ts">
	import { base } from '$app/paths';
	import { eventHistory, metrics } from '$lib/api/admin/telemetry';
	import { toUiError } from '$lib/api/errors';
	import { t } from '$lib/copy/t';
	import { preferences } from '$lib/stores/preferences.svelte';
	import type { Annotation, UiError } from '$lib/ui/types';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Button from '../../atoms/Button.svelte';
	import IconButton from '../../atoms/IconButton.svelte';
	import Text from '../../atoms/Text.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import Tabs from '../../molecules/Tabs.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import Panel from './Panel.svelte';
	import RangeBar, { insightsRange } from './RangeBar.svelte';
	import {
		ANNOTATION_EVENT_TYPES,
		POLL_MS,
		annotationsOf,
		buildDashboards,
		metricNames,
		panelFromCustom,
		processStartedAt,
		PROCESS_START_METRIC,
		type PanelDefinition,
		type TimeRange
	} from './dashboards';

	type SharedRequest = { range: TimeRange; names: string[]; generation: number };

	const dashboards = buildDashboards();
	const CUSTOM_ID = 'custom';

	let activeId = $state<string>(dashboards[0]?.id ?? CUSTOM_ID);
	let editing = $state(false);
	let updatedAt = $state<number | null>(null);
	let revision = $state(0);
	let annotations = $state<Annotation[]>([]);
	let reportedMap = $state<Record<string, boolean> | null>(null);
	let startedAt = $state<number | null>(null);
	let loadError = $state<UiError | null>(null);
	let latestShared = 0;

	const store = insightsRange();
	const range = $derived<TimeRange>({
		from: Math.floor(store.value.from / 1000),
		to: Math.floor(store.value.to / 1000),
		live: store.value.live
	});
	const tabItems = $derived([
		...dashboards.map((dashboard) => ({ id: dashboard.id, label: dashboard.label })),
		{ id: CUSTOM_ID, label: t('dashboards.tab.custom') }
	]);
	const custom = $derived(preferences.value.dashboards.custom.map(panelFromCustom));
	const customOrder = $derived(orderedPanels(custom, preferences.value.dashboards.order));
	const hiddenIds = $derived(preferences.value.dashboards.hidden);
	const visibleCustom = $derived(customOrder.filter((panel) => !hiddenIds.includes(panel.id)));
	const hiddenCustom = $derived(customOrder.filter((panel) => hiddenIds.includes(panel.id)));
	const active = $derived(dashboards.find((dashboard) => dashboard.id === activeId));
	const activePanels = $derived<PanelDefinition[]>(undefined === active ? visibleCustom : active.panels);
	const allPanels = $derived([...dashboards.flatMap((dashboard) => dashboard.panels), ...custom]);

	function orderedPanels(panels: PanelDefinition[], order: string[]): PanelDefinition[] {
		const rank = (panel: PanelDefinition): number => {
			const index = order.indexOf(panel.id);
			return -1 === index ? order.length : index;
		};
		return [...panels].sort((left, right) => rank(left) - rank(right));
	}

	function reportedFor(panel: PanelDefinition): boolean {
		return null === reportedMap || true === reportedMap[panel.metric];
	}

	async function loadShared(request: SharedRequest): Promise<void> {
		latestShared = request.generation;
		const [catalog, events] = await Promise.all([
			metrics(request.names),
			eventHistory({ types: ANNOTATION_EVENT_TYPES, since: new Date(request.range.from * 1000).toISOString() })
		]);
		if (request.generation !== latestShared) {
			return;
		}
		reportedMap = catalog.ok
			? Object.fromEntries(catalog.value.families.map((family): [string, boolean] => [family.name, family.emitted]))
			: null;
		startedAt = catalog.ok ? processStartedAt(catalog.value.families) : null;
		if (events.ok) {
			annotations = annotationsOf(events.value.items);
		}
		const failure = !catalog.ok ? catalog.error : !events.ok ? events.error : null;
		loadError = null === failure ? null : toUiError(failure, { action: 'dashboards.load' });
		updatedAt = Date.now();
	}

	function refreshAll(): void {
		revision += 1;
	}

	function moveCustom(id: string, step: -1 | 1): void {
		const ids = customOrder.map((panel) => panel.id);
		const from = ids.indexOf(id);
		const to = from + step;
		if (0 > to || ids.length <= to) {
			return;
		}
		const next = [...ids];
		next.splice(from, 1);
		next.splice(to, 0, id);
		preferences.update({ dashboards: { order: next } });
	}

	function setHidden(id: string, hidden: boolean): void {
		const remaining = hiddenIds.filter((entry) => entry !== id);
		preferences.update({ dashboards: { hidden: hidden ? [...remaining, id] : remaining } });
	}

	$effect(() => {
		void loadShared({ range, names: [...metricNames(allPanels), PROCESS_START_METRIC], generation: revision });
	});
</script>

<PageFrame title={t('dashboards.title')} job={t('dashboards.job')}>
	{#snippet status()}
		<StaleIndicator {updatedAt} intervalMs={POLL_MS} state={range.live ? 'live' : 'paused'} onrefresh={refreshAll} />
	{/snippet}

	{#snippet toolbar()}
		<div class="toolbar">
			<RangeBar {store} />
			{#if undefined === active && 0 < custom.length}
				<Button variant="secondary" size="sm" iconStart="layers" aria-pressed={editing} onclick={() => (editing = !editing)}>
					{editing ? t('dashboards.edit.done') : t('dashboards.edit')}
				</Button>
			{/if}
		</div>
	{/snippet}

	{#snippet tabs()}
		<Tabs bind:value={activeId} items={tabItems} label={t('dashboards.tabs')} />
	{/snippet}

	<Stack gap="6">
		{#if null !== loadError}
			<InlineAlert tone="warn" title={loadError.title} body={loadError.body} />
		{/if}

		{#if undefined === active}
			{#if editing && 0 < hiddenCustom.length}
				<Cluster gap="2" align="center" wrap>
					<Text tone="muted">{t('dashboards.edit.hidden')}</Text>
					{#each hiddenCustom as panel (panel.id)}
						<Button variant="secondary" size="sm" iconStart="eye" onclick={() => setHidden(panel.id, false)}>
							{t('dashboards.edit.show', { title: panel.title })}
						</Button>
					{/each}
				</Cluster>
			{/if}
			{#if 0 === custom.length}
				<EmptyState
					variant="first-use"
					icon="line-chart"
					title={t('dashboards.empty.custom.title')}
					body={t('dashboards.empty.custom.body')}
					primary={{ label: t('dashboards.empty.custom.action'), href: `${base}/insights/explore` }}
				/>
			{/if}
		{/if}

		<div class="dashboard">
			{#each activePanels as panel, index (panel.id)}
				<div class="cell" style:--span={panel.span}>
					<Panel definition={panel} {range} reported={reportedFor(panel)} {startedAt} {annotations} syncKey="insights" {revision} />
					{#if editing && undefined === active}
						<Cluster gap="2" align="center" wrap>
							<IconButton icon="arrow-up" size="sm" variant="secondary" label={t('dashboards.edit.earlier', { title: panel.title })} disabled={0 === index} onclick={() => moveCustom(panel.id, -1)} />
							<IconButton icon="arrow-down" size="sm" variant="secondary" label={t('dashboards.edit.later', { title: panel.title })} disabled={index === activePanels.length - 1} onclick={() => moveCustom(panel.id, 1)} />
							<IconButton icon="eye-off" size="sm" variant="secondary" label={t('dashboards.edit.hide', { title: panel.title })} onclick={() => setHidden(panel.id, true)} />
						</Cluster>
					{/if}
				</div>
			{/each}
		</div>
	</Stack>
</PageFrame>

<style>
	.toolbar {
		display: flex;
		flex: 1;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
		min-inline-size: 0;
	}

	.dashboard {
		display: grid;
		grid-template-columns: repeat(12, minmax(0, 1fr));
		gap: var(--space-4);
	}

	.cell {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		grid-column: span var(--span, 12);
		min-inline-size: 0;
	}

	@media (max-width: 1023px) {
		.dashboard {
			grid-template-columns: minmax(0, 1fr);
		}

		.cell {
			grid-column: 1 / -1;
		}
	}
</style>
