<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { getContext, untrack, type Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { session } from '$lib/stores/session.svelte';
	import Tabs from '../molecules/Tabs.svelte';
	import { pathFromRoute } from '../planets/files/browse-state.svelte';
	import { filesStore } from './files-store.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class' | 'children'> {
		class?: string;
		children: Snippet;
	}

	let { class: className = '', children, ...rest }: Props = $props();

	const COMMANDS_CONTEXT = 'callfs.commands';
	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	const sections = [
		{ id: 'browse', labelKey: 'systems-a.files.tab.browse', href: '/files', routeId: '/(console)/files/[...path]', adminOnly: false },
		{ id: 'search', labelKey: 'systems-a.files.tab.search', href: '/files/search', routeId: '/(console)/files/search', adminOnly: false },
		{ id: 'flat', labelKey: 'systems-a.files.tab.flat', href: '/files/flat', routeId: '/(console)/files/flat', adminOnly: true },
		{ id: 'transfers', labelKey: 'systems-a.files.tab.transfers', href: '/files/transfers', routeId: '/(console)/files/transfers', adminOnly: false },
		{ id: 'trash', labelKey: 'systems-a.files.tab.trash', href: '/files/trash', routeId: '/(console)/files/trash', adminOnly: false }
	] as const;

	const tabs = $derived(
		sections
			.filter((section) => !section.adminOnly || session.isAdmin)
			.map((section) => ({ id: section.id, label: t(section.labelKey), href: `${base}${section.href}` }))
	);

	const activeTab = $derived(sections.find((section) => section.routeId === page.route.id)?.id ?? 'browse');

	const browsePath = $derived(undefined === page.params.path ? null : pathFromRoute(page.params.path));

	$effect(() => {
		if (null === browsePath || '/' === browsePath) {
			return;
		}
		const path = browsePath;
		untrack(() => filesStore.remember(path));
	});

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'files.go',
				title: t('systems-a.files.go'),
				group: 'Files',
				keywords: [t('systems-a.files.go')],
				shortcut: ['g', 'f'],
				icon: 'folder',
				run: () => goto(`${base}/files`)
			},
			{
				id: 'files.go.transfers',
				title: t('systems-a.files.go_transfers'),
				group: 'Files',
				keywords: [t('systems-a.files.go_transfers')],
				shortcut: ['g', 't'],
				icon: 'activity',
				run: () => goto(`${base}/files/transfers`)
			}
		]);
	});
</script>

<div class="files-system {className}" {...rest}>
	<Tabs label={t('systems-a.files.nav_label')} items={tabs} value={activeTab} />
	{@render children()}
</div>

<style>
	.files-system {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}
</style>
