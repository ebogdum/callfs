<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/copy/t';
	import { navItems, type ClusterSubgroup, type NavChild } from '$lib/nav/model';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { session } from '$lib/stores/session.svelte';
	import SectionRail from '../environments/SectionRail.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import type { IconName } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	let { children, class: className, ...rest }: Props = $props();

	const ICONS: Readonly<Record<string, IconName>> = {
		'cluster.topology': 'network',
		'cluster.members': 'users',
		'cluster.replication': 'leader',
		'cluster.servers': 'server',
		'cluster.storage': 'hard-drive',
		'cluster.durability': 'shield',
		'cluster.backup': 'backup',
		'cluster.hygiene': 'fix',
		'cluster.locks': 'lock',
		'cluster.cache': 'cache',
		'cluster.limits': 'limit'
	};
	const SUBGROUPS: ClusterSubgroup[] = ['Members', 'Data', 'Runtime'];
	const clusterChildren: readonly NavChild[] = navItems.find((item) => 'cluster' === item.id)?.children ?? [];

	$effect(() => {
		void capabilities.load();
	});

	const enabledFeatures = $derived(
		null === capabilities.data
			? []
			: Object.entries(capabilities.data.features)
					.filter(([, on]) => true === on)
					.map(([name]) => name)
	);

	function isEnabled(child: NavChild): boolean {
		return null === child.capability || enabledFeatures.includes(child.capability);
	}

	const groups = $derived(
		SUBGROUPS.map((subgroup) => ({
			label: t(`systems-b.cluster.group.${subgroup.toLowerCase()}`),
			items: clusterChildren
				.filter((child) => subgroup === child.subgroup)
				.map((child) => ({
					id: child.id,
					label: t(`systems-b.${child.id}`),
					href: `${base}${child.href}`,
					icon: ICONS[child.id] ?? 'network',
					badge: isEnabled(child) ? undefined : t('shell.cluster.off')
				}))
		})).filter((group) => 0 < group.items.length)
	);

	const appPath = $derived(page.url.pathname.startsWith(base) ? page.url.pathname.slice(base.length) || '/' : page.url.pathname);
	const current = $derived(
		clusterChildren
			.filter((child) => appPath === child.href || appPath.startsWith(`${child.href}/`))
			.sort((a, b) => b.href.length - a.href.length)[0]?.id ?? ''
	);
</script>

<div class={['cluster-system', className]} {...rest}>
	{#if false === session.isAdmin}
		<EmptyState
			variant="no-permission"
			icon="network"
			title={t('systems-b.cluster.admin_only.title')}
			body={t('systems-b.cluster.admin_only.body')}
		/>
	{:else}
		<div class="body">
			<SectionRail {groups} {current} label={t('systems-b.cluster.nav.label')} />
			<div class="content">
				{@render children()}
			</div>
		</div>
	{/if}
</div>

<style>
	.cluster-system {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.body {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.content {
		min-inline-size: 0;
	}

	@media (min-width: 840px) {
		.body {
			grid-template-columns: calc(var(--space-4) * 14) minmax(0, 1fr);
			align-items: start;
			gap: var(--space-10);
		}
	}
</style>
