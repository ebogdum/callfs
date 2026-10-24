<script lang="ts" module>
	export const TOKENS = ['--session-dialog-bg', '--session-dialog-border', '--session-dialog-radius', '--session-dialog-pad'] as const;

	export type SessionExpiryReason = 'idle' | 'expiry' | 'expired';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { AlertDialog } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Duration from '../atoms/Duration.svelte';
	import Text from '../atoms/Text.svelte';
	import SecretInput from '../molecules/SecretInput.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		open: boolean;
		reason: SessionExpiryReason;
		secondsLeft: number;
		onstay: () => void;
		onsignout: () => void;
		onreauth: (key: string) => void;
		class?: string;
	}

	let { open, reason, secondsLeft, onstay, onsignout, onreauth, class: className, ...rest }: Props = $props();

	let key = $state('');
	const trimmedKey = $derived(key.trim());
	const title = $derived(
		'expired' === reason
			? t('support.session.expired_title')
			: 'expiry' === reason
				? t('support.session.expiry_title')
				: t('support.session.idle_title')
	);
	const body = $derived(
		'expired' === reason
			? t('support.session.expired_body')
			: 'expiry' === reason
				? t('support.session.expiry_body')
				: t('support.session.idle_body')
	);
	const countdownMs = $derived(Math.max(0, secondsLeft) * 1000);

	function submitKey(): void {
		onreauth(trimmedKey);
	}
</script>

<AlertDialog.Root {open}>
	<AlertDialog.Portal>
		<AlertDialog.Overlay class="session-scrim" />
		<AlertDialog.Content class={className ? `session-dialog ${className}` : 'session-dialog'}>
			<div {...rest}>
			<Stack gap="4">
				<AlertDialog.Title>
					<Text size="title-sm">{title}</Text>
				</AlertDialog.Title>
				<AlertDialog.Description>
					<Text tone="muted">{body}</Text>
				</AlertDialog.Description>
				{#if 'expired' === reason}
					<SecretInput
						label={t('support.session.key_label')}
						isSet={false}
						generate={false}
						autocomplete="current-password"
						bind:value={key}
					/>
					<Cluster justify="end" align="center" wrap gap="2">
						<Button variant="secondary" onclick={onsignout}>{t('support.session.sign_out')}</Button>
						<Button
							variant="primary"
							disabledReason={0 === trimmedKey.length ? t('support.session.key_required') : undefined}
							onclick={submitKey}>{t('support.session.sign_in')}</Button
						>
					</Cluster>
				{:else}
					<Cluster gap="2" align="center" wrap>
						<Text>{t('support.session.countdown')}</Text>
						<Duration value={countdownMs} />
					</Cluster>
					<Cluster justify="end" align="center" wrap gap="2">
						<Button variant="primary" onclick={onstay}>{t('support.session.stay')}</Button>
						<Button variant="secondary" onclick={onsignout}>{t('support.session.sign_out')}</Button>
					</Cluster>
				{/if}
			</Stack>
			</div>
		</AlertDialog.Content>
	</AlertDialog.Portal>
</AlertDialog.Root>

<style>
	:global(.session-scrim) {
		position: fixed;
		inset: 0;
		background: var(--scrim);
		z-index: var(--z-dialog);
	}

	:global(.session-dialog) {
		position: fixed;
		inset-block-start: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		inline-size: min(var(--dialog-sm), calc(100% - var(--space-8)));
		max-block-size: calc(100% - var(--space-8));
		overflow: auto;
		padding: var(--session-dialog-pad, var(--space-6));
		z-index: var(--z-dialog);
		background: var(--session-dialog-bg, var(--surface-overlay));
		color: var(--text);
		border: var(--hairline) solid var(--session-dialog-border, var(--border-subtle));
		border-radius: var(--session-dialog-radius, var(--radius-xl));
		box-shadow: var(--shadow-xl);
	}

	:global(.session-dialog:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}
</style>
