<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Link from '../atoms/Link.svelte';
	import Text from '../atoms/Text.svelte';
	import PageFrame from '../environments/PageFrame.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props {
		class?: string;
	}

	const TOP_LINKS: readonly { key: string; href: string }[] = [
		{ key: 'overview', href: '/' },
		{ key: 'files', href: '/files' },
		{ key: 'sharing', href: '/sharing' }
	];

	const { class: className }: Props = $props();

	function goToOverview(): void {
		void goto(`${base}/`);
	}

	function goToSearch(): void {
		void goto(`${base}/files/search`);
	}
</script>

<PageFrame title={t('about.notfound.title')} class={className}>
	{#snippet children()}
		<Stack gap="6">
			<Text tone="muted">{t('about.notfound.body')}</Text>
			<Cluster gap="2">
				<Button variant="primary" onclick={goToOverview}>{t('about.notfound.overview')}</Button>
				<Button variant="secondary" onclick={goToSearch}>{t('about.notfound.search')}</Button>
			</Cluster>
			<nav aria-label={t('about.notfound.links')}>
				<ul class="top-links">
					{#each TOP_LINKS as link (link.href)}
						<li>
							<Link href={`${base}${link.href}`}>{t(`about.notfound.link.${link.key}`)}</Link>
						</li>
					{/each}
				</ul>
			</nav>
		</Stack>
	{/snippet}
</PageFrame>

<style>
	.top-links {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}
</style>
