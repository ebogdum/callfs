<script lang="ts" module>
	export type AlertDialogTone = 'danger' | 'warn';
	export const TOKENS = ['--alert-dialog-bg', '--alert-dialog-radius', '--alert-dialog-shadow'] as const;
</script>

<script lang="ts">
	import { AlertDialog } from 'bits-ui';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
		open: boolean;
		title: string;
		body: string;
		confirmLabel: string;
		tone?: AlertDialogTone;
		typedWord?: string;
		onconfirm: () => void;
	}

	let {
		open = $bindable(false),
		title,
		body,
		confirmLabel,
		tone = 'danger',
		typedWord,
		onconfirm,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const typedInputId = `${uid}-typed`;
	const typedErrorId = `${uid}-typed-error`;

	let content = $state<HTMLDivElement | null>(null);
	let typed = $state('');
	let attempted = $state(false);

	const mismatch = $derived(attempted && typedWord !== typed.trim());

	$effect(() => {
		if (!open) {
			typed = '';
			attempted = false;
		}
	});

	function focusCancel(event: Event) {
		event.preventDefault();
		content?.querySelector<HTMLElement>('.actions button')?.focus();
	}

	function confirm() {
		if (undefined !== typedWord && typedWord !== typed.trim()) {
			attempted = true;
			return;
		}
		onconfirm();
		open = false;
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Portal>
		<AlertDialog.Overlay>
			{#snippet child({ props })}
				<div {...props} class="scrim"></div>
			{/snippet}
		</AlertDialog.Overlay>
		<AlertDialog.Content
			bind:ref={content}
			escapeKeydownBehavior="close"
			interactOutsideBehavior="ignore"
			onOpenAutoFocus={focusCancel}
		>
			{#snippet child({ props })}
				<div {...rest} {...props} class={['alert', className]}>
					<div class="copy">
						<AlertDialog.Title>
							<span class="title">{title}</span>
						</AlertDialog.Title>
						<AlertDialog.Description>
							<span class="body">{body}</span>
						</AlertDialog.Description>
					</div>
					{#if undefined !== typedWord}
						<div class="typed">
							<Label for={typedInputId}>{t('u092overlays.type_word', { word: typedWord })}</Label>
							<Input
								id={typedInputId}
								bind:value={typed}
								autocomplete="off"
								invalid={mismatch}
								aria-describedby={mismatch ? typedErrorId : undefined}
							/>
							{#if mismatch}
								<FieldError id={typedErrorId} message={t('u092overlays.type_mismatch', { word: typedWord })} />
							{/if}
						</div>
					{/if}
					<div class="actions">
						<Button
							variant="secondary"
							onclick={() => {
								open = false;
							}}
						>
							{t('u092overlays.cancel')}
						</Button>
						<Button variant={'danger' === tone ? 'danger' : 'primary'} onclick={confirm}>
							{confirmLabel}
						</Button>
					</div>
				</div>
			{/snippet}
		</AlertDialog.Content>
	</AlertDialog.Portal>
</AlertDialog.Root>

<style>
	.scrim {
		position: fixed;
		inset: 0;
		z-index: var(--z-dialog);
		background: var(--scrim);
	}

	.alert {
		position: fixed;
		inset-block-start: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		z-index: var(--z-dialog);
		display: grid;
		gap: var(--space-4);
		inline-size: min(calc(100% - var(--space-8)), var(--dialog-sm));
		padding: var(--space-6);
		background: var(--alert-dialog-bg, var(--surface-overlay));
		color: var(--text);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--alert-dialog-radius, var(--radius-xl));
		box-shadow: var(--alert-dialog-shadow, var(--shadow-xl));
		animation: alert-in var(--dur-large) var(--ease-out);
	}

	.copy {
		display: grid;
		gap: var(--space-2);
	}

	.title {
		display: block;
		font-size: var(--text-title-sm-size);
		font-weight: var(--text-title-sm-weight);
		line-height: var(--text-title-sm-line);
		letter-spacing: var(--text-title-sm-tracking);
	}

	.body {
		display: block;
		color: var(--text-muted);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.typed {
		display: grid;
		gap: var(--space-1-5);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: var(--space-2);
	}

	@keyframes alert-in {
		from {
			opacity: 0;
			transform: scale(0.97);
		}
	}

	@keyframes alert-fade {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.alert {
			animation-name: alert-fade;
		}
	}

	:global(html[data-motion='reduced']) .alert {
		animation-name: alert-fade;
	}

	:global(html[data-motion='none']) .alert {
		animation: none;
	}
</style>
