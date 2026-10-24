<script lang="ts" module>
	export type DialogSize = 'sm' | 'md' | 'lg';
	export const TOKENS = ['--dialog-bg', '--dialog-radius', '--dialog-shadow'] as const;
</script>

<script lang="ts">
	import { Dialog } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import IconButton from '../atoms/IconButton.svelte';
	import Scroll from '../quarks/Scroll.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title' | 'children'> {
		open: boolean;
		title: string;
		description?: string;
		size?: DialogSize;
		dismissible?: boolean;
		children: Snippet;
		footer?: Snippet;
	}

	let {
		open = $bindable(false),
		title,
		description,
		size = 'md',
		dismissible = true,
		children,
		footer,
		class: className,
		...rest
	}: Props = $props();

	let content = $state<HTMLDivElement | null>(null);

	const FIELD_SELECTOR = 'input:not([type="hidden"]):not(:disabled), textarea:not(:disabled), select:not(:disabled)';
	const FOOTER_ACTION_SELECTOR = '.foot button:not([data-tone="danger"]):not([aria-disabled="true"])';
	const closeBehavior = $derived(dismissible ? 'close' : 'ignore');
	const dirtyReason = $derived(dismissible ? undefined : t('u092overlays.dirty_reason'));

	function focusInitial(event: Event) {
		event.preventDefault();
		(content?.querySelector<HTMLElement>(FIELD_SELECTOR) ?? content?.querySelector<HTMLElement>(FOOTER_ACTION_SELECTOR))?.focus();
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay>
			{#snippet child({ props })}
				<div {...props} class="scrim"></div>
			{/snippet}
		</Dialog.Overlay>
		<Dialog.Content
			bind:ref={content}
			escapeKeydownBehavior={closeBehavior}
			interactOutsideBehavior={closeBehavior}
			onOpenAutoFocus={focusInitial}
		>
			{#snippet child({ props })}
				<div {...rest} {...props} class={['dialog', className]} data-size={size}>
					<header class="head">
						<div class="titles">
							<Dialog.Title>
								<span class="title">{title}</span>
							</Dialog.Title>
							{#if description}
								<Dialog.Description>
									<span class="description">{description}</span>
								</Dialog.Description>
							{/if}
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
				</div>
			{/snippet}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.scrim {
		position: fixed;
		inset: 0;
		z-index: var(--z-dialog);
		background: var(--scrim);
	}

	.dialog {
		--dialog-width: var(--dialog-md);
		position: fixed;
		inset-block-start: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		z-index: var(--z-dialog);
		display: grid;
		grid-template-rows: auto minmax(0, 1fr) auto;
		inline-size: min(calc(100% - var(--space-8)), var(--dialog-width));
		max-block-size: calc(100% - var(--space-8));
		background: var(--dialog-bg, var(--surface-overlay));
		color: var(--text);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--dialog-radius, var(--radius-xl));
		box-shadow: var(--dialog-shadow, var(--shadow-xl));
		animation: dialog-in var(--dur-large) var(--ease-out);
	}

	.dialog[data-size='sm'] {
		--dialog-width: var(--dialog-sm);
	}

	.dialog[data-size='lg'] {
		--dialog-width: var(--dialog-lg);
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-2);
		padding: var(--space-6) var(--space-6) 0;
	}

	.titles {
		display: grid;
		gap: var(--space-1-5);
		min-inline-size: 0;
	}

	.title {
		display: block;
		font-size: var(--text-title-sm-size);
		font-weight: var(--text-title-sm-weight);
		line-height: var(--text-title-sm-line);
		letter-spacing: var(--text-title-sm-tracking);
	}

	.description {
		display: block;
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.body {
		padding: var(--space-6);
	}

	.foot {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: var(--space-2);
		padding: var(--space-4) var(--space-6);
		border-block-start: var(--hairline) solid var(--border-subtle);
		border-end-start-radius: inherit;
		border-end-end-radius: inherit;
		background: var(--dialog-foot-bg, var(--bg-subtle));
	}

	@keyframes dialog-in {
		from {
			opacity: 0;
			transform: scale(0.97);
		}
	}

	@keyframes dialog-fade {
		from {
			opacity: 0;
		}
	}

	@media (max-width: 639.98px) {
		.dialog {
			inset: 0;
			translate: none;
			inline-size: 100%;
			block-size: 100%;
			max-block-size: none;
			border: 0;
			border-radius: 0;
			box-shadow: none;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dialog {
			animation-name: dialog-fade;
		}
	}

	:global(html[data-motion='reduced']) .dialog {
		animation-name: dialog-fade;
	}

	:global(html[data-motion='none']) .dialog {
		animation: none;
	}
</style>
