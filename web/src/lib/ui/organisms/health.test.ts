import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ComponentProps } from 'svelte';
import { t } from '$lib/copy/t';
import StatTile from '../molecules/StatTile.svelte';
import FindingList from './FindingList.svelte';
import type { Finding } from './FindingList.svelte';
import KpiStrip from './KpiStrip.svelte';
import PlacementPlanner from './PlacementPlanner.svelte';
import type { PlacementPlan } from './PlacementPlanner.svelte';

type PlannerProps = ComponentProps<typeof PlacementPlanner>;
type ListProps = ComponentProps<typeof FindingList>;
type StatTileProps = ComponentProps<typeof StatTile>;

afterEach(() => {
	cleanup();
});

const plan = (overrides: Partial<PlacementPlan> = {}): PlacementPlan => ({
	total_shards: 6,
	assignments: [],
	per_instance: { 'callfs-1': 3, 'callfs-2': 3 },
	max_on_one_instance: 3,
	tolerated_instance_failures: 1,
	tolerated_piece_losses: 2,
	overhead_ratio: 1.5,
	fault_tolerant: true,
	warnings: [],
	...overrides
});

const plannerProps = (overrides: Partial<PlannerProps> = {}): PlannerProps => ({
	data: 4,
	spare: 2,
	servers: ['callfs-1', 'callfs-2'],
	plan: null,
	status: 'success',
	...overrides
});

const finding = (overrides: Partial<Finding> = {}): Finding => ({
	id: 'tls.plain_http',
	severity: 'critical',
	area: 'security',
	title: 'Connections are not encrypted',
	body: 'Clients send passwords in clear text.',
	affected: [{ kind: 'setting', id: 'server.protocol', href: '/settings/network' }],
	since: '2026-10-22T10:00:00Z',
	fix: { label: 'Open setting', route: '/settings/network', setting_key: 'server.protocol', command: null },
	muted: null,
	...overrides
});

const mutedFinding = (overrides: Partial<Finding> = {}): Finding =>
	finding({
		id: 'disk.low',
		severity: 'warning',
		title: 'Disk is almost full',
		body: 'Local storage has less than ten percent free.',
		affected: [],
		muted: { until: '2026-10-24T10:00:00Z', by: 'alice', reason: 'Planned work' },
		...overrides
	});

const listProps = (overrides: Partial<ListProps> = {}): ListProps => ({
	findings: [finding()],
	onfix: vi.fn(),
	onmute: vi.fn(),
	onunmute: vi.fn(),
	...overrides
});

const tile = (label: string, value: number): StatTileProps => ({ label, value, unit: 'count' });

describe('PlacementPlanner', () => {
	it('names the region and labels both piece inputs', () => {
		render(PlacementPlanner, { props: plannerProps() });
		expect(screen.getByRole('region', { name: t('findings.planner.label') })).toBeInTheDocument();
		expect(screen.getByLabelText(t('findings.planner.data'))).toHaveValue('4');
		expect(screen.getByLabelText(t('findings.planner.spare'))).toHaveValue('2');
	});

	it('groups the presets as native buttons and marks the active one', () => {
		render(PlacementPlanner, { props: plannerProps() });
		const group = screen.getByRole('group', { name: t('findings.planner.presets') });
		expect(group.querySelectorAll('button')).toHaveLength(4);
		const active = screen.getByRole('button', { name: t('findings.planner.preset', { data: 4, spare: 2 }) });
		expect(active).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByText(t('findings.planner.recommended'))).toBeInTheDocument();
	});

	it('applies a preset to both inputs on activation', () => {
		render(PlacementPlanner, { props: plannerProps() });
		const preset = screen.getByRole('button', { name: t('findings.planner.preset', { data: 6, spare: 3 }) });
		fireEvent.click(preset);
		expect(preset).toHaveAttribute('aria-pressed', 'true');
		expect(screen.getByLabelText(t('findings.planner.data'))).toHaveValue('6');
		expect(screen.getByLabelText(t('findings.planner.spare'))).toHaveValue('3');
	});

	it('shows the empty copy when no plan exists', () => {
		render(PlacementPlanner, { props: plannerProps() });
		expect(screen.getByText(t('findings.planner.empty'))).toBeInTheDocument();
	});

	it('shows the error copy and no plan when the plan failed', () => {
		render(PlacementPlanner, { props: plannerProps({ status: 'error', plan: plan() }) });
		expect(screen.getByText(t('findings.planner.error_title'))).toBeInTheDocument();
		expect(screen.queryByText(t('findings.planner.can_lose', { count: 1 }))).toBeNull();
	});

	it('shows the loss allowance, overhead and pieces per server', () => {
		render(PlacementPlanner, { props: plannerProps({ plan: plan() }) });
		expect(screen.getByText(t('findings.planner.can_lose', { count: 1 }))).toBeInTheDocument();
		expect(screen.getByText(t('findings.planner.overhead', { ratio: '1.5' }))).toBeInTheDocument();
		expect(screen.getByRole('progressbar', { name: 'callfs-1' })).toHaveAttribute('aria-valuenow', '3');
		expect(screen.getByRole('progressbar', { name: 'callfs-2' })).toHaveAttribute('aria-valuenow', '3');
	});

	it('warns when one server holds more pieces than the spares', () => {
		render(PlacementPlanner, {
			props: plannerProps({ spare: 2, plan: plan({ max_on_one_instance: 3 }) })
		});
		expect(
			screen.getByText(t('findings.planner.concentrated_title', { pieces: 3, spare: 2 }))
		).toBeInTheDocument();
	});

	it('does not warn when the spares cover the largest server', () => {
		render(PlacementPlanner, {
			props: plannerProps({ spare: 3, plan: plan({ max_on_one_instance: 3 }) })
		});
		expect(
			screen.queryByText(t('findings.planner.concentrated_title', { pieces: 3, spare: 3 }))
		).toBeNull();
	});

	it('keeps the plan and adds the partial alert', () => {
		render(PlacementPlanner, { props: plannerProps({ status: 'partial', plan: plan() }) });
		expect(screen.getByText(t('findings.planner.partial'))).toBeInTheDocument();
		expect(screen.getByText(t('findings.planner.can_lose', { count: 1 }))).toBeInTheDocument();
	});

	it('hides the plan while loading', () => {
		render(PlacementPlanner, { props: plannerProps({ status: 'loading', plan: plan() }) });
		expect(screen.queryByText(t('findings.planner.can_lose', { count: 1 }))).toBeNull();
	});
});

describe('KpiStrip', () => {
	it('lists every tile in order under the key numbers name', () => {
		render(KpiStrip, {
			props: { tiles: [tile('Requests', 12408), tile('Speed', 120), tile('Storage', 48210), tile('Cluster', 3)] }
		});
		const list = screen.getByRole('list', { name: t('findings.kpi.label') });
		const items = screen.getAllByRole('listitem');
		expect(list).toBeInTheDocument();
		expect(items).toHaveLength(4);
		expect(items[0]).toHaveTextContent('Requests');
		expect(items[3]).toHaveTextContent('Cluster');
	});

	it('renders an empty list without tiles', () => {
		render(KpiStrip, { props: { tiles: [] } });
		expect(screen.getByRole('list', { name: t('findings.kpi.label') })).toBeInTheDocument();
		expect(screen.queryAllByRole('listitem')).toHaveLength(0);
	});
});

describe('FindingList', () => {
	it('names the region and shows the empty state without findings', () => {
		render(FindingList, { props: listProps({ findings: [] }) });
		expect(screen.getByRole('region', { name: t('findings.list.label') })).toBeInTheDocument();
		expect(screen.getByText(t('findings.list.empty_title'))).toBeInTheDocument();
	});

	it('shows each open finding as a level three heading with its severity word', () => {
		render(FindingList, { props: listProps() });
		expect(screen.getByRole('heading', { level: 3, name: 'Connections are not encrypted' })).toBeInTheDocument();
		expect(screen.getByText(t('status.finding.critical'))).toBeInTheDocument();
	});

	it('links each affected entity to its target', () => {
		render(FindingList, { props: listProps() });
		expect(screen.getByRole('link', { name: 'server.protocol' })).toHaveAttribute('href', '/settings/network');
	});

	it('runs the fix callback with the finding', () => {
		const onfix = vi.fn();
		const current = finding();
		render(FindingList, { props: listProps({ findings: [current], onfix }) });
		fireEvent.click(screen.getByRole('button', { name: `${t('findings.list.fix')}: ${current.title}` }));
		expect(onfix).toHaveBeenCalledWith(current);
	});

	it('mutes for the duration chosen in the menu', async () => {
		const onmute = vi.fn();
		const current = finding();
		render(FindingList, { props: listProps({ findings: [current], onmute }) });
		fireEvent.keyDown(screen.getByRole('button', { name: t('findings.list.mute') }), { key: 'Enter' });
		fireEvent.keyDown(await screen.findByRole('menuitem', { name: t('findings.mute.day') }), { key: 'Enter' });
		await waitFor(() => expect(onmute).toHaveBeenCalledWith(current, 86400));
	});

	it('hides muted findings until the muted section is opened', () => {
		const current = finding();
		const muted = mutedFinding();
		render(FindingList, { props: listProps({ findings: [current, muted] }) });
		const toggle = screen.getByRole('button', { name: t('findings.list.muted_show', { count: 1 }) });
		expect(toggle).toHaveAttribute('aria-expanded', 'false');
		expect(screen.queryByRole('heading', { level: 3, name: muted.title })).toBeNull();
		fireEvent.click(toggle);
		expect(toggle).toHaveAttribute('aria-expanded', 'true');
		expect(screen.getByRole('heading', { level: 3, name: muted.title })).toBeInTheDocument();
		expect(screen.getByText(t('status.finding.muted'))).toBeInTheDocument();
		expect(screen.getByText(t('findings.list.muted_by', { by: 'alice' }), { exact: false })).toBeInTheDocument();
	});

	it('unmutes a muted finding through its callback', () => {
		const onunmute = vi.fn();
		const muted = mutedFinding();
		render(FindingList, { props: listProps({ findings: [muted], onunmute }) });
		fireEvent.click(screen.getByRole('button', { name: t('findings.list.muted_show', { count: 1 }) }));
		fireEvent.click(screen.getByRole('button', { name: `${t('findings.list.unmute')}: ${muted.title}` }));
		expect(onunmute).toHaveBeenCalledWith(muted);
	});

	it('hides the ignore action when no ignore callback is given', () => {
		render(FindingList, { props: listProps() });
		expect(screen.queryByRole('button', { name: `${t('findings.list.ignore')}: Connections are not encrypted` })).toBeNull();
	});

	it('ignores a finding with the reason typed in the form', async () => {
		const onignore = vi.fn();
		const current = finding();
		render(FindingList, { props: listProps({ findings: [current], onignore }) });
		fireEvent.click(screen.getByRole('button', { name: `${t('findings.list.ignore')}: ${current.title}` }));
		await fireEvent.input(screen.getByLabelText(t('findings.list.ignore_reason'), { exact: false }), {
			target: { value: ' Internal network ' }
		});
		fireEvent.click(screen.getByRole('button', { name: t('findings.list.ignore_confirm') }));
		expect(onignore).toHaveBeenCalledWith(current, 'Internal network');
	});

	it('lists ignored findings in their own section and stops ignoring through its callback', () => {
		const onunignore = vi.fn();
		const ignoredFinding = finding({ ignored: { at: '2026-10-23T10:00:00Z', by: 'alice', reason: 'Internal network' } });
		render(FindingList, { props: listProps({ findings: [], ignored: [ignoredFinding], onunignore }) });
		expect(screen.queryByText(t('findings.list.empty_title'))).toBeNull();
		const toggle = screen.getByRole('button', { name: t('findings.list.ignored_show', { count: 1 }) });
		expect(toggle).toHaveAttribute('aria-expanded', 'false');
		fireEvent.click(toggle);
		expect(screen.getByText(t('status.finding.ignored'))).toBeInTheDocument();
		expect(screen.getByText(t('findings.list.ignored_reason', { reason: 'Internal network' }))).toBeInTheDocument();
		fireEvent.click(screen.getByRole('button', { name: `${t('findings.list.unignore')}: ${ignoredFinding.title}` }));
		expect(onunignore).toHaveBeenCalledWith(ignoredFinding);
	});

	it('keeps ignored findings out of the open list', () => {
		const ignoredFinding = finding({ ignored: { at: '2026-10-23T10:00:00Z', by: 'alice', reason: '' } });
		render(FindingList, { props: listProps({ findings: [ignoredFinding] }) });
		expect(screen.queryByRole('heading', { level: 3, name: ignoredFinding.title })).toBeNull();
		expect(screen.getByRole('button', { name: t('findings.list.ignored_show', { count: 1 }) })).toBeInTheDocument();
	});
});
