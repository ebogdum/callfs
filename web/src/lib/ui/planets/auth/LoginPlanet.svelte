<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { recentRequestRecords } from '$lib/api/http';
	import type { Me, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { UiError } from '$lib/ui/types';
	import type { HTMLAttributes } from 'svelte/elements';
	import Button from '../../atoms/Button.svelte';
	import FieldError from '../../atoms/FieldError.svelte';
	import Input from '../../atoms/Input.svelte';
	import Label from '../../atoms/Label.svelte';
	import HelpText from '../../atoms/HelpText.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import HelpPanel from '../../organisms/HelpPanel.svelte';
	import Icon from '../../quarks/Icon.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		onsignedin?: (me: Me) => void;
	}

	const HELP_TOPIC = 'access-key';
	const HTTP_UNAUTHORIZED = 401;
	const HTTP_TOO_MANY_REQUESTS = 429;

	let { onsignedin, class: className, ...rest }: Props = $props();

	let key = $state('');
	let pending = $state(false);
	let failure = $state<UiError | null>(null);
	let helpOpen = $state(false);
	let helpTopic = $state(HELP_TOPIC);
	let revealed = $state(false);

	const rejectedKey = $derived(null !== failure && HTTP_UNAUTHORIZED === failure.status);
	const alertFailure = $derived(null !== failure && false === rejectedKey);
	const retryable = $derived(null !== failure && failure.retryable);

	const alertTitle = $derived.by(() => {
		if (null === failure) {
			return '';
		}
		if (HTTP_TOO_MANY_REQUESTS === failure.status) {
			return t('login.error.rate_limited');
		}
		if ('network' === failure.layer) {
			return t('login.error.unreachable');
		}
		return failure.title;
	});

	const alertBody = $derived.by(() => {
		if (null === failure || HTTP_TOO_MANY_REQUESTS === failure.status || 'network' === failure.layer) {
			return undefined;
		}
		return failure.body;
	});

	function destinationFor(redirect: string | null): string {
		if (null === redirect || false === redirect.startsWith('/') || redirect.startsWith('//') || redirect.startsWith('/\\')) {
			return `${base}/`;
		}
		return `${base}${redirect}`;
	}

	async function establishSession(): Promise<Result<Me, UiError>> {
		const signedIn = await session.signIn(key);
		if (false === signedIn.ok) {
			return signedIn;
		}
		const refreshed = await session.refresh();
		if (false === refreshed.ok) {
			return refreshed;
		}
		capabilities.reset();
		const loaded = await capabilities.load();
		if (false === loaded.ok) {
			return loaded;
		}
		return refreshed;
	}

	async function signIn(): Promise<void> {
		if (true === pending) {
			return;
		}
		pending = true;
		failure = null;
		const outcome = await establishSession();
		pending = false;
		if (false === outcome.ok) {
			failure = outcome.error;
			return;
		}
		key = '';
		if (undefined !== onsignedin) {
			onsignedin(outcome.value);
			return;
		}
		await goto(destinationFor(page.url.searchParams.get('redirect')));
	}

	function submit(event: SubmitEvent): void {
		event.preventDefault();
		void signIn();
	}

	function toggleReveal(): void {
		revealed = !revealed;
	}

	function openHelp(): void {
		helpOpen = true;
	}

	function selectHelpTopic(topic: string): void {
		helpTopic = topic;
	}
</script>

<svelte:head>
	<title>{t('login.document_title')}</title>
</svelte:head>

<div class={['login-planet', className]} {...rest}>
	<header class="intro">
		<h1 class="title">{t('login.title')}</h1>
		<p class="lead">{t('shell.login.lead')}</p>
	</header>
	<form class="login-form" onsubmit={submit} novalidate>
		<div class="field">
			<div class="label-row">
				<Label for="login-key">{t('login.key.label')}</Label>
				<button type="button" class="where" onclick={openHelp}>{t('login.key.where')}</button>
			</div>
			<Input
				id="login-key"
				name="access-key"
				type={revealed ? 'text' : 'password'}
				size="lg"
				mono={revealed}
				bind:value={key}
				invalid={rejectedKey}
				autocomplete="current-password"
				spellcheck={false}
				required
				aria-describedby={rejectedKey ? 'login-key-error' : 'login-key-help'}
			>
				{#snippet prefix()}
					<span class="adorn"><Icon name="key" size="md" /></span>
				{/snippet}
				{#snippet suffix()}
					<button
						type="button"
						class="reveal"
						aria-pressed={revealed}
						aria-label={revealed ? t('shell.login.hide_key') : t('shell.login.show_key')}
						onclick={toggleReveal}
					>
						<Icon name={revealed ? 'eye-off' : 'eye'} size="md" />
					</button>
				{/snippet}
			</Input>
			<div aria-live="polite">
				{#if rejectedKey}
					<FieldError id="login-key-error" message={t('login.error.invalid_key')} />
				{:else}
					<HelpText id="login-key-help">{t('login.key.help')}</HelpText>
				{/if}
			</div>
		</div>
		{#if alertFailure}
			<InlineAlert tone="danger" title={alertTitle} body={alertBody}>
				{#snippet actions()}
					{#if retryable}
						<Button size="sm" onclick={signIn}>{t('common.retry')}</Button>
					{/if}
				{/snippet}
			</InlineAlert>
		{/if}
		<Button type="submit" variant="primary" size="lg" full loading={pending}>
			{t('login.submit')}
		</Button>
	</form>
	<HelpPanel bind:open={helpOpen} topic={helpTopic} ontopic={selectHelpTopic} diagnostics={JSON.stringify(recentRequestRecords())} />
</div>

<style>
	.login-planet {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.intro {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
	}

	.title {
		margin: 0;
		font-size: var(--text-title-size);
		line-height: var(--text-title-line);
		font-weight: var(--text-title-weight);
		letter-spacing: var(--text-title-tracking);
		color: var(--text);
	}

	.lead {
		margin: 0;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		color: var(--text-muted);
	}

	.login-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.label-row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-3);
	}

	.where {
		padding: 0;
		border: 0;
		border-radius: var(--radius-xs);
		background: none;
		color: var(--text-link);
		font: inherit;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		cursor: pointer;
	}

	@media (hover: hover) {
		.where:hover {
			text-decoration: underline;
		}
	}

	.adorn {
		display: inline-flex;
		align-items: center;
		padding-inline-start: var(--space-3);
		color: var(--text-faint);
	}

	.reveal {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		inline-size: var(--control-h);
		border: 0;
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}

	@media (hover: hover) {
		.reveal:hover {
			color: var(--text);
			background: var(--surface-hover);
		}
	}
</style>
