<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { getConfig } from '$lib/api/admin/config';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { session } from '$lib/stores/session.svelte';
	import SettingsLayout from '$lib/ui/environments/SettingsLayout.svelte';
	import Banner from '$lib/ui/molecules/Banner.svelte';
	import type { IconName } from '$lib/ui/types';

	interface SettingsSection {
		slug: string;
		labelKey: string;
		icon: IconName;
		adminOnly: boolean;
	}

	interface SettingsGroup {
		labelKey: string;
		sections: readonly SettingsSection[];
	}

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	const CONFIG_STALE_MS = 60_000;
	const INDEX_ROUTE = '/(console)/settings';
	const GROUP_ROUTE = '/(console)/settings/[group]';

	const groups: readonly SettingsGroup[] = [
		{
			labelKey: 'systems-d.settings.group.server',
			sections: [
				{ slug: 'network', labelKey: 'systems-d.settings.nav.network', icon: 'network', adminOnly: true },
				{ slug: 'limits', labelKey: 'systems-d.settings.nav.limits', icon: 'limit', adminOnly: true },
				{ slug: 'security', labelKey: 'systems-d.settings.nav.security', icon: 'shield', adminOnly: true }
			]
		},
		{
			labelKey: 'systems-d.settings.group.data',
			sections: [
				{ slug: 'storage', labelKey: 'systems-d.settings.nav.storage', icon: 'database', adminOnly: true },
				{ slug: 'index', labelKey: 'systems-d.settings.nav.index', icon: 'layers', adminOnly: true },
				{ slug: 'coordination', labelKey: 'systems-d.settings.nav.coordination', icon: 'lock', adminOnly: true }
			]
		},
		{
			labelKey: 'systems-d.settings.group.access',
			sections: [{ slug: 'access', labelKey: 'systems-d.settings.nav.users', icon: 'key', adminOnly: true }]
		},
		{
			labelKey: 'systems-d.settings.group.system',
			sections: [
				{ slug: 'logging', labelKey: 'systems-d.settings.nav.logging', icon: 'logs', adminOnly: true },
				{ slug: 'file', labelKey: 'systems-d.settings.nav.file', icon: 'code', adminOnly: true },
				{ slug: 'about', labelKey: 'systems-d.settings.nav.about', icon: 'info', adminOnly: false }
			]
		},
		{
			labelKey: 'systems-d.settings.group.personal',
			sections: [
				{ slug: 'appearance', labelKey: 'systems-d.settings.nav.appearance', icon: 'palette', adminOnly: false },
				{ slug: 'shortcuts', labelKey: 'systems-d.settings.nav.shortcuts', icon: 'keyboard', adminOnly: false }
			]
		}
	];

	let { children, class: className, ...rest }: Props = $props();

	let search = $state('');

	const configQuery = createQuery(() => ({
		key: ['config'],
		fetch: () => getConfig(),
		enabled: () => session.isAdmin,
		staleMs: CONFIG_STALE_MS,
		revalidateOnFocus: true
	}));

	const pendingCount = $derived(true === configQuery.data?.ok ? configQuery.data.value.pending.length : 0);

	const restartMessage = $derived(
		1 === pendingCount
			? t('systems-d.settings.restart.one')
			: t('systems-d.settings.restart.many', { count: pendingCount })
	);

	const navGroups = $derived(
		groups
			.map((group) => ({
				label: t(group.labelKey),
				items: group.sections
					.filter((section) => session.isAdmin || !section.adminOnly)
					.map((section) => ({
						id: section.slug,
						label: t(section.labelKey),
						href: `${base}/settings/${section.slug}`,
						icon: section.icon
					}))
			}))
			.filter((group) => 0 < group.items.length)
	);

	const current = $derived(page.url.pathname.split('/').at(-1) ?? '');
	const isIndex = $derived(INDEX_ROUTE === page.route.id);
	const showsOwnRestart = $derived(GROUP_ROUTE === page.route.id);

	$effect(() => {
		if (false === isIndex) {
			return;
		}
		void goto(`${base}/settings/${session.isAdmin ? 'network' : 'appearance'}`, { replaceState: true });
	});

	const reviewChanges = (): void => {
		void goto(`${base}/settings/file`);
	};
</script>

<div class={['settings-system', className]} {...rest}>
	<SettingsLayout nav={navGroups} {current} bind:search>
		{#if 0 < pendingCount && !showsOwnRestart}
			<Banner
				tone="warn"
				title={restartMessage}
				action={{ label: t('systems-d.settings.restart.review'), run: reviewChanges }}
			/>
		{/if}
		{#if false === isIndex}
			{@render children()}
		{/if}
	</SettingsLayout>
</div>

<style>
	.settings-system {
		display: contents;
	}
</style>
