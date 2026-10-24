<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/copy/t';
	import { visibleNavItems, type Role } from '$lib/nav/model';
	import { session } from '$lib/stores/session.svelte';
	import type { IconName } from '$lib/ui/types';
	import Tabs from '../molecules/Tabs.svelte';
	import { provideTimeRange } from '$lib/stores/time-range.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		children: Snippet;
	}

	const SECTION_ID = 'insights';
	const SECTION_LABEL_KEY = 'nav.insights';
	const ICONS: Readonly<Record<string, IconName>> = {
		'insights.dashboards': 'bar-chart',
		'insights.explore': 'circle-dot',
		'insights.storage': 'hard-drive',
		'insights.metrics': 'file-code'
	};

	let { children: content, class: className, ...rest }: Props = $props();

	provideTimeRange();

	const role: Role = $derived(session.isAdmin ? 'admin' : 'member');
	const section = $derived(visibleNavItems(role, []).find((item) => SECTION_ID === item.id));
	const groups = $derived(
		undefined === section
			? []
			: [
					{
						label: t(SECTION_LABEL_KEY),
						items: section.children.map((child) => ({
							id: child.id,
							label: t(child.labelKey),
							href: `${base}${child.href}`,
							icon: ICONS[child.id]
						}))
					}
				]
	);
	const appPath = $derived(page.url.pathname.startsWith(base) ? page.url.pathname.slice(base.length) || '/' : page.url.pathname);
	const tabs = $derived(groups.flatMap((group) => group.items));
	const current = $derived(
		(section?.children ?? [])
			.filter((child) => appPath === child.href || appPath.startsWith(`${child.href}/`))
			.sort((a, b) => b.href.length - a.href.length)[0]?.id ?? ''
	);
</script>

<div class={['insights-system', className]} {...rest}>
	<Tabs label={t(SECTION_LABEL_KEY)} items={tabs} value={current} />
	{@render content()}
</div>

<style>
	.insights-system {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}
</style>
