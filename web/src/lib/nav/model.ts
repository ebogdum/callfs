import type { IconName } from '$lib/ui/types';

export type Role = 'admin' | 'member';
export type NavRole = 'all' | 'admin';
export type NavGroup = 'Work' | 'Operate' | 'Configure';
export type ClusterSubgroup = 'Members' | 'Data' | 'Runtime';

export interface NavChild {
	id: string;
	labelKey: string;
	href: string;
	role: NavRole;
	capability: string | null;
	subgroup: ClusterSubgroup | null;
}

export interface NavItem {
	id: string;
	labelKey: string;
	icon: IconName;
	href: string;
	memberHref: string | null;
	group: NavGroup;
	role: NavRole;
	capability: string | null;
	children: NavChild[];
}

export const navGroups: readonly NavGroup[] = ['Work', 'Operate', 'Configure'];

const child = (
	id: string,
	href: string,
	role: NavRole = 'all',
	capability: string | null = null,
	subgroup: ClusterSubgroup | null = null
): NavChild => ({ id, labelKey: `nav.${id}`, href, role, capability, subgroup });

export const navItems: readonly NavItem[] = [
	{
		id: 'overview',
		labelKey: 'nav.overview',
		icon: 'home',
		href: '/',
		memberHref: null,
		group: 'Work',
		role: 'all',
		capability: null,
		children: []
	},
	{
		id: 'files',
		labelKey: 'nav.files',
		icon: 'folder',
		href: '/files',
		memberHref: null,
		group: 'Work',
		role: 'all',
		capability: null,
		children: [
			child('files.browse', '/files'),
			child('files.search', '/files/search'),
			child('files.flat', '/files/flat', 'admin'),
			child('files.transfers', '/files/transfers'),
			child('files.trash', '/files/trash')
		]
	},
	{
		id: 'sharing',
		labelKey: 'nav.sharing',
		icon: 'link',
		href: '/sharing',
		memberHref: null,
		group: 'Work',
		role: 'all',
		capability: null,
		children: []
	},
	{
		id: 'cluster',
		labelKey: 'nav.cluster',
		icon: 'network',
		href: '/cluster',
		memberHref: null,
		group: 'Operate',
		role: 'admin',
		capability: null,
		children: [
			child('cluster.topology', '/cluster', 'admin', null, 'Members'),
			child('cluster.members', '/cluster/nodes', 'admin', null, 'Members'),
			child('cluster.replication', '/cluster/replication', 'admin', 'nats', 'Members'),
			child('cluster.servers', '/cluster/servers', 'admin', null, 'Members'),
			child('cluster.storage', '/cluster/storage', 'admin', null, 'Data'),
			child('cluster.durability', '/cluster/durability', 'admin', null, 'Data'),
			child('cluster.backup', '/cluster/backup', 'admin', null, 'Data'),
			child('cluster.hygiene', '/cluster/hygiene', 'admin', null, 'Data'),
			child('cluster.locks', '/cluster/locks', 'admin', null, 'Runtime'),
			child('cluster.cache', '/cluster/cache', 'admin', null, 'Runtime'),
			child('cluster.limits', '/cluster/limits', 'admin', null, 'Runtime')
		]
	},
	{
		id: 'insights',
		labelKey: 'nav.insights',
		icon: 'line-chart',
		href: '/insights',
		memberHref: null,
		group: 'Operate',
		role: 'admin',
		capability: null,
		children: [
			child('insights.dashboards', '/insights', 'admin'),
			child('insights.explore', '/insights/explore', 'admin'),
			child('insights.storage', '/insights/storage', 'admin'),
			child('insights.metrics', '/insights/metrics', 'admin')
		]
	},
	{
		id: 'activity',
		labelKey: 'nav.activity',
		icon: 'activity',
		href: '/activity',
		memberHref: null,
		group: 'Operate',
		role: 'all',
		capability: null,
		children: [
			child('activity.events', '/activity', 'admin'),
			child('activity.audit', '/activity/audit', 'admin'),
			child('activity.logs', '/activity/logs', 'admin'),
			child('activity.jobs', '/activity/jobs'),
			child('activity.signins', '/activity/sign-ins', 'admin')
		]
	},
	{
		id: 'settings',
		labelKey: 'nav.settings',
		icon: 'settings',
		href: '/settings/network',
		memberHref: '/settings/appearance',
		group: 'Configure',
		role: 'all',
		capability: null,
		children: []
	}
];

const isVisible = (
	entry: { role: NavRole; capability: string | null },
	role: Role,
	capabilities: readonly string[]
): boolean =>
	('all' === entry.role || 'admin' === role) &&
	(null === entry.capability || capabilities.includes(entry.capability));

export function visibleNavItems(role: Role, capabilities: readonly string[]): NavItem[] {
	return navItems
		.filter((item) => isVisible(item, role, capabilities))
		.map((item) => ({
			...item,
			href: 'member' === role && null !== item.memberHref ? item.memberHref : item.href,
			children: item.children.filter((entry) => isVisible(entry, role, capabilities))
		}));
}
