<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { cluster as loadCluster } from '$lib/api/admin/cluster';
	import { getConfig } from '$lib/api/admin/config';
	import { findings as loadFindings } from '$lib/api/admin/findings';
	import { health as loadHealth } from '$lib/api/admin/health';
	import { recentRequestRecords } from '$lib/api/http';
	import { roleOf } from '$lib/api/nodes';
	import { commands, parseCommandQuery, setCommandContext, type Command, type SearchHit } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import type { GlossaryKey } from '$lib/copy/glossary';
	import { applyStatusSignals, liveHub as hub } from '$lib/live/hub.svelte';
	import { navGroups, visibleNavItems, type NavGroup, type NavItem } from '$lib/nav/model';
	import type { ShortcutScope } from '$lib/shortcuts/keymap';
	import { ShortcutManager } from '$lib/shortcuts/manager.svelte';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { jobs } from '$lib/stores/jobs.svelte';
	import { nodeScope } from '$lib/stores/node-scope.svelte';
	import { controlNode, nodes } from '$lib/stores/nodes.svelte';
	import { notifications } from '$lib/stores/notifications.svelte';
	import { preferences, type ThemeMode } from '$lib/stores/preferences.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import { transfers } from '$lib/stores/transfers.svelte';
	import type { MenuItem } from '$lib/ui/types';
	import type { HTMLAttributes } from 'svelte/elements';
	import { untrack, type Snippet } from 'svelte';
	import BottomNav, { type BottomNavItem } from '../environments/BottomNav.svelte';
	import AppSidebar, { type SidebarGroup } from '../environments/AppSidebar.svelte';
	import Dock from '../environments/Dock.svelte';
	import OverlayHost from '../environments/OverlayHost.svelte';
	import TopBar from '../environments/TopBar.svelte';
	import Banner from '../molecules/Banner.svelte';
	import ControlStrip from '../molecules/ControlStrip.svelte';
	import ServerPicker, { type ServerChoice } from '../molecules/ServerPicker.svelte';
	import Menu from '../molecules/Menu.svelte';
	import Avatar from '../atoms/Avatar.svelte';
	import Badge from '../atoms/Badge.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import IconButton from '../atoms/IconButton.svelte';
	import CommandPalette from '../organisms/CommandPalette.svelte';
	import Drawer from '../organisms/Drawer.svelte';
	import HelpPanel from '../organisms/HelpPanel.svelte';
	import NotificationCenter from '../organisms/NotificationCenter.svelte';
	import SessionExpiryDialog from '../organisms/SessionExpiryDialog.svelte';
	import ShortcutSheet, { type KeymapSection } from '../organisms/ShortcutSheet.svelte';
	import Toaster from '../organisms/Toaster.svelte';
	import TransferTray from '../organisms/TransferTray.svelte';
	import Icon from '../quarks/Icon.svelte';
	import { BREAKPOINTS } from '../quarks/tokens/breakpoints';
	import LiveRegions from './LiveRegions.svelte';
	import {
		buildDiagnostics,
		createGlobalCommands,
		createNodeCommands,
		recentRequestIds,
		selectBanner,
		type BannerKind
	} from './global-commands';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		children: Snippet;
		bulkBar?: Snippet;
		class?: string;
	}

	const POLL_INTERVAL_MS = 10_000;
	const NODE_POLL_MS = 30_000;
	const SESSION_TICK_MS = 1_000;
	const PRIMARY_NAV_COUNT = 4;
	const SCOPE_ORDER: readonly ShortcutScope[] = ['global', 'lists', 'data', 'tables', 'files', 'logs', 'chart', 'settings'];
	const ADMIN_TOPICS: readonly string[] = [
		'health.changed',
		'cluster.*',
		'finding.*',
		'config.*',
		'job.*',
		'server.*'
	];
	const LIVE_TONES = {
		live: 'ok',
		reconnecting: 'warn',
		offline: 'danger',
		paused: 'muted'
	} as const;
	const GROUP_LABEL_KEYS: Record<NavGroup, string> = {
		Work: 'galaxy.group.work',
		Operate: 'galaxy.group.operate',
		Configure: 'galaxy.group.configure'
	};

	let { children, bulkBar, class: className, ...rest }: Props = $props();

	const shortcuts = new ShortcutManager();

	let noLeader = $state(false);
	let restartRequired = $state(false);
	let criticalFindings = $state(0);
	let warningFindings = $state(0);
	let narrow = $state(false);
	let crowded = $state(false);
	let paletteOpen = $state(false);
	let sheetOpen = $state(false);
	let helpOpen = $state(false);
	let helpTopic = $state<GlossaryKey | undefined>(undefined);
	let notificationsOpen = $state(false);
	let trayCollapsed = $state(false);

	const isAdmin = $derived(session.isAdmin);
	const role = $derived<'admin' | 'member'>(isAdmin ? 'admin' : 'member');
	const pathname = $derived(stripBase(page.url.pathname));
	const capabilityNames = $derived(
		null !== capabilities.data
			? Object.entries(capabilities.data.features)
					.filter(([, enabled]) => true === enabled)
					.map(([name]) => name)
			: []
	);
	const visibleItems = $derived(visibleNavItems(role, capabilityNames));
	const currentItem = $derived(visibleItems.find((item) => sectionOf(item.href) === sectionOf(pathname)));
	const currentId = $derived(currentItem?.id ?? '');
	const currentLabel = $derived(undefined === currentItem ? '' : t(`galaxy.nav.${currentItem.id}`));
	const currentGroup = $derived(undefined === currentItem ? '' : t(GROUP_LABEL_KEYS[currentItem.group]));
	const liveState = $derived(hub.connection.indicator);
	const liveTone = $derived(LIVE_TONES[liveState]);
	const serverName = $derived(nodeScope.selected ?? capabilities.data?.instance_id ?? '');
	const remoteNode = $derived(nodeScope.selected);
	const homeNode = $derived(nodes.selfId ?? capabilities.data?.instance_id ?? '');
	const activeNode = $derived(remoteNode ?? homeNode);
	const serverChoices = $derived<ServerChoice[]>(
		nodes.items.map((node) => ({
			id: node.id,
			endpoint: node.endpoint,
			self: node.self,
			up: 'up' === node.status,
			role: roleOf(node, nodes.items),
			latencyMs: true === node.self ? null : node.latency_ms
		}))
	);
	const tightBar = $derived(crowded);
	const nodeCommandKey = $derived(
		nodes.items.map((node) => `${node.id}:${node.status}:${nodes.isActive(node) ? 1 : 0}`).join('|')
	);
	const versionLabel = $derived(t('shell.footer.version', { version: capabilities.data?.version ?? '' }));
	const userId = $derived(session.me?.user_id ?? '');
	const sidebarGroups = $derived(groupNav(visibleItems));
	const primaryNav = $derived<BottomNavItem[]>(
		visibleItems.slice(0, PRIMARY_NAV_COUNT).map((item) => toNavItem(item))
	);
	const moreNav = $derived<SidebarGroup[]>(groupNav(visibleItems.slice(PRIMARY_NAV_COUNT)));
	const bannerKind = $derived<BannerKind | null>(
		selectBanner({ restartRequired, noLeader, apiReachable: hub.connection.apiReachable, criticalFindings })
	);
	const hasDock = $derived(0 < transfers.items.length || undefined !== bulkBar);
	const commandContext = $derived({
		role,
		route: pathname,
		selectionCount: 0,
		capabilities: capabilityNames
	});
	$effect.pre(() => {
		setCommandContext(commandContext);
	});
	const paletteCommands = $derived(commands.list(commandContext).map((entry) => entry.command));
	const accountItems = $derived<MenuItem[]>([
		{
			id: 'appearance',
			label: t('galaxy.account.appearance'),
			icon: 'palette',
			onselect: () => goTo('/settings/appearance')
		},
		{
			id: 'shortcuts',
			label: t('galaxy.account.shortcuts'),
			icon: 'keyboard',
			onselect: () => goTo('/settings/shortcuts')
		},
		{
			id: 'copy-user-id',
			label: t('galaxy.account.copy_user_id'),
			icon: 'clipboard',
			onselect: copyUserId
		},
		{
			id: 'sign-out',
			label: t('galaxy.account.sign_out'),
			icon: 'logout',
			tone: 'danger',
			onselect: () => void signOutFromAccount()
		}
	]);
	const keymapSections = $derived<KeymapSection[]>(
		SCOPE_ORDER.map((scope) => ({
			id: scope,
			title: t(`galaxy.shortcuts.scope.${scope}`),
			entries: shortcuts
				.effective()
				.filter((shortcut) => scope === shortcut.scope)
				.map((shortcut) => ({ id: shortcut.id, label: t(shortcut.labelKey), keys: shortcut.keys }))
		})).filter((section) => 0 < section.entries.length)
	);

	function stripBase(pathnameValue: string): string {
		if (0 < base.length && pathnameValue.startsWith(base)) {
			return pathnameValue.slice(base.length) || '/';
		}
		return pathnameValue;
	}

	function sectionOf(path: string): string {
		const [, section = ''] = path.split('/');
		return section;
	}

	function toNavItem(item: NavItem): BottomNavItem {
		return { id: item.id, label: t(`galaxy.nav.${item.id}`), href: `${base}${item.href}`, icon: item.icon };
	}

	function groupNav(items: readonly NavItem[]): SidebarGroup[] {
		return navGroups
			.map((group) => ({
				id: group.toLowerCase(),
				label: t(GROUP_LABEL_KEYS[group]),
				items: items.filter((item) => group === item.group).map((item) => toNavItem(item))
			}))
			.filter((group) => 0 < group.items.length);
	}

	function goTo(path: string): void {
		void goto(`${base}${path}`);
	}

	function interceptAppLinks(event: MouseEvent): void {
		if (event.defaultPrevented || 0 !== event.button || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
			return;
		}
		const target = event.target;
		if (false === target instanceof Element) {
			return;
		}
		const anchor = target.closest('a[href]');
		if (null === anchor || anchor.hasAttribute('download') || '_self' !== (anchor.getAttribute('target') ?? '_self')) {
			return;
		}
		const href = anchor.getAttribute('href') ?? '';
		if (false === href.startsWith('/') || href.startsWith('//') || href.startsWith('/v1') || base === href || href.startsWith(`${base}/`)) {
			return;
		}
		event.preventDefault();
		goTo(href);
	}

	function openPalette(): void {
		paletteOpen = true;
	}

	function openSheet(): void {
		sheetOpen = true;
	}

	function openHelp(term?: GlossaryKey): void {
		helpTopic = term;
		helpOpen = true;
	}

	function openNotificationsDrawer(): void {
		notificationsOpen = true;
	}

	function openJobs(): void {
		goTo('/activity/jobs');
	}

	function toggleSidebar(): void {
		preferences.update({ sidebarCollapsed: !preferences.value.sidebarCollapsed });
	}

	function toggleTheme(): void {
		preferences.update({ theme: 'dark' === preferences.value.theme ? 'light' : 'dark' });
	}

	function readTheme(): ThemeMode {
		return preferences.value.theme;
	}

	function readCollapsed(): boolean {
		return preferences.value.sidebarCollapsed;
	}

	function writeCollapsed(value: boolean): void {
		preferences.update({ sidebarCollapsed: value });
	}

	function focusSearch(): void {
		paletteOpen = true;
	}

	function closeTopmost(): void {
		if (true === paletteOpen) {
			paletteOpen = false;
		} else if (true === sheetOpen) {
			sheetOpen = false;
		} else if (true === helpOpen) {
			helpOpen = false;
		} else if (true === notificationsOpen) {
			notificationsOpen = false;
		}
	}

	function selectHelpTopic(key: GlossaryKey): void {
		helpTopic = key;
	}

	function currentDiagnostics(): string {
		return buildDiagnostics({
			version: capabilities.data?.version ?? '',
			route: pathname,
			requestIds: recentRequestIds(recentRequestRecords()),
			userAgent: navigator.userAgent
		});
	}

	function copyText(text: string, success: string, failure: string): void {
		const clipboard = navigator.clipboard;
		if (undefined === clipboard) {
			toasts.push({ tone: 'danger', title: failure });
			return;
		}
		clipboard.writeText(text).then(
			() => toasts.push({ tone: 'ok', title: success }),
			() => toasts.push({ tone: 'danger', title: failure })
		);
	}

	function copyUserId(): void {
		if (null === session.me) {
			return;
		}
		copyText(
			session.me.user_id,
			t('galaxy.account.user_id_copied'),
			t('galaxy.account.user_id_failed')
		);
	}

	function reportProblem(): void {
		copyText(currentDiagnostics(), t('galaxy.report.copied'), t('galaxy.report.failed'));
	}

	async function signOutFromAccount(): Promise<void> {
		const result = await session.signOut();
		if (false === result.ok) {
			toasts.push({ tone: 'danger', title: result.error.title, body: result.error.body });
		}
	}

	async function staySignedIn(): Promise<void> {
		const result = await session.refresh();
		if (false === result.ok) {
			toasts.push({ tone: 'danger', title: t('galaxy.session.stay_failed') });
		}
	}

	async function reauthenticate(key: string): Promise<void> {
		const result = await session.signIn(key);
		if (false === result.ok) {
			toasts.push({ tone: 'danger', title: result.error.title, body: result.error.body });
		}
	}

	function runCommand(command: Command, variant: 'primary' | 'secondary'): void {
		paletteOpen = false;
		void commands.execute(command.id, variant).then((result) => {
			if (false === result.ok) {
				toasts.push({ tone: 'warn', title: result.error.reason ?? t('galaxy.command.failed') });
			}
		});
	}

	function searchCommands(raw: string): SearchHit[] {
		const parsed = parseCommandQuery(raw);
		return commands.search(parsed.query, parsed.mode);
	}

	async function refresh(): Promise<void> {
		const [healthResult, clusterResult, configResult, findingsResult] = await Promise.all([
			loadHealth(true),
			loadCluster(),
			getConfig(),
			loadFindings()
		]);
		hub.connection.apiReachable = true === healthResult.ok || 'network' !== healthResult.error.layer;
		if (true === clusterResult.ok) {
			noLeader = 'nats' === clusterResult.value.mode && null === clusterResult.value.leader_id;
		}
		if (true === configResult.ok) {
			restartRequired = true === configResult.value.restart_required;
		}
		if (true === findingsResult.ok) {
			criticalFindings = findingsResult.value.counts.critical;
			warningFindings = findingsResult.value.counts.warning;
		}
	}

	function bannerRunRestart(): void {
		goTo('/settings/file');
	}

	function bannerRunTopology(): void {
		goTo('/cluster');
	}

	function bannerRunOverview(): void {
		goTo('/');
	}

	function bindShortcuts(): () => void {
		const navigation: [string, string][] = [
			['go.overview', '/'],
			['go.files', '/files'],
			['go.sharing', '/sharing'],
			['go.cluster', '/cluster'],
			['go.insights', '/insights'],
			['go.activity', '/activity'],
			['go.transfers', '/files/transfers'],
			['go.jobs', '/activity/jobs'],
			['go.logs', '/activity/logs']
		];
		const disposers = [
			shortcuts.bind('palette.open', openPalette),
			shortcuts.bind('search.focus', focusSearch),
			shortcuts.bind('sheet.open', openSheet),
			shortcuts.bind('go.settings', () => goTo(isAdmin ? '/settings/network' : '/settings/appearance')),
			shortcuts.bind('sidebar.toggle', () => {
				if (false === narrow) {
					toggleSidebar();
				}
			}),
			shortcuts.bind('notifications.open', openNotificationsDrawer),
			shortcuts.bind('theme.toggle', toggleTheme),
			shortcuts.bind('escape.close', closeTopmost),
			...navigation.map(([id, path]) => shortcuts.bind(id, () => goTo(path)))
		];
		return () => {
			for (const dispose of disposers) {
				dispose();
			}
		};
	}

	$effect(() => {
		const query = window.matchMedia(`(min-width: ${BREAKPOINTS.sm}px)`);
		const sync = (): void => {
			narrow = false === query.matches;
		};
		sync();
		query.addEventListener('change', sync);
		return () => query.removeEventListener('change', sync);
	});

	$effect(() => {
		const query = window.matchMedia(`(min-width: ${BREAKPOINTS.xl}px)`);
		const sync = (): void => {
			crowded = false === query.matches;
		};
		sync();
		query.addEventListener('change', sync);
		return () => query.removeEventListener('change', sync);
	});

	$effect(() => shortcuts.start(window));

	$effect(bindShortcuts);

	$effect(() => session.watchActivity(window));

	$effect(() => {
		const timer = setInterval(() => session.tick(), SESSION_TICK_MS);
		return () => clearInterval(timer);
	});

	$effect(() => {
		if (null === session.me) {
			return;
		}
		void capabilities.load().then((result) => {
			if (false === result.ok) {
				toasts.push({ tone: 'danger', title: result.error.title, body: result.error.body });
			}
		});
	});

	$effect(() => {
		if (false === isAdmin) {
			return;
		}
		void nodeScope.selected;
		void refresh();
		const timer = setInterval(() => void refresh(), POLL_INTERVAL_MS);
		return () => clearInterval(timer);
	});

	$effect(() => {
		if (false === isAdmin) {
			return;
		}
		hub.start();
		const unsubscribe = hub.subscribe(ADMIN_TOPICS);
		const removeListener = hub.addListener((event) => {
			if ('job.progress' !== event.type) {
				void refresh();
			}
		});
		return () => {
			removeListener();
			unsubscribe();
			hub.stop();
		};
	});

	$effect(() => {
		if (null === session.me) {
			return;
		}
		void nodes.load();
		const timer = setInterval(() => {
			if (false === document.hidden) {
				void nodes.load();
			}
		}, NODE_POLL_MS);
		return () => clearInterval(timer);
	});

	$effect(() => {
		void nodeCommandKey;
		return untrack(() =>
			commands.register(
				createNodeCommands(
					nodes.items.map((node) => ({ id: node.id, up: 'up' === node.status, active: nodes.isActive(node) })),
					(id) => controlNode(id)
				)
			)
		);
	});

	$effect(() => {
		applyStatusSignals({ criticalFindings, failedJobs: jobs.failed, warningFindings });
	});

	$effect(() => {
		const currentRole = role;
		return untrack(() =>
			commands.register(
			createGlobalCommands({
				role: currentRole,
				go: goTo,
				currentTheme: readTheme,
				setPreferences: (patch) => preferences.update(patch),
				openShortcuts: openSheet,
				openHelp,
				openNotifications: openNotificationsDrawer,
				toggleSidebar,
				reportProblem,
				signOut: () => void signOutFromAccount(),
				copyUserId
			})
			)
		);
	});

	$effect(() => {
		document.addEventListener('click', interceptAppLinks);
		return () => document.removeEventListener('click', interceptAppLinks);
	});

	$effect(() => {
		shortcuts.setSingleKeyEnabled(preferences.value.singleKeyShortcuts);
	});
</script>

<div class={['console-galaxy', className]} {...rest}>
	<OverlayHost />
	<a class="skip-link" href="#main">{t('galaxy.skip_link')}</a>
	{#snippet liveDot(withLabel: boolean)}
		<span class="live" data-tone={liveTone}>
			<span class="live-dot" aria-hidden="true"></span>
			{#if withLabel}
				<span class="live-label">{t(`u080molecules.connection.${liveState}`)}</span>
			{:else}
				<span class="sr-only">{t(`u080molecules.connection.${liveState}`)}</span>
			{/if}
		</span>
	{/snippet}
	<div class={['shell', narrow && 'narrow']}>
		{#if false === narrow}
			<div class="sidebar-col">
				<AppSidebar groups={sidebarGroups} current={currentId} home={`${base}/`} bind:collapsed={readCollapsed, writeCollapsed}>
					{#snippet footer(collapsed)}
						{#if collapsed}
							<Tooltip content={`${serverName} · ${versionLabel}`} side="right">
								<span class="server-rail" tabindex="0">{@render liveDot(false)}</span>
							</Tooltip>
						{:else}
							<div class="server-card">
								<span class="server-icon" aria-hidden="true"><Icon name="server" size="md" /></span>
								<span class="server-text">
									<span class="server-name" title={serverName}>{serverName}</span>
									<span class="server-meta">{versionLabel}</span>
								</span>
								{@render liveDot(false)}
							</div>
						{/if}
					{/snippet}
				</AppSidebar>
			</div>
		{/if}
		<div class="column">
			{#snippet titleSlot()}
				{#if narrow}
					<a class="mobile-brand" href={`${base}/`} aria-label={t('shell.brand.home')}>
						<span class="mobile-mark" aria-hidden="true"><Icon name="hard-drive" size="sm" /></span>
					</a>
				{/if}
				<nav class="crumbs" aria-label={t('shell.breadcrumb.label')}>
					{#if '' !== currentGroup && false === narrow}
						<span class="crumb-group">{currentGroup}</span>
						<span class="crumb-sep" aria-hidden="true"><Icon name="chevron-right" size="sm" /></span>
					{/if}
					<span class="crumb-current" aria-current="page">{currentLabel}</span>
				</nav>
			{/snippet}
			{#snippet searchSlot()}
				<button type="button" id="global-search" class="search-trigger" aria-haspopup="dialog" onclick={openPalette}>
					<Icon name="search" size="md" />
					<span class="search-placeholder">{t('shell.search.placeholder')}</span>
					<Kbd keys={['mod', 'k']} />
				</button>
			{/snippet}
			{#snippet statusSlot()}
				{#if nodes.multi}
					<ServerPicker
						servers={serverChoices}
						active={activeNode}
						remote={null !== remoteNode}
						compact={narrow || tightBar}
						onselect={(id) => controlNode(id)}
					/>
				{/if}
				{#if false === narrow}
					{@render liveDot(false === tightBar)}
				{/if}
			{/snippet}
			{#snippet actionsSlot()}
				<span class="search-icon">
					<IconButton icon="search" label={t('shell.search.open')} onclick={openPalette} />
				</span>
				{#if false === narrow}
					<IconButton
						icon="jobs"
						label={t('galaxy.jobs.open')}
						badge={0 < jobs.running ? jobs.running : undefined}
						onclick={openJobs}
					/>
				{/if}
				<IconButton
					icon="bell"
					label={t('galaxy.notifications.open')}
					badge={0 < notifications.unread ? notifications.unread : undefined}
					shortcut={['mod', '.']}
					onclick={openNotificationsDrawer}
				/>
				{#if narrow}
					<IconButton
						icon={'dark' === preferences.value.theme ? 'sun' : 'moon'}
						label={t('galaxy.appearance.theme.toggle')}
						onclick={toggleTheme}
					/>
				{:else}
					<IconButton icon="help" label={t('galaxy.help.open')} onclick={() => openHelp()} />
				{/if}
			{/snippet}
			{#snippet accountSlot()}
				<Menu label={t('galaxy.account.menu')} items={accountItems}>
					{#snippet trigger(props)}
						<button type="button" class="account" {...props} aria-label={t('shell.account.label', { user: userId })}>
							<Avatar name={userId} size="sm" />
							{#if false === narrow}
								{#if false === tightBar}
									<span class="account-name">{userId}</span>
								{/if}
								{#if isAdmin && false === tightBar}
									<Badge tone="accent" variant="soft">{t('galaxy.account.admin')}</Badge>
								{/if}
								{#if false === tightBar}
									<span class="account-chevron" aria-hidden="true"><Icon name="chevron-down" size="sm" /></span>
								{/if}
							{/if}
						</button>
					{/snippet}
				</Menu>
			{/snippet}
			<TopBar title={titleSlot} search={searchSlot} status={statusSlot} actions={actionsSlot} account={accountSlot} />
			{#if null !== remoteNode}
				<ControlStrip node={remoteNode} home={homeNode} compact={narrow} onback={() => controlNode(null)} />
			{/if}
			{#if null !== bannerKind}
				<div class="banners">
					{#if 'restart' === bannerKind}
						<Banner
							tone="warn"
							title={t('galaxy.banner.restart.title')}
							body={t('galaxy.banner.restart.body')}
							action={{ label: t('galaxy.banner.restart.action'), run: bannerRunRestart }}
						/>
					{:else if 'no-leader' === bannerKind}
						<Banner
							tone="danger"
							title={t('galaxy.banner.no_leader.title')}
							body={t('galaxy.banner.no_leader.body')}
							action={{ label: t('galaxy.banner.no_leader.action'), run: bannerRunTopology }}
						/>
					{:else if 'unreachable' === bannerKind}
						<Banner
							tone="danger"
							title={t('galaxy.banner.unreachable.title')}
							body={t('galaxy.banner.unreachable.body')}
							action={{ label: t('galaxy.banner.unreachable.action'), run: () => void refresh() }}
						/>
					{:else if 'findings' === bannerKind}
						<Banner
							tone="danger"
							title={t('galaxy.banner.findings.title', { count: criticalFindings })}
							action={{ label: t('galaxy.banner.findings.action'), run: bannerRunOverview }}
						/>
					{/if}
				</div>
			{/if}
			<main id="main" class={['main', hasDock && 'has-dock']} tabindex="-1">
				<div class="page">
					{#key remoteNode}
						{@render children()}
					{/key}
				</div>
			</main>
		</div>
		{#if narrow}
			<BottomNav items={primaryNav} current={currentId} more={moreNav} />
		{/if}
	</div>
	{#if hasDock}
		<Dock aria-label={t('galaxy.dock.label')}>
			{#if 0 < transfers.items.length}
				<TransferTray
					items={transfers.items}
					aggregate={transfers.aggregate}
					bind:collapsed={trayCollapsed}
					onpause={(id) => transfers.pause(id)}
					onresume={(id) => transfers.resume(id)}
					oncancel={(id) => void transfers.cancel(id)}
					onretry={(id) => transfers.retry(id)}
					onretryfailed={() => transfers.retryFailed()}
					onclear={() => transfers.clearFinished()}
				/>
			{/if}
			{#if undefined !== bulkBar}
				{@render bulkBar()}
			{/if}
		</Dock>
	{/if}
	<Toaster
		items={toasts.items}
		ondismiss={(id) => toasts.dismiss(id)}
		onpause={(id) => toasts.pause(id)}
		onresume={(id) => toasts.resume(id)}
	/>
	<Drawer bind:open={notificationsOpen} title={t('galaxy.notifications.title')}>
		<NotificationCenter
			items={notifications.items}
			onread={(id) => notifications.markRead(id)}
			onclear={(id) => notifications.clear(id)}
			onopen={(entry) => {
				notifications.markRead(entry.id);
				notificationsOpen = false;
			}}
		/>
	</Drawer>
	<CommandPalette
		bind:open={paletteOpen}
		commands={paletteCommands}
		recent={[...commands.recent]}
		context={commandContext}
		search={searchCommands}
		onrun={runCommand}
	/>
	<ShortcutSheet bind:open={sheetOpen} keymap={keymapSections} singleKeyShortcuts={shortcuts.singleKeyEnabled} />
	<HelpPanel
		open={helpOpen}
		topic={helpTopic}
		ontopic={selectHelpTopic}
		diagnostics={currentDiagnostics()}
		shortcuts={shortcuts
			.effective()
			.filter((shortcut) => 'global' === shortcut.scope)
			.map((shortcut) => ({ label: t(shortcut.labelKey), keys: shortcut.keys.map((alternative) => alternative.join('+')) }))}
	/>
	<SessionExpiryDialog
		open={null !== session.warning}
		reason={session.warning?.reason ?? 'idle'}
		secondsLeft={session.warning?.secondsLeft ?? 0}
		onstay={staySignedIn}
		onsignout={() => void signOutFromAccount()}
		onreauth={reauthenticate}
	/>
	<LiveRegions />
</div>

<style>
	.console-galaxy {
		min-block-size: 100dvh;
		background: var(--bg);
	}

	.shell {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		min-block-size: 100dvh;
	}

	.shell.narrow {
		grid-template-columns: minmax(0, 1fr);
	}

	.sidebar-col {
		border-inline-end: var(--hairline) solid var(--border-subtle);
		background: var(--bg-subtle);
	}

	.column {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
	}

	.sr-only {
		position: absolute;
		inline-size: var(--space-px);
		block-size: var(--space-px);
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.crumbs {
		display: flex;
		align-items: center;
		gap: var(--space-1-5);
		min-inline-size: 0;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.crumb-group {
		min-inline-size: 0;
		overflow: hidden;
		color: var(--text-faint);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.crumb-sep {
		display: inline-flex;
		color: var(--text-faint);
	}

	.crumb-current {
		min-inline-size: 0;
		overflow: hidden;
		color: var(--text);
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.mobile-brand {
		display: inline-flex;
		border-radius: var(--radius-sm);
	}

	.mobile-mark {
		display: grid;
		place-items: center;
		inline-size: var(--space-6);
		block-size: var(--space-6);
		border-radius: var(--radius-sm);
		background: var(--accent);
		color: var(--text-on-accent);
	}

	.search-trigger {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		inline-size: 100%;
		block-size: var(--control-h);
		padding-inline: var(--space-3) var(--space-1-5);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface);
		color: var(--text-faint);
		font: inherit;
		font-size: var(--text-body-size);
		text-align: start;
		cursor: pointer;
		box-shadow: var(--shadow-xs);
		transition:
			border-color var(--dur-micro) var(--ease-out),
			background-color var(--dur-micro) var(--ease-out);
	}

	@media (hover: hover) {
		.search-trigger:hover {
			border-color: var(--border);
			color: var(--text-muted);
		}
	}

	.search-placeholder {
		flex: auto;
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.search-icon {
		display: none;
	}

	.live {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1-5);
		flex: none;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: 500;
		color: var(--text-muted);
		white-space: nowrap;
	}

	.live-dot {
		position: relative;
		inline-size: var(--space-2);
		block-size: var(--space-2);
		border-radius: var(--radius-full);
		background: var(--text-faint);
	}

	.live[data-tone='ok'] .live-dot {
		background: var(--ok);
		box-shadow: 0 0 0 calc(var(--space-px) * 3) var(--ok-soft);
	}

	.live[data-tone='warn'] .live-dot {
		background: var(--warn);
		box-shadow: 0 0 0 calc(var(--space-px) * 3) var(--warn-soft);
	}

	.live[data-tone='danger'] .live-dot {
		background: var(--danger);
		box-shadow: 0 0 0 calc(var(--space-px) * 3) var(--danger-soft);
	}

	.live[data-tone='danger'] .live-label {
		color: var(--danger-text);
	}

	.account {
		display: inline-flex;
		flex: none;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: max-content;
		block-size: var(--control-h);
		padding-inline: var(--space-1) var(--space-2);
		border: 0;
		border-radius: var(--radius-full);
		background: none;
		color: var(--text);
		font: inherit;
		font-size: var(--text-body-size);
		cursor: pointer;
	}

	@media (hover: hover) {
		.account:hover {
			background: var(--surface-hover);
		}
	}

	.account:focus-visible,
	.search-trigger:focus-visible,
	.server-rail:focus-visible,
	.mobile-brand:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.account-name {
		max-inline-size: calc(var(--space-4) * 10);
		overflow: hidden;
		font-weight: 500;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.account-chevron {
		display: inline-flex;
		color: var(--text-faint);
	}

	.server-card {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface);
	}

	.server-icon {
		display: inline-flex;
		color: var(--text-faint);
	}

	.server-text {
		display: flex;
		flex: auto;
		flex-direction: column;
		min-inline-size: 0;
	}

	.server-name {
		overflow: hidden;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: 600;
		color: var(--text);
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.server-meta {
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		color: var(--text-faint);
	}

	.server-rail {
		display: grid;
		place-items: center;
		inline-size: var(--space-8);
		block-size: var(--space-8);
		border-radius: var(--radius-md);
	}

	.banners {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-block-start: var(--space-4);
		padding-inline: var(--gutter);
	}

	.main {
		flex: auto;
		min-inline-size: 0;
		padding-block: var(--space-8) var(--space-12);
		padding-inline: var(--gutter);
	}

	.main:focus {
		outline: var(--focus-width) solid transparent;
	}

	.page {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		inline-size: 100%;
		max-inline-size: var(--content-max);
		min-inline-size: 0;
		margin-inline: auto;
	}

	.main.has-dock {
		padding-block-end: calc(var(--dock-h) + var(--space-8));
	}

	.shell.narrow .main {
		padding-block: var(--space-5) calc(var(--touch-target) + var(--space-10));
	}

	@media (max-width: 899px) {
		.search-icon {
			display: inline-flex;
		}
	}
</style>
