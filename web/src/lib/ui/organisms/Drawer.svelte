<script lang="ts" module>
	export type DrawerSide = 'end' | 'start';
	export type DrawerWidth = 'default' | 'form' | 'wide';
	export const TOKENS = ['--drawer-bg', '--drawer-radius', '--drawer-shadow'] as const;

	const DRAWER_MIN_PX = 320;
	const DRAWER_MAX_PX = 960;
	const SHEET_LOW = 50;
	const SHEET_HIGH = 90;
	const SHEET_SNAP_MIDPOINT = 70;
</script>

<script lang="ts">
	import { Dialog } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { t } from '$lib/copy/t';
	import IconButton from '../atoms/IconButton.svelte';
	import ResizeHandle from '../molecules/ResizeHandle.svelte';
	import Scroll from '../quarks/Scroll.svelte';

	interface Props {
		open: boolean;
		title: string;
		side?: DrawerSide;
		width?: DrawerWidth;
		modal?: boolean;
		dirty?: boolean;
		children: Snippet;
		footer?: Snippet;
		class?: string;
	}

	let {
		open = $bindable(false),
		title,
		side = 'end',
		width = 'default',
		modal = false,
		dirty = false,
		children,
		footer,
		class: className
	}: Props = $props();

	let content = $state<HTMLDivElement | null>(null);
	let panel = $state<HTMLDivElement | null>(null);
	let panelWidth = $state(0);
	let sheetPercent = $state(SHEET_LOW);
	let dragging = false;

	const FIELD_SELECTOR = 'input:not([type="hidden"]):not(:disabled), textarea:not(:disabled), select:not(:disabled)';
	const FOOTER_ACTION_SELECTOR = '.foot button:not([data-tone="danger"]):not([aria-disabled="true"])';
	const dismissible = $derived(!dirty);
	const escapeBehavior = $derived(dismissible ? 'close' : 'ignore');
	const outsideBehavior = $derived(modal && dismissible ? 'close' : 'ignore');
	const dirtyReason = $derived(dismissible ? undefined : t('u092overlays.dirty_reason'));
	const inlineSize = $derived(0 < panelWidth ? `${panelWidth}px` : undefined);
	const sheetHeight = $derived(`${sheetPercent}%`);

	$effect(() => {
		if (open && null !== panel && 0 === panelWidth) {
			panelWidth = panel.getBoundingClientRect().width;
		}
	});

	function focusInitial(event: Event) {
		event.preventDefault();
		(content?.querySelector<HTMLElement>(FIELD_SELECTOR) ?? content?.querySelector<HTMLElement>(FOOTER_ACTION_SELECTOR))?.focus();
	}

	function clampPercent(value: number): number {
		return Math.min(SHEET_HIGH, Math.max(SHEET_LOW, value));
	}

	function handleGripKey(event: KeyboardEvent) {
		if ('ArrowUp' === event.key || 'End' === event.key) {
			sheetPercent = SHEET_HIGH;
		} else if ('ArrowDown' === event.key || 'Home' === event.key) {
			sheetPercent = SHEET_LOW;
		} else {
			return;
		}
		event.preventDefault();
	}

	function startGripDrag(event: PointerEvent) {
		dragging = true;
		if (event.currentTarget instanceof HTMLElement) {
			event.currentTarget.setPointerCapture(event.pointerId);
		}
	}

	function moveGripDrag(event: PointerEvent) {
		if (!dragging) {
			return;
		}
		const viewportHeight = window.innerHeight;
		sheetPercent = clampPercent(((viewportHeight - event.clientY) / viewportHeight) * 100);
	}

	function endGripDrag() {
		if (!dragging) {
			return;
		}
		dragging = false;
		sheetPercent = SHEET_SNAP_MIDPOINT > sheetPercent ? SHEET_LOW : SHEET_HIGH;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		{#if modal}
			<Dialog.Overlay>
				{#snippet child({ props })}
					<div {...props} class="scrim"></div>
				{/snippet}
			</Dialog.Overlay>
		{/if}
		<Dialog.Content
			bind:ref={content}
			escapeKeydownBehavior={escapeBehavior}
			interactOutsideBehavior={outsideBehavior}
			onOpenAutoFocus={focusInitial}
			trapFocus={modal}
			preventScroll={modal}
		>
			{#snippet child({ props })}
				<div
					{...props}
					bind:this={panel}
					class={['drawer', className]}
					data-side={side}
					data-width={width}
					data-modal={modal ? '' : undefined}
					style:--drawer-inline={inlineSize}
					style:--drawer-sheet={sheetHeight}
				>
					<div
						class="grip"
						role="separator"
						tabindex="0"
						aria-orientation="horizontal"
						aria-label={t('u092overlays.resize_panel')}
						aria-valuemin={SHEET_LOW}
						aria-valuemax={SHEET_HIGH}
						aria-valuenow={sheetPercent}
						aria-valuetext={t('u092overlays.panel_height', { percent: sheetPercent })}
						onkeydown={handleGripKey}
						onpointerdown={startGripDrag}
						onpointermove={moveGripDrag}
						onpointerup={endGripDrag}
						onpointercancel={endGripDrag}
					></div>
					<header class="head">
						<div class="titles">
							<Dialog.Title>
								<span class="title">{title}</span>
							</Dialog.Title>
						</div>
						<IconButton
							icon="x"
							label={t('u092overlays.close')}
							disabledReason={dirtyReason}
							onclick={() => {
								open = false;
							}}
						/>
					</header>
					<Scroll axis="y" label={title}>
						<div class="body">{@render children()}</div>
					</Scroll>
					{#if footer}
						<footer class="foot">{@render footer()}</footer>
					{/if}
					<div class="resize">
						<ResizeHandle
							orientation="vertical"
							bind:value={panelWidth}
							min={DRAWER_MIN_PX}
							max={DRAWER_MAX_PX}
							label={t('u092overlays.resize_width')}
						/>
					</div>
				</div>
			{/snippet}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.scrim {
		position: fixed;
		inset: 0;
		z-index: var(--z-drawer);
		background: var(--scrim);
	}

	.drawer {
		--drawer-size: var(--drawer-w);
		position: fixed;
		inset-block: 0;
		z-index: var(--z-drawer);
		display: grid;
		grid-template-rows: auto minmax(0, 1fr) auto;
		inline-size: var(--drawer-inline, var(--drawer-size));
		max-inline-size: 100%;
		background: var(--drawer-bg, var(--surface-overlay));
		color: var(--text);
		box-shadow: var(--drawer-shadow, var(--shadow-lg));
		animation: drawer-in var(--dur-large) var(--ease-out);
	}

	.drawer[data-width='form'] {
		--drawer-size: var(--drawer-w-form);
	}

	.drawer[data-width='wide'] {
		--drawer-size: var(--drawer-w-wide);
	}

	.drawer[data-side='end'] {
		--drawer-offset: 100%;
		inset-inline-end: 0;
		border-inline-start: var(--hairline) solid var(--border-subtle);
	}

	.drawer[data-side='start'] {
		--drawer-offset: -100%;
		inset-inline-start: 0;
		border-inline-end: var(--hairline) solid var(--border-subtle);
	}

	.grip {
		position: absolute;
		inset-block-start: 0;
		inset-inline: 0;
		display: none;
		place-items: center;
		block-size: var(--touch-target);
		outline: var(--focus-width) solid transparent;
		cursor: grab;
		touch-action: none;
	}

	.grip::before {
		content: '';
		inline-size: var(--space-12);
		block-size: var(--space-1);
		border-radius: var(--radius-full);
		background: var(--border-strong);
	}

	.grip:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.resize {
		position: absolute;
		inset-block: 0;
		inset-inline-start: 0;
		display: flex;
	}

	.drawer[data-side='start'] .resize {
		inset-inline-start: auto;
		inset-inline-end: 0;
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-2);
		padding: var(--space-5);
	}

	.titles {
		display: grid;
		min-inline-size: 0;
	}

	.title {
		display: block;
		font-size: var(--text-title-sm-size);
		font-weight: var(--text-title-sm-weight);
		line-height: var(--text-title-sm-line);
		letter-spacing: var(--text-title-sm-tracking);
	}

	.body {
		padding: var(--space-5);
	}

	.foot {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: var(--space-2);
		padding: var(--space-5);
	}

	@keyframes drawer-in {
		from {
			transform: translateX(var(--drawer-offset));
		}
	}

	@keyframes sheet-in {
		from {
			transform: translateY(100%);
		}
	}

	@keyframes fade-in {
		from {
			opacity: 0;
		}
	}

	@media (max-width: 639.98px) {
		.drawer {
			inset-inline: 0;
			inset-block: auto 0;
			inline-size: 100%;
			block-size: var(--drawer-sheet, 50%);
			border-start-start-radius: var(--radius-xl);
			border-start-end-radius: var(--radius-xl);
			animation: sheet-in var(--dur-large) var(--ease-out);
		}

		.drawer[data-side] {
			border: 0;
		}

		.grip {
			display: grid;
		}

		.resize {
			display: none;
		}

		.head {
			padding-block-start: var(--touch-target);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.drawer {
			animation-name: fade-in;
		}
	}

	:global(html[data-motion='reduced']) .drawer {
		animation-name: fade-in;
	}

	:global(html[data-motion='none']) .drawer {
		animation: none;
	}
</style>
