import type { Command } from '$lib/commands/registry.svelte';
import { GLOSSARY, type GlossaryKey } from '$lib/copy/glossary';
import { t } from '$lib/copy/t';
import type { Role } from '$lib/nav/model';
import type { PreferencesPatch, ThemeMode } from '$lib/stores/preferences.svelte';

const DIAGNOSTIC_REQUEST_LIMIT = 10;

export interface RouteEntry {
	slug: string;
	path: string;
	admin: boolean;
	shortcut?: string[];
}

export const ROUTES: readonly RouteEntry[] = [
	{ slug: 'overview', path: '/', admin: false, shortcut: ['g', 'o'] },
	{ slug: 'files', path: '/files', admin: false, shortcut: ['g', 'f'] },
	{ slug: 'files.search', path: '/files/search', admin: false },
	{ slug: 'files.flat', path: '/files/flat', admin: true },
	{ slug: 'files.transfers', path: '/files/transfers', admin: false, shortcut: ['g', 't'] },
	{ slug: 'files.trash', path: '/files/trash', admin: false },
	{ slug: 'sharing', path: '/sharing', admin: false, shortcut: ['g', 's'] },
	{ slug: 'cluster', path: '/cluster', admin: true, shortcut: ['g', 'c'] },
	{ slug: 'cluster.nodes', path: '/cluster/nodes', admin: true },
	{ slug: 'cluster.raft', path: '/cluster/raft', admin: true },
	{ slug: 'cluster.servers', path: '/cluster/servers', admin: true },
	{ slug: 'cluster.storage', path: '/cluster/storage', admin: true },
	{ slug: 'cluster.durability', path: '/cluster/durability', admin: true },
	{ slug: 'cluster.backup', path: '/cluster/backup', admin: true },
	{ slug: 'cluster.hygiene', path: '/cluster/hygiene', admin: true },
	{ slug: 'cluster.locks', path: '/cluster/locks', admin: true },
	{ slug: 'cluster.cache', path: '/cluster/cache', admin: true },
	{ slug: 'cluster.limits', path: '/cluster/limits', admin: true },
	{ slug: 'insights', path: '/insights', admin: true, shortcut: ['g', 'i'] },
	{ slug: 'insights.explore', path: '/insights/explore', admin: true },
	{ slug: 'insights.storage', path: '/insights/storage', admin: true },
	{ slug: 'insights.metrics', path: '/insights/metrics', admin: true },
	{ slug: 'activity', path: '/activity', admin: false, shortcut: ['g', 'a'] },
	{ slug: 'activity.audit', path: '/activity/audit', admin: true },
	{ slug: 'activity.logs', path: '/activity/logs', admin: true, shortcut: ['g', 'l'] },
	{ slug: 'activity.jobs', path: '/activity/jobs', admin: false, shortcut: ['g', 'j'] },
	{ slug: 'activity.sign-ins', path: '/activity/sign-ins', admin: true },
	{ slug: 'settings.network', path: '/settings/network', admin: true },
	{ slug: 'settings.limits', path: '/settings/limits', admin: true },
	{ slug: 'settings.security', path: '/settings/security', admin: true },
	{ slug: 'settings.storage', path: '/settings/storage', admin: true },
	{ slug: 'settings.index', path: '/settings/index', admin: true },
	{ slug: 'settings.coordination', path: '/settings/coordination', admin: true },
	{ slug: 'settings.logging', path: '/settings/logging', admin: true },
	{ slug: 'settings.access', path: '/settings/access', admin: true },
	{ slug: 'settings.file', path: '/settings/file', admin: true },
	{ slug: 'settings.appearance', path: '/settings/appearance', admin: false },
	{ slug: 'settings.shortcuts', path: '/settings/shortcuts', admin: false },
	{ slug: 'settings.about', path: '/settings/about', admin: false },
	{ slug: 'wall', path: '/wall', admin: false },
	{ slug: 'report', path: '/report', admin: false }
];

interface AppearanceChoice {
	id: string;
	copy: string;
	patch: PreferencesPatch;
}

const APPEARANCE_CHOICES: readonly AppearanceChoice[] = [
	{ id: 'theme.system', copy: 'galaxy.appearance.theme.system', patch: { theme: 'system' } },
	{ id: 'theme.light', copy: 'galaxy.appearance.theme.light', patch: { theme: 'light' } },
	{ id: 'theme.dark', copy: 'galaxy.appearance.theme.dark', patch: { theme: 'dark' } },
	{ id: 'contrast.standard', copy: 'galaxy.appearance.contrast.standard', patch: { contrast: 'standard' } },
	{ id: 'contrast.more', copy: 'galaxy.appearance.contrast.more', patch: { contrast: 'more' } },
	{ id: 'density.compact', copy: 'galaxy.appearance.density.compact', patch: { density: 'compact' } },
	{ id: 'density.default', copy: 'galaxy.appearance.density.default', patch: { density: 'default' } },
	{ id: 'density.comfortable', copy: 'galaxy.appearance.density.comfortable', patch: { density: 'comfortable' } },
	{ id: 'motion.full', copy: 'galaxy.appearance.motion.full', patch: { motion: 'full' } },
	{ id: 'motion.reduced', copy: 'galaxy.appearance.motion.reduced', patch: { motion: 'reduced' } },
	{ id: 'motion.none', copy: 'galaxy.appearance.motion.none', patch: { motion: 'none' } },
	{ id: 'font.small', copy: 'galaxy.appearance.font.small', patch: { fontScale: 0.875 } },
	{ id: 'font.default', copy: 'galaxy.appearance.font.default', patch: { fontScale: 1 } },
	{ id: 'font.large', copy: 'galaxy.appearance.font.large', patch: { fontScale: 1.125 } },
	{ id: 'font.xlarge', copy: 'galaxy.appearance.font.xlarge', patch: { fontScale: 1.25 } },
	{ id: 'radius.sharp', copy: 'galaxy.appearance.radius.sharp', patch: { radius: 'sharp' } },
	{ id: 'radius.medium', copy: 'galaxy.appearance.radius.medium', patch: { radius: 'medium' } },
	{ id: 'radius.round', copy: 'galaxy.appearance.radius.round', patch: { radius: 'round' } }
];

export const APPEARANCE_RESET: PreferencesPatch = {
	theme: 'system',
	contrast: 'standard',
	density: 'default',
	motion: 'full',
	fontScale: 1,
	radius: 'medium'
};

export interface GlobalCommandDeps {
	role: Role;
	go: (path: string) => void;
	currentTheme: () => ThemeMode;
	setPreferences: (patch: PreferencesPatch) => void;
	openShortcuts: () => void;
	openHelp: (term: GlossaryKey) => void;
	openNotifications: () => void;
	toggleSidebar: () => void;
	reportProblem: () => void;
	signOut: () => void;
	copyUserId: () => void;
}

export interface DiagnosticsInput {
	version: string;
	route: string;
	requestIds: string[];
	userAgent: string;
}

export type BannerKind = 'restart' | 'no-leader' | 'unreachable' | 'findings';

export interface BannerInputs {
	restartRequired: boolean;
	noLeader: boolean;
	apiReachable: boolean;
	criticalFindings: number;
}

function routeCommand(route: RouteEntry, deps: GlobalCommandDeps): Command {
	return {
		id: `nav.${route.slug}`,
		title: t(`galaxy.route.${route.slug}`),
		group: 'Navigate',
		keywords: [route.path],
		shortcut: route.shortcut,
		run: () => deps.go(route.path)
	};
}

function navigationCommands(deps: GlobalCommandDeps): Command[] {
	return [
		{
			id: 'nav.notifications',
			title: t('galaxy.command.notifications'),
			group: 'Navigate',
			keywords: ['alerts', 'errors'],
			shortcut: ['mod', '.'],
			run: () => deps.openNotifications()
		},
		{
			id: 'nav.sidebar',
			title: t('galaxy.command.sidebar'),
			group: 'Navigate',
			keywords: ['collapse', 'expand'],
			shortcut: ['['],
			run: () => deps.toggleSidebar()
		}
	];
}

function appearanceCommands(deps: GlobalCommandDeps): Command[] {
	const choices: Command[] = APPEARANCE_CHOICES.map((choice) => ({
		id: `appearance.${choice.id}`,
		title: t(choice.copy),
		group: 'Appearance',
		keywords: ['appearance', 'theme', 'display'],
		run: () => deps.setPreferences(choice.patch)
	}));
	return [
		...choices,
		{
			id: 'appearance.theme.toggle',
			title: t('galaxy.appearance.theme.toggle'),
			group: 'Appearance',
			keywords: ['dark', 'light', 'mode'],
			shortcut: ['mod', 'shift', 'l'],
			run: () => deps.setPreferences({ theme: 'dark' === deps.currentTheme() ? 'light' : 'dark' })
		},
		{
			id: 'appearance.editor',
			title: t('galaxy.appearance.editor'),
			group: 'Appearance',
			keywords: ['colors', 'accent', 'theme editor'],
			run: () => deps.go('/settings/appearance')
		},
		{
			id: 'appearance.reset',
			title: t('galaxy.appearance.reset'),
			group: 'Appearance',
			keywords: ['defaults', 'restore'],
			run: () => deps.setPreferences(APPEARANCE_RESET)
		}
	];
}

function helpCommands(deps: GlobalCommandDeps): Command[] {
	const terms = (Object.keys(GLOSSARY) as GlossaryKey[]).map((key): Command => ({
		id: `help.glossary.${key}`,
		title: t('galaxy.help.term', { term: GLOSSARY[key].word }),
		group: 'Help',
		keywords: [key, GLOSSARY[key].means],
		run: () => deps.openHelp(key)
	}));
	return [
		{
			id: 'help.shortcuts',
			title: t('galaxy.help.shortcuts'),
			group: 'Help',
			keywords: ['keyboard', 'keys', 'hotkeys'],
			shortcut: ['?'],
			run: () => deps.openShortcuts()
		},
		...terms,
		{
			id: 'help.report',
			title: t('galaxy.help.report'),
			group: 'Help',
			keywords: ['bug', 'diagnostics', 'support'],
			run: () => deps.reportProblem()
		}
	];
}

function accountCommands(deps: GlobalCommandDeps): Command[] {
	return [
		{
			id: 'account.sign_out',
			title: t('galaxy.account.sign_out'),
			group: 'Account',
			keywords: ['log out', 'leave'],
			run: () => deps.signOut()
		},
		{
			id: 'account.copy_user_id',
			title: t('galaxy.account.copy_user_id'),
			group: 'Account',
			keywords: ['user', 'id', 'identity'],
			run: () => deps.copyUserId()
		}
	];
}

export function createGlobalCommands(deps: GlobalCommandDeps): Command[] {
	const routes = ROUTES.filter((route) => 'admin' === deps.role || !route.admin).map((route) =>
		routeCommand(route, deps)
	);
	return [
		...routes,
		...navigationCommands(deps),
		...appearanceCommands(deps),
		...helpCommands(deps),
		...accountCommands(deps)
	];
}

export function selectBanner(inputs: BannerInputs): BannerKind | null {
	if (true === inputs.restartRequired) {
		return 'restart';
	}
	if (true === inputs.noLeader) {
		return 'no-leader';
	}
	if (false === inputs.apiReachable) {
		return 'unreachable';
	}
	if (0 < inputs.criticalFindings) {
		return 'findings';
	}
	return null;
}

export function recentRequestIds(records: readonly { requestId: string | null }[]): string[] {
	const ids: string[] = [];
	for (const record of records) {
		if (null !== record.requestId) {
			ids.push(record.requestId);
		}
	}
	return ids.slice(-DIAGNOSTIC_REQUEST_LIMIT);
}

export function buildDiagnostics(input: DiagnosticsInput): string {
	return JSON.stringify(
		{ version: input.version, route: input.route, requestIds: input.requestIds, browser: input.userAgent },
		null,
		2
	);
}

export interface NodeCommandTarget {
	id: string;
	up: boolean;
	active: boolean;
}

export function createNodeCommands(targets: readonly NodeCommandTarget[], switchTo: (id: string) => void): Command[] {
	if (2 > targets.length) {
		return [];
	}
	return targets.map((target): Command => ({
		id: `node.switch.${target.id}`,
		title: t('nodes.command.switch', { node: target.id }),
		group: 'Cluster',
		keywords: ['server', 'node', 'control', 'switch', target.id],
		icon: 'server',
		disabledReason: () => {
			if (target.active) {
				return t('nodes.command.current');
			}
			return target.up ? null : t('nodes.command.down');
		},
		run: () => switchTo(target.id)
	}));
}
