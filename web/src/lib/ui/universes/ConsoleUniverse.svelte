<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import '$lib/ui/quarks/tokens/index.css';
	import '@fontsource-variable/inter';
	import '@fontsource-variable/jetbrains-mono';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/copy/t';
	import AuthGalaxy from '$lib/ui/galaxies/AuthGalaxy.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import Spinner from '$lib/ui/quarks/Spinner.svelte';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { ThemeController } from '$lib/theme/controller.svelte';

	interface Props {
		children: Snippet;
	}

	const LOGIN_PATH = '/login';
	const REDIRECT_PARAM = 'redirect';

	let { children }: Props = $props();

	const appPath = $derived(stripBase(page.url.pathname));
	const onLogin = $derived(LOGIN_PATH === appPath);
	const signedIn = $derived(null !== session.me);
	const ready = $derived(true === session.restored && (false === signedIn || 'success' === capabilities.status));
	const bootFailed = $derived(true === session.restored && true === signedIn && null !== capabilities.error && 'error' === capabilities.status);

	function stripBase(pathname: string): string {
		if (0 < base.length && pathname.startsWith(base)) {
			return pathname.slice(base.length) || '/';
		}
		return pathname;
	}

	function safeTarget(value: string | null): string {
		if (null === value || false === value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) {
			return '/';
		}
		return value;
	}

	$effect(() => {
		document.documentElement.lang = 'en';
		document.documentElement.dir = 'ltr';
	});

	$effect(() => {
		const themeController = new ThemeController();
		return () => themeController.dispose();
	});

	$effect(() => {
		void session.restore();
	});

	$effect(() => {
		if (false === signedIn) {
			return;
		}
		void capabilities.load();
	});

	let preferencesUser: string | null = null;

	$effect(() => {
		const userId = session.me?.user_id;
		if (undefined === userId || userId === preferencesUser) {
			return;
		}
		preferencesUser = userId;
		untrack(() => {
			void preferences.load(userId);
		});
	});

	$effect(() => {
		if (false === session.restored) {
			return;
		}
		if (false === signedIn && false === onLogin) {
			const wanted = `${appPath}${page.url.search}`;
			const query = '/' === wanted ? '' : `?${REDIRECT_PARAM}=${encodeURIComponent(wanted)}`;
			void goto(`${base}${LOGIN_PATH}${query}`, { replaceState: true });
			return;
		}
		if (true === signedIn && true === onLogin) {
			void goto(`${base}${safeTarget(page.url.searchParams.get(REDIRECT_PARAM))}`, { replaceState: true });
		}
	});
</script>

{#if bootFailed && null !== capabilities.error}
	<div class="boot">
		<span class="mark" aria-hidden="true">c</span>
		<ErrorState error={capabilities.error} variant="region" onretry={() => void capabilities.load()} />
	</div>
{:else if false === ready || (true === signedIn && true === onLogin)}
	<div class="boot" aria-busy="true">
		<span class="mark" aria-hidden="true">c</span>
		<Spinner label={t('shell.boot.loading')} />
	</div>
{:else if false === signedIn}
	{#if onLogin}
		<AuthGalaxy expired={session.expired} />
	{:else}
		<div class="boot" aria-busy="true">
			<Spinner label={t('shell.boot.loading')} />
		</div>
	{/if}
{:else}
	{@render children()}
{/if}

<style>
	.boot {
		display: grid;
		place-content: center;
		justify-items: center;
		gap: var(--space-4);
		min-block-size: 100dvh;
		background: var(--bg);
		color: var(--text-muted);
	}

	.mark {
		display: grid;
		place-items: center;
		inline-size: var(--space-10);
		block-size: var(--space-10);
		border-radius: var(--radius-lg);
		background: var(--accent);
		color: var(--text-on-accent);
		font-size: var(--text-title-size);
		font-weight: var(--text-title-weight);
		line-height: 1;
	}
</style>
