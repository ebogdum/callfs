<script lang="ts" module>
	import type { Snippet } from 'svelte';

	export interface DashboardPanel {
		id: string;
		title: string;
		span: 3 | 4 | 6 | 8 | 12;
		rows: 1 | 2;
		content: Snippet;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import IconButton from '../atoms/IconButton.svelte';
	import { announce } from '../quarks/announce';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'hidden'> {
		panels: DashboardPanel[];
		editable?: boolean;
		order: string[];
		hidden: string[];
		oninspect?: (id: string) => void;
	}

	let {
		panels,
		editable = false,
		order = $bindable(),
		hidden = $bindable(),
		oninspect,
		class: className,
		...rest
	}: Props = $props();

	let expandedId: string | undefined = $state();

	const panelById = $derived(new Map(panels.map((panel) => [panel.id, panel])));
	const orderedIds = $derived([
		...new Set([
			...order.filter((id) => panelById.has(id)),
			...panels.map((panel) => panel.id)
		])
	]);
	const orderedPanels = $derived(
		orderedIds.flatMap((id) => {
			const panel = panelById.get(id);
			return panel ? [panel] : [];
		})
	);
	const hiddenIds = $derived(new Set(hidden));
	const visiblePanels = $derived(orderedPanels.filter((panel) => !hiddenIds.has(panel.id)));
	const hiddenPanels = $derived(orderedPanels.filter((panel) => hiddenIds.has(panel.id)));

	function movePanel(panel: DashboardPanel, step: -1 | 1): void {
		const index = visiblePanels.indexOf(panel);
		const targetIndex = index + step;
		const target = visiblePanels[targetIndex];
		if (0 > index || 0 > targetIndex || undefined === target) {
			return;
		}
		const next = [...orderedIds];
		const from = next.indexOf(panel.id);
		const to = next.indexOf(target.id);
		next[from] = target.id;
		next[to] = panel.id;
		order = next;
		announce(
			t('u111dashboard.grid.moved', {
				title: panel.title,
				position: index + step + 1,
				total: visiblePanels.length
			}),
			'polite'
		);
	}

	function hidePanel(panel: DashboardPanel): void {
		hidden = [...hidden, panel.id];
		announce(t('u111dashboard.grid.hidden', { title: panel.title }), 'polite');
	}

	function showPanel(panel: DashboardPanel): void {
		hidden = hidden.filter((id) => panel.id !== id);
		announce(t('u111dashboard.grid.shown', { title: panel.title }), 'polite');
	}

	function toggleFullScreen(id: string): void {
		expandedId = id === expandedId ? undefined : id;
	}

	function isTypingTarget(target: EventTarget | null): boolean {
		return (
			target instanceof HTMLElement &&
			(target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
		);
	}

	function handlePanelKeydown(event: KeyboardEvent, panel: DashboardPanel): void {
		if (
			'v' !== event.key ||
			event.altKey ||
			event.ctrlKey ||
			event.metaKey ||
			isTypingTarget(event.target)
		) {
			return;
		}
		event.preventDefault();
		toggleFullScreen(panel.id);
	}
</script>

<div class={['dashboard-grid', className]} data-editable={editable} {...rest}>
	<div class="panels">
		{#each visiblePanels as panel, index (panel.id)}
			<section
				class={['panel', `span-${panel.span}`, `rows-${panel.rows}`]}
				data-state={panel.id === expandedId ? 'expanded' : undefined}
				aria-labelledby={`panel-${panel.id}-title`}
				onkeydown={(event: KeyboardEvent) => handlePanelKeydown(event, panel)}
			>
				<header class="panel-head">
					<h2 class="panel-title" id={`panel-${panel.id}-title`}>{panel.title}</h2>
					<div class="panel-actions" role="group" aria-label={t('shell.panel.actions')}>
						{#if editable}
							<IconButton
								icon="arrow-left"
								size="sm"
								label={t('u111dashboard.grid.named_action', {
									action: t('u111dashboard.grid.move_earlier'),
									title: panel.title
								})}
								disabled={0 === index}
								disabledReason={0 === index ? t('u111dashboard.grid.first_panel') : undefined}
								onclick={() => movePanel(panel, -1)}
							/>
							<IconButton
								icon="arrow-right"
								size="sm"
								label={t('u111dashboard.grid.named_action', {
									action: t('u111dashboard.grid.move_later'),
									title: panel.title
								})}
								disabled={index === visiblePanels.length - 1}
								disabledReason={index === visiblePanels.length - 1 ? t('u111dashboard.grid.last_panel') : undefined}
								onclick={() => movePanel(panel, 1)}
							/>
							<IconButton
								icon="eye-off"
								size="sm"
								label={t('u111dashboard.grid.named_action', {
									action: t('u111dashboard.grid.hide'),
									title: panel.title
								})}
								onclick={() => hidePanel(panel)}
							/>
						{/if}
						{#if oninspect}
							<IconButton
								icon="inspector"
								size="sm"
								label={t('u111dashboard.grid.inspect')}
								onclick={() => oninspect(panel.id)}
							/>
						{/if}
						<IconButton
							icon={panel.id === expandedId ? 'minimize' : 'maximize'}
							size="sm"
							label={panel.id === expandedId
								? t('u111dashboard.grid.exit_full_screen')
								: t('u111dashboard.grid.full_screen')}
							pressed={panel.id === expandedId}
							onclick={() => toggleFullScreen(panel.id)}
						/>
					</div>
				</header>
				<div class="panel-body">
					{@render panel.content()}
				</div>
			</section>
		{/each}
	</div>
	{#if editable && 0 < hiddenPanels.length}
		<section class="hidden-tray" aria-labelledby="dashboard-hidden-title">
			<h2 class="hidden-title" id="dashboard-hidden-title">{t('u111dashboard.grid.hidden_title')}</h2>
			<div class="hidden-list">
				{#each hiddenPanels as panel (panel.id)}
					<Button variant="secondary" size="sm" iconStart="eye" onclick={() => showPanel(panel)}>
						{t('u111dashboard.grid.show_panel', { title: panel.title })}
					</Button>
				{/each}
			</div>
		</section>
	{/if}
</div>

<style>
	.dashboard-grid {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.panels {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		grid-auto-flow: row dense;
		gap: var(--space-4);
		inline-size: 100%;
		min-inline-size: 0;
	}

	.panel {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		box-shadow: var(--shadow-xs);
		overflow: hidden;
	}

	.panel:focus-within {
		border-color: var(--border);
	}

	.panel[data-state='expanded'] {
		grid-column: 1 / -1;
	}

	.panel-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-block-size: calc(var(--control-h) + var(--space-5));
		padding-block: var(--space-2);
		padding-inline: var(--space-4) var(--space-2);
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.panel-title {
		margin: 0;
		min-inline-size: 0;
		overflow: hidden;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.panel-actions {
		display: flex;
		flex: none;
		align-items: center;
		gap: var(--space-0-5);
		color: var(--text-muted);
	}

	.panel-body {
		display: flex;
		flex: auto;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
		max-inline-size: 100%;
		padding: var(--space-4);
		overflow-x: auto;
	}

	@media (min-width: 48rem) {
		.panels {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.span-8,
		.span-12 {
			grid-column: 1 / -1;
		}
	}

	@media (min-width: 72rem) {
		.panels {
			grid-template-columns: repeat(12, minmax(0, 1fr));
		}

		.span-3 {
			grid-column: span 3;
		}

		.span-4 {
			grid-column: span 4;
		}

		.span-6 {
			grid-column: span 6;
		}

		.span-8 {
			grid-column: span 8;
		}

		.span-12 {
			grid-column: 1 / -1;
		}

		.rows-2 {
			grid-row: span 2;
		}

		.panel[data-state='expanded'] {
			grid-column: 1 / -1;
		}
	}

	.hidden-tray {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-4);
		border: var(--hairline) dashed var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-sunken);
	}

	.hidden-title {
		margin: 0;
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: 600;
		letter-spacing: var(--text-caption-tracking);
		color: var(--text-muted);
	}

	.hidden-list {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
