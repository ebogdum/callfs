<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { untrack, type ComponentProps } from 'svelte';
	import { cluster as loadCluster } from '$lib/api/admin/cluster';
	import { dataOf, series as loadSeries, type SeriesQuery } from '$lib/api/admin/telemetry';
	import { findings as loadFindings, ignore, ignoreUnsupported, mute, unignore, unmute } from '$lib/api/admin/findings';
	import { health as loadHealth } from '$lib/api/admin/health';
	import { cancel as cancelJob, jobs as loadJobs, retryFailed as retryJob } from '$lib/api/admin/jobs';
	import { status as loadStatus } from '$lib/api/admin/status';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { listLinks } from '$lib/api/links';
	import { usage as loadUsage } from '$lib/api/search';
	import { myEvents, myJobs } from '$lib/api/session';
	import type {
		AdminStatus,
		ClusterInfo,
		Event as EventRecord,
		Finding,
		Health,
		Job,
		Link as LinkRecord,
		Result,
		SeriesResult,
		Usage
	} from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatNumber } from '$lib/format/number';
	import { liveHub } from '$lib/live/hub.svelte';
	import { nodes } from '$lib/stores/nodes.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { QueryStatus, Series, StatusKey, UiError } from '$lib/ui/types';
	import StatTile from '../../molecules/StatTile.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import DashboardGrid, { type DashboardPanel } from '../../environments/DashboardGrid.svelte';
	import Button from '../../atoms/Button.svelte';
	import Link from '../../atoms/Link.svelte';
	import RelTime from '../../atoms/RelTime.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import StatusPill from '../../atoms/StatusPill.svelte';
	import Text from '../../atoms/Text.svelte';
	import BarChart from '../../organisms/chart/BarChart.svelte';
	import TimeSeriesChart from '../../organisms/chart/TimeSeriesChart.svelte';
	import FindingList from '../../organisms/FindingList.svelte';
	import JobList from '../../organisms/JobList.svelte';
	import KpiStrip from '../../organisms/KpiStrip.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import Icon from '../../quarks/Icon.svelte';
	import Spinner from '../../quarks/Spinner.svelte';
	import SetupChecklist, { type SetupItem } from './SetupChecklist.svelte';

	type StatTileProps = ComponentProps<typeof StatTile>;
	type Outcome<T> = Result<T, UiError> | null;
	type SeriesLine = SeriesResult['series'][number];
	type Points = [number, number | null][];
	type VerdictTone = 'ok' | 'warn' | 'danger' | 'info' | 'loading';

	const POLL_MS = 10_000;
	const HOUR_S = 3_600;
	const DAY_S = 86_400;
	const DAY_MS = DAY_S * 1_000;
	const RECENT_LIMIT = 5;
	const ATTENTION_LIMIT = 5;
	const CHECKLIST_KEY = 'overview.setup';
	const CLASSES = ['2xx', '3xx', '4xx', '5xx'] as const;
	const VERDICT_ICONS = { ok: 'ok', warn: 'warn', danger: 'danger', info: 'info' } as const;

	let healthOutcome = $state<Outcome<Health>>(null);
	let statusOutcome = $state<Outcome<AdminStatus>>(null);
	let clusterOutcome = $state<Outcome<ClusterInfo>>(null);
	let findingsOutcome = $state<Outcome<Finding[]>>(null);
	let ignoredFindings = $state<Finding[]>([]);
	let ignoreListed = $state(false);
	let ignoreRouteMissing = $state(false);
	let jobsOutcome = $state<Outcome<Job[]>>(null);
	let hourlyOutcome = $state<Outcome<SeriesResult>>(null);
	let trafficOutcome = $state<Outcome<SeriesResult>>(null);
	let speedOutcome = $state<Outcome<SeriesResult>>(null);
	let usageOutcome = $state<Outcome<Usage>>(null);
	let linksOutcome = $state<Outcome<LinkRecord[]>>(null);
	let memberJobsOutcome = $state<Outcome<Job[]>>(null);
	let eventsOutcome = $state<Outcome<EventRecord[]>>(null);
	let actionError = $state<UiError | null>(null);
	let updatedAt = $state<number | null>(null);
	let showAllFindings = $state(false);
	let findingsRegion: HTMLElement | undefined = $state();
	let panelOrder = $state<string[]>([]);
	let hiddenPanels = $state<string[]>([]);

	const viewer = $derived({ admin: session.isAdmin, userId: session.me?.user_id ?? null });

	const nowSeconds = (): number => Math.floor(Date.now() / 1000);

	function uiErrorOf(error: UiError | ApiFailure): UiError {
		return 'title' in error ? error : toUiError(error, { action: 'overview.load' });
	}

	function settle<T, U>(result: Result<T, UiError | ApiFailure>, pick: (value: T) => U): Result<U, UiError> {
		return true === result.ok ? { ok: true, value: pick(result.value) } : { ok: false, error: uiErrorOf(result.error) };
	}

	function valueOf<T>(outcome: Outcome<T>): T | null {
		return null !== outcome && true === outcome.ok ? outcome.value : null;
	}

	function errorOf<T>(outcome: Outcome<T>): UiError | null {
		return null !== outcome && false === outcome.ok ? outcome.error : null;
	}

	function statusOf<T>(outcome: Outcome<T>, isEmpty: boolean): QueryStatus {
		if (null === outcome) {
			return 'loading';
		}
		if (false === outcome.ok) {
			return 'error';
		}
		return isEmpty ? 'empty' : 'success';
	}

	async function loadSinceSampling(query: SeriesQuery & { from: number; to: number; stepS: number }): Promise<Result<SeriesResult, ApiFailure>> {
		const first = await loadSeries(query);
		if (false === first.ok || null === first.value.sampled_since) {
			return first;
		}
		const started = Math.ceil(Date.parse(first.value.sampled_since) / 1000);
		const hasLatest = first.value.series.some((line) => null !== (line.points.at(-1)?.[1] ?? null));
		if (hasLatest || started <= query.to - query.stepS || started >= query.to) {
			return first;
		}
		return loadSeries({ ...query, from: started, stepS: query.to - started });
	}

	function sumSeries(lines: SeriesLine[]): Points {
		const length = lines.reduce((max, line) => Math.max(max, line.points.length), 0);
		return Array.from({ length }, (_, index): [number, number] => {
			const stamp = lines.map((line) => line.points[index]?.[0]).find((value) => undefined !== value) ?? 0;
			const total = lines.reduce((sum, line) => sum + (line.points[index]?.[1] ?? 0), 0);
			return [stamp, total];
		});
	}

	function isFailureLine(line: SeriesLine): boolean {
		const first = (line.labels.status_code ?? '').charAt(0);
		return '4' === first || '5' === first;
	}

	function lastValue(points: Points): number | null {
		const last = points.at(-1);
		return undefined === last ? null : last[1];
	}

	function previousValue(points: Points): number | null {
		const previous = points.at(-2);
		return undefined === previous ? null : previous[1];
	}

	function classSeries(lines: SeriesLine[]): Series[] {
		return CLASSES.map((cls, index): Series => {
			const members = lines.filter((line) => cls.charAt(0) === (line.labels.status_code ?? '').charAt(0));
			return {
				id: cls,
				label: t(`overview.class.${cls}`),
				unit: 'per_s',
				points: sumSeries(members),
				colorIndex: index + 1
			};
		}).filter((series) => 0 < series.points.length);
	}

	function nowMinus(seconds: number): number {
		return nowSeconds() - seconds;
	}

	function mergeJobs(
		running: Result<{ items: Job[] }, ApiFailure>,
		failed: Result<{ items: Job[] }, ApiFailure>
	): Result<Job[], UiError> {
		if (false === running.ok) {
			return { ok: false, error: uiErrorOf(running.error) };
		}
		if (false === failed.ok) {
			return { ok: false, error: uiErrorOf(failed.error) };
		}
		return { ok: true, value: [...running.value.items, ...failed.value.items] };
	}

	async function loadAdmin(): Promise<void> {
		void nodes.load();
		const end = nowSeconds();
		const [health, status, cluster, findings, running, failed, usage, hourly, traffic, speed] = await Promise.all([
			loadHealth(),
			loadStatus(),
			loadCluster(),
			loadFindings(undefined, true),
			loadJobs({ state: ['running'] }),
			loadJobs({ state: ['failed'] }),
			loadUsage('/').then((response) => dataOf(response)),
			loadSinceSampling({
				name: 'callfs_http_requests_total',
				measure: 'total',
				by: ['status_code'],
				from: nowMinus(2 * HOUR_S),
				to: end,
				stepS: HOUR_S
			}),
			loadSeries({
				name: 'callfs_http_requests_total',
				measure: 'rate',
				by: ['status_code'],
				from: nowMinus(DAY_S),
				to: end,
				stepS: 900
			}),
			loadSinceSampling({
				name: 'callfs_http_request_duration_seconds',
				measure: 'p95',
				from: nowMinus(HOUR_S),
				to: end,
				stepS: HOUR_S
			})
		]);
		healthOutcome = settle(health, (value) => value);
		statusOutcome = settle(status, (value) => value);
		clusterOutcome = settle(cluster, (value) => value);
		findingsOutcome = settle(findings, (page) => page.items.filter((finding) => null === (finding.ignored ?? null)));
		if (true === findings.ok) {
			ignoreListed = Array.isArray(findings.value.ignored);
			ignoredFindings = [
				...(findings.value.ignored ?? []),
				...findings.value.items.filter((finding) => null !== (finding.ignored ?? null))
			];
		}
		jobsOutcome = mergeJobs(running, failed);
		usageOutcome = settle(usage, (value) => value);
		hourlyOutcome = settle(hourly, (value) => value);
		trafficOutcome = settle(traffic, (value) => value);
		speedOutcome = settle(speed, (value) => value);
	}

	async function loadMember(userId: string): Promise<void> {
		const [usage, links, jobs, events] = await Promise.all([
			loadUsage('/', { owner: userId }).then((response) => dataOf(response)),
			listLinks({ status: 'active', createdBy: userId, limit: 50 }, null),
			myJobs({ state: 'running' }),
			myEvents({ limit: RECENT_LIMIT })
		]);
		usageOutcome = settle(usage, (value) => value);
		linksOutcome = settle(links, (page) => page.items);
		memberJobsOutcome = settle(jobs, (page) => page.items);
		eventsOutcome = settle(events, (page) => page.items);
	}

	async function loadFor(current: { admin: boolean; userId: string | null }): Promise<void> {
		if (current.admin) {
			await loadAdmin();
		} else if (null !== current.userId) {
			await loadMember(current.userId);
		}
		updatedAt = Date.now();
	}

	function refreshNow(): void {
		void loadFor(viewer);
	}

	$effect(() => {
		const current = viewer;
		untrack(() => {
			void loadFor(current);
		});
		const timer = setInterval(() => {
			if (false === document.hidden) {
				void loadFor(current);
			}
		}, POLL_MS);
		return () => {
			clearInterval(timer);
		};
	});

	async function runAction<T>(pending: Promise<Result<T, UiError | ApiFailure>>): Promise<void> {
		const outcome = settle(await pending, (value) => value);
		actionError = false === outcome.ok ? outcome.error : null;
		refreshNow();
	}

	const health = $derived(valueOf(healthOutcome));
	const adminStatus = $derived(valueOf(statusOutcome));
	const cluster = $derived(valueOf(clusterOutcome));
	const findingList = $derived(valueOf(findingsOutcome));
	const jobList = $derived(valueOf(jobsOutcome));
	const hourly = $derived(valueOf(hourlyOutcome));
	const traffic = $derived(valueOf(trafficOutcome));
	const speed = $derived(valueOf(speedOutcome));
	const usage = $derived(valueOf(usageOutcome));
	const links = $derived(valueOf(linksOutcome));
	const memberJobs = $derived(valueOf(memberJobsOutcome));
	const events = $derived(valueOf(eventsOutcome));

	const adminFailure = $derived(errorOf(healthOutcome) ?? errorOf(clusterOutcome) ?? errorOf(findingsOutcome));
	const firstLoad = $derived(null === usageOutcome && null === hourlyOutcome && null === linksOutcome);
	const hasNoFiles = $derived(null !== usage && 0 === usage.files);

	const attention = $derived((findingList ?? []).filter((finding) => null === finding.muted && 'info' !== finding.severity));
	const mutedFindings = $derived((findingList ?? []).filter((finding) => null !== finding.muted));
	const topFinding = $derived(attention.find((finding) => 'critical' === finding.severity) ?? attention[0] ?? null);
	const visibleFindings = $derived(
		null === findingList
			? []
			: showAllFindings
				? findingList
				: [...attention.slice(0, ATTENTION_LIMIT), ...mutedFindings]
	);

	const noLeader = $derived(null !== cluster && 'raft' === cluster.mode && null === cluster.leader_id);
	const downComponent = $derived(health?.components.find((component) => 'down' === component.status) ?? null);

	const verdict = $derived.by((): { title: string; cause: string | null; tone: VerdictTone } => {
		if (false === viewer.admin) {
			const usageError = errorOf(usageOutcome);
			if (null === usageOutcome) {
				return { title: t('overview.verdict.loading'), cause: null, tone: 'loading' };
			}
			return null === usageError
				? { title: t('overview.verdict.member'), cause: t('overview.cause.member'), tone: 'ok' }
				: { title: t('overview.verdict.unknown'), cause: usageError.title, tone: 'info' };
		}
		if (noLeader) {
			return { title: t('overview.verdict.blocked'), cause: t('overview.cause.no_leader'), tone: 'danger' };
		}
		if (null !== downComponent) {
			return { title: t('overview.verdict.blocked'), cause: downComponent.detail, tone: 'danger' };
		}
		if (0 < attention.length && null !== topFinding) {
			return {
				title: t('overview.verdict.attention', { count: attention.length }),
				cause: topFinding.title,
				tone: 'critical' === topFinding.severity ? 'danger' : 'warn'
			};
		}
		if (null !== adminFailure) {
			return { title: t('overview.verdict.unknown'), cause: adminFailure.title, tone: 'info' };
		}
		if (null === health || null === findingList) {
			return { title: t('overview.verdict.loading'), cause: null, tone: 'loading' };
		}
		return { title: t('overview.verdict.ok'), cause: t('overview.cause.ok'), tone: 'ok' };
	});

	const CHECK_NAMES: Record<string, string> = {
		file_index: 'overview.check.file_index',
		locks: 'overview.check.locks',
		'storage.localfs': 'overview.check.localfs',
		'storage.s3': 'overview.check.s3',
		'storage.internal_proxy': 'overview.check.internal_proxy',
		raft: 'overview.check.raft',
		erasure: 'overview.check.erasure',
		tls: 'overview.check.tls',
		disk: 'overview.check.disk',
		replication: 'overview.check.replication',
		link_cleanup: 'overview.check.link_cleanup'
	};

	function checkName(name: string): string {
		const key = CHECK_NAMES[name];
		return undefined === key ? name : t(key);
	}

	const healthChecks = $derived(
		(health?.components ?? [])
			.filter((component) => 'disabled' !== component.status)
			.map((component) => ({
				name: component.name,
				label: checkName(component.name),
				detail: component.detail,
				status: `health.${component.status}` as StatusKey
			}))
	);
	const offChecks = $derived((health?.components ?? []).filter((component) => 'disabled' === component.status).length);

	function routeOf(finding: Finding): string {
		return null === finding.fix.setting_key ? finding.fix.route : `${finding.fix.route}#${finding.fix.setting_key}`;
	}

	function openFix(finding: Finding): void {
		void goto(`${base}${routeOf(finding)}`);
	}

	function uploadFiles(): void {
		void goto(`${base}/files`);
	}

	const primaryAction = $derived.by((): { label: string; run: () => void } | null => {
		if (null !== topFinding && viewer.admin) {
			const finding = topFinding;
			return { label: finding.fix.label, run: () => openFix(finding) };
		}
		return hasNoFiles ? null : { label: t('overview.upload'), run: uploadFiles };
	});

	function muteFinding(finding: Finding, durationS: number): void {
		void runAction(mute(finding.id, durationS, t('overview.mute.reason')));
	}

	function unmuteFinding(finding: Finding): void {
		void runAction(unmute(finding.id));
	}

	const ignoreAvailable = $derived(ignoreListed && false === ignoreRouteMissing);

	async function runIgnoreAction<T>(pending: Promise<Result<T, UiError>>): Promise<void> {
		const result = await pending;
		if (false === result.ok && ignoreUnsupported(result.error)) {
			ignoreRouteMissing = true;
			actionError = null;
			refreshNow();
			return;
		}
		await runAction(Promise.resolve(result));
	}

	function ignoreFinding(finding: Finding, reason: string): void {
		void runIgnoreAction(ignore(finding.id, reason));
	}

	function unignoreFinding(finding: Finding): void {
		void runIgnoreAction(unignore(finding.id));
	}

	function cancelRow(job: Job): void {
		void runAction(cancelJob(job.id));
	}

	function retryRow(job: Job): void {
		void runAction(retryJob(job.id));
	}

	function openJob(job: Job): void {
		void goto(`${base}/activity/jobs/${job.id}`);
	}

	const requestTotals = $derived(hourly ? sumSeries(hourly.series) : []);
	const failedTotals = $derived(hourly ? sumSeries(hourly.series.filter(isFailureLine)) : []);
	const trafficSeries = $derived(traffic ? classSeries(traffic.series) : []);
	const latencyMs = $derived.by((): number | null => {
		const points = speed?.series[0]?.points ?? [];
		const seconds = lastValue(points);
		return null === seconds ? null : Math.round(seconds * 1_000);
	});

	const healthyMembers = $derived(
		cluster ? cluster.members.filter((member) => !['unreachable', 'shutdown'].includes(member.state)).length : null
	);

	const quorumKey = $derived.by((): 'quorum.ok' | 'quorum.at_risk' | 'quorum.lost' | undefined => {
		if (null === cluster) {
			return undefined;
		}
		if (false === cluster.has_quorum) {
			return 'quorum.lost';
		}
		return 0 === cluster.can_lose ? 'quorum.at_risk' : 'quorum.ok';
	});

	const adminTiles = $derived.by((): StatTileProps[] => {
		const total = lastValue(requestTotals);
		const failed = lastValue(failedTotals);
		const previous = previousValue(requestTotals);
		const nodeTotal = nodes.items.length;
		const nodeTile: StatTileProps | null =
			0 === nodeTotal
				? null
				: {
						label: t('overview.kpi.cluster'),
						value: nodes.upCount,
						unit: 'count',
						base: 1 < nodeTotal ? { num: nodes.upCount, den: nodeTotal } : undefined,
						status: null === cluster || 'single' === cluster.mode ? undefined : quorumKey,
						help:
							1 === nodeTotal
								? t('overview.kpi.single_server')
								: t('overview.kpi.servers_up', { up: nodes.upCount, total: nodeTotal })
					};
		const memberTile: StatTileProps =
			null === cluster
				? { label: t('overview.kpi.cluster'), value: null, unit: 'count', help: errorOf(clusterOutcome)?.title }
				: 'single' === cluster.mode
					? {
							label: t('overview.kpi.cluster'),
							value: 1,
							unit: 'count',
							help: t('overview.kpi.single_server')
						}
					: {
							label: t('overview.kpi.cluster'),
							value: healthyMembers,
							unit: 'count',
							base: { num: healthyMembers ?? 0, den: cluster.voters },
							status: quorumKey
						};
		const clusterTile = nodeTile ?? memberTile;
		return [
			{
				label: t('overview.kpi.requests'),
				value: total,
				unit: 'count',
				base: null === total || null === failed ? undefined : { num: failed, den: total },
				delta:
					null === total || null === previous
						? undefined
						: { value: total - previous, goodWhen: 'none', period: t('overview.kpi.vs_prev_hour') },
				sparkline: sumSeries(traffic?.series ?? []).map((point) => point[1]),
				help: null === total ? (errorOf(hourlyOutcome)?.title ?? t('overview.kpi.no_data')) : undefined
			},
			{
				label: t('overview.kpi.speed'),
				value: latencyMs,
				unit: 'ms',
				help: null === latencyMs ? (errorOf(speedOutcome)?.title ?? t('overview.kpi.no_data')) : t('overview.kpi.speed_help')
			},
			{
				label: t('overview.kpi.storage'),
				value: usage?.bytes ?? null,
				unit: 'bytes',
				help: null === usage ? errorOf(usageOutcome)?.title : t('overview.kpi.storage_files', { count: formatNumber(usage.files) })
			},
			clusterTile
		];
	});

	const memberTiles = $derived.by((): StatTileProps[] => [
		{
			label: t('overview.kpi.my_files'),
			value: usage?.files ?? null,
			unit: 'count',
			help: null === usage ? errorOf(usageOutcome)?.title : t('overview.kpi.my_files_size', { size: formatBytes(usage.bytes).text }),
			loading: null === usageOutcome
		},
		{
			label: t('overview.kpi.my_links'),
			value: links ? links.length : null,
			unit: 'count',
			help: errorOf(linksOutcome)?.title,
			loading: null === linksOutcome
		},
		{
			label: t('overview.kpi.my_jobs'),
			value: memberJobs ? memberJobs.length : null,
			unit: 'count',
			help: errorOf(memberJobsOutcome)?.title,
			loading: null === memberJobsOutcome
		}
	]);

	const kpiTiles = $derived(viewer.admin ? adminTiles : memberTiles);

	const expiringToday = $derived(
		(links ?? []).filter((link) => Date.parse(link.expires_at) <= Date.now() + DAY_MS)
	);

	const findingIds = $derived(
		null === findingList ? null : new Set([...findingList, ...ignoredFindings].map((finding) => finding.id))
	);

	function absent(id: string): boolean {
		return null !== findingIds && false === findingIds.has(id);
	}

	const checklistItems: SetupItem[] = $derived([
		{
			id: 'https',
			label: t('overview.setup.https'),
			done: null !== adminStatus && 'https' === adminStatus.protocol_effective,
			href: `${base}/settings/network#server.protocol`
		},
		{
			id: 'secrets',
			label: t('overview.setup.secrets'),
			done: absent('secret.default'),
			href: `${base}/settings/security#auth.internal_proxy_secret`
		},
		{ id: 'keys', label: t('overview.setup.keys'), done: absent('keys.positional'), href: `${base}/settings/access` },
		{ id: 'admin', label: t('overview.setup.admin'), done: absent('admin.none'), href: `${base}/settings/access` },
		{
			id: 'metrics',
			label: t('overview.setup.metrics'),
			done: absent('metrics.public_port'),
			href: `${base}/settings/network#metrics.listen_addr`
		},
		{
			id: 'sslmode',
			label: t('overview.setup.sslmode'),
			done: absent('index.sslmode_disable'),
			href: `${base}/settings/index#metadata_store.dsn`
		},
		{ id: 'backup', label: t('overview.setup.backup'), done: false, href: `${base}/settings/index` }
	]);

	const checklistVisible = $derived(
		viewer.admin &&
			!preferences.value.dismissed.includes(CHECKLIST_KEY) &&
			checklistItems.some((item) => !item.done)
	);

	function dismissChecklist(): void {
		preferences.update({ dismissed: [...preferences.value.dismissed, CHECKLIST_KEY] });
	}

	const barData = $derived(
		usage
			? [
					{ label: t('overview.storage.localfs'), value: usage.by_storage.localfs?.bytes ?? 0, colorIndex: 1 },
					{ label: t('overview.storage.s3'), value: usage.by_storage.s3?.bytes ?? 0, colorIndex: 2 },
					{ label: t('overview.storage.split'), value: usage.erasure_raw_bytes, colorIndex: 3 }
				]
			: []
	);

	const adminPanels = $derived.by((): DashboardPanel[] => {
		const panels: DashboardPanel[] = [
			{ id: 'requests', title: t('overview.charts.requests'), span: 8, rows: 2, content: requestsPanel },
			{ id: 'storage', title: t('overview.storage.title'), span: 4, rows: 1, content: storagePanel },
			{ id: 'split', title: t('overview.charts.split'), span: 4, rows: 1, content: splitPanel },
			{ id: 'jobs', title: t('overview.jobs.title'), span: checklistVisible ? 4 : 12, rows: 1, content: jobsPanel }
		];
		return checklistVisible
			? [...panels.slice(0, 3), { id: 'setup', title: t('overview.setup.title'), span: 8, rows: 1, content: setupPanel }, panels[3]]
			: panels;
	});

	const memberPanels = $derived.by((): DashboardPanel[] => [
		{ id: 'recent', title: t('overview.member.recent'), span: 6, rows: 1, content: recentPanel },
		{ id: 'expiring', title: t('overview.member.expiring'), span: 6, rows: 1, content: expiringPanel }
	]);

	function handleKey(event: KeyboardEvent): void {
		if ('f' !== event.key || event.metaKey || event.ctrlKey || event.altKey) {
			return;
		}
		if (event.target instanceof HTMLElement && (event.target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(event.target.tagName))) {
			return;
		}
		findingsRegion?.querySelector('button')?.focus();
	}
</script>

<svelte:window onkeydown={handleKey} />

{#snippet regionLoading()}
	<Skeleton shape="rect" height="var(--space-16)" />
{/snippet}

{#snippet requestsPanel()}
	<TimeSeriesChart
		series={trafficSeries}
		unit="per_s"
		stacked
		status={statusOf(trafficOutcome, 0 === trafficSeries.length)}
		error={errorOf(trafficOutcome) ?? undefined}
		emptyReason="no_samples"
		height="lg"
		onretry={refreshNow}
	/>
{/snippet}

{#snippet storagePanel()}
	{#if null !== usage}
		<BarChart data={barData} unit="bytes" label={t('overview.storage.label')} />
		<div class="panel-foot">
			<Link href={`${base}/cluster/storage`}>{t('overview.storage.open')}</Link>
		</div>
	{:else if null !== usageOutcome && false === usageOutcome.ok}
		<ErrorState error={usageOutcome.error} onretry={refreshNow} variant="region" />
	{:else}
		{@render regionLoading()}
	{/if}
{/snippet}

{#snippet splitPanel()}
	{#if null !== usage}
		<div class="figure">
			<span class="figure-value">{formatNumber(usage.erasure_files)}</span>
			<Text tone="muted">{t('overview.charts.split_unit', { count: usage.erasure_files })}</Text>
		</div>
		<Text tone="muted" size="sm">{t('overview.charts.split_body')}</Text>
		<div class="panel-foot">
			<Link href={`${base}/cluster/durability`}>{t('overview.charts.split_open')}</Link>
		</div>
	{:else if null !== usageOutcome && false === usageOutcome.ok}
		<ErrorState error={usageOutcome.error} onretry={refreshNow} variant="region" />
	{:else}
		{@render regionLoading()}
	{/if}
{/snippet}

{#snippet jobsPanel()}
	{#if null !== jobList}
		{#if 0 === jobList.length}
			<div class="quiet">
				<Icon name="ok" size="md" />
				<div class="quiet-text">
					<Text weight={500}>{t('overview.jobs.empty_title')}</Text>
					<Text tone="muted" size="sm">{t('overview.jobs.empty_body')}</Text>
				</div>
			</div>
		{:else}
			<JobList
				jobs={jobList.slice(0, ATTENTION_LIMIT)}
				onopen={openJob}
				oncancel={cancelRow}
				onretryfailed={retryRow}
			/>
		{/if}
		<div class="panel-foot">
			<Link href={`${base}/activity/jobs`}>{t('overview.jobs.open')}</Link>
		</div>
	{:else if null !== jobsOutcome && false === jobsOutcome.ok}
		<ErrorState error={jobsOutcome.error} onretry={refreshNow} variant="region" />
	{:else}
		{@render regionLoading()}
	{/if}
{/snippet}

{#snippet setupPanel()}
	<SetupChecklist items={checklistItems} ondismiss={dismissChecklist} />
{/snippet}

{#snippet recentPanel()}
	{#if null !== events}
		{#if 0 === events.length}
			<Text tone="muted">{t('overview.member.none_recent')}</Text>
		{:else}
			<ul class="rows">
				{#each events as event (event.id)}
					<li class="row">
						<Text truncate>{event.message}</Text>
						<Text tone="muted" size="sm"><RelTime at={event.at} /></Text>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if null !== eventsOutcome && false === eventsOutcome.ok}
		<ErrorState error={eventsOutcome.error} onretry={refreshNow} variant="region" />
	{:else}
		<Skeleton shape="text" lines={3} />
	{/if}
{/snippet}

{#snippet expiringPanel()}
	{#if null !== links}
		{#if 0 === expiringToday.length}
			<Text tone="muted">{t('overview.member.none_links')}</Text>
		{:else}
			<ul class="rows">
				{#each expiringToday as link (link.id)}
					<li class="row">
						<Link href={`${base}/sharing/${link.id}`}>{link.file_path}</Link>
						<Text tone="muted" size="sm"><RelTime at={link.expires_at} /></Text>
					</li>
				{/each}
			</ul>
		{/if}
	{:else if null !== linksOutcome && false === linksOutcome.ok}
		<ErrorState error={linksOutcome.error} onretry={refreshNow} variant="region" />
	{:else}
		<Skeleton shape="text" lines={3} />
	{/if}
{/snippet}

<PageFrame title={t('overview.title')} job={t('overview.job')}>
	{#snippet primary()}
		{#if null !== primaryAction}
			<Button variant="primary" iconStart={null !== topFinding && viewer.admin ? 'fix' : 'upload'} onclick={primaryAction.run}>
				{primaryAction.label}
			</Button>
		{/if}
	{/snippet}

	<div class="overview">
		<section class="health" data-tone={verdict.tone} aria-labelledby="overview-verdict" aria-live="polite">
			<div class="health-head">
				<span class="health-icon" aria-hidden="true">
					{#if 'loading' === verdict.tone}
						<Spinner size="md" />
					{:else}
						<Icon name={VERDICT_ICONS[verdict.tone]} size="lg" />
					{/if}
				</span>
				<div class="health-text">
					<h2 class="health-title" id="overview-verdict">{verdict.title}</h2>
					{#if null !== verdict.cause}
						<Text tone="muted">{verdict.cause}</Text>
					{/if}
				</div>
				<div class="health-meta">
					<StaleIndicator
						updatedAt={updatedAt}
						intervalMs={POLL_MS}
						state={liveHub.connection.pageState}
						onrefresh={refreshNow}
					/>
				</div>
			</div>

			{#if 0 < healthChecks.length}
				<ul class="checks" aria-label={t('overview.health.checks')}>
					{#each healthChecks as check (check.name)}
						<li>
							<StatusPill status={check.status} label={check.label} detail={check.detail} size="sm" />
						</li>
					{/each}
					{#if 0 < offChecks}
						<li class="checks-off">
							<Text tone="muted" size="sm">{t('overview.health.off', { count: offChecks })}</Text>
						</li>
					{/if}
				</ul>
			{/if}

			{#if viewer.admin && (null !== actionError || 0 < visibleFindings.length || 0 < ignoredFindings.length || ATTENTION_LIMIT < attention.length)}
				<div class="health-body" bind:this={findingsRegion} role="region" aria-label={t('overview.findings.label')}>
					{#if null !== actionError}
						<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
					{/if}
					{#if 0 < visibleFindings.length || 0 < ignoredFindings.length}
						<FindingList
							findings={visibleFindings}
							ignored={ignoredFindings}
							onfix={openFix}
							onmute={muteFinding}
							onunmute={unmuteFinding}
							onignore={ignoreAvailable ? ignoreFinding : undefined}
							onunignore={ignoreAvailable ? unignoreFinding : undefined}
						/>
					{/if}
					{#if ATTENTION_LIMIT < attention.length}
						<div>
							<Button variant="ghost" size="sm" onclick={() => (showAllFindings = !showAllFindings)}>
								{showAllFindings
									? t('overview.findings.show_fewer')
									: t('overview.findings.view_all', { count: attention.length })}
							</Button>
						</div>
					{/if}
				</div>
			{/if}
		</section>

		{#if hasNoFiles}
			<section class="start" aria-labelledby="overview-start">
				<span class="start-icon" aria-hidden="true"><Icon name="upload" size="lg" /></span>
				<div class="start-text">
					<h2 class="start-title" id="overview-start">{t('overview.empty.title')}</h2>
					<Text tone="muted">{t('overview.empty.body')}</Text>
				</div>
				<Button variant="primary" iconStart="upload" onclick={uploadFiles}>{t('overview.upload')}</Button>
			</section>
		{/if}

		<section aria-label={t('overview.regions.kpis')}>
			{#if firstLoad}
				<div class="kpi-skeleton">
					{#each [0, 1, 2, 3] as index (index)}
						<Skeleton shape="rect" height="calc(var(--space-16) * 2)" />
					{/each}
				</div>
			{:else}
				<KpiStrip tiles={kpiTiles} />
			{/if}
		</section>

		<DashboardGrid
			panels={viewer.admin ? adminPanels : memberPanels}
			bind:order={panelOrder}
			bind:hidden={hiddenPanels}
		/>
	</div>
</PageFrame>

<style>
	.overview {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.health {
		--verdict: var(--text-muted);
		--verdict-soft: var(--surface-sunken);
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		box-shadow: var(--shadow-xs);
		overflow: hidden;
	}

	.health[data-tone='ok'] {
		--verdict: var(--ok-text);
		--verdict-soft: var(--ok-soft);
	}

	.health[data-tone='warn'] {
		--verdict: var(--warn-text);
		--verdict-soft: var(--warn-soft);
	}

	.health[data-tone='danger'] {
		--verdict: var(--danger-text);
		--verdict-soft: var(--danger-soft);
	}

	.health[data-tone='info'] {
		--verdict: var(--info-text);
		--verdict-soft: var(--info-soft);
	}

	.health-head {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: start;
		gap: var(--space-4);
		padding: var(--card-pad);
	}

	.health-icon,
	.start-icon {
		display: grid;
		place-items: center;
		inline-size: var(--space-12);
		block-size: var(--space-12);
		border-radius: var(--radius-md);
		background: var(--verdict-soft);
		color: var(--verdict);
	}

	.health-text,
	.start-text {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.health-title,
	.start-title {
		margin: 0;
		font-size: var(--text-title-size);
		line-height: var(--text-title-line);
		font-weight: var(--text-title-weight);
		letter-spacing: var(--text-title-tracking);
		color: var(--text);
		text-wrap: balance;
	}

	.health-meta {
		grid-column: 1 / -1;
		display: flex;
		justify-content: flex-start;
	}

	.checks {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin: 0;
		padding-block: var(--space-3);
		padding-inline: var(--card-pad);
		list-style: none;
		border-block-start: var(--hairline) solid var(--border-subtle);
		background: var(--bg-subtle);
	}

	.checks-off {
		display: flex;
		align-items: center;
	}

	.health-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--card-pad);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.start {
		--verdict: var(--accent-text);
		--verdict-soft: var(--accent-soft);
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: center;
		gap: var(--space-4);
		padding: var(--card-pad);
		border: var(--hairline) dashed var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface);
	}

	.start > :global(:last-child) {
		grid-column: 1 / -1;
		justify-self: start;
	}

	.kpi-skeleton {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 14)), 1fr));
		gap: var(--space-4);
	}

	.figure {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.figure-value {
		font-size: var(--text-display-size);
		line-height: var(--text-display-line);
		font-weight: var(--text-display-weight);
		letter-spacing: var(--text-display-tracking);
		font-variant-numeric: tabular-nums;
		color: var(--text);
	}

	.panel-foot {
		display: flex;
		padding-block-start: var(--space-3);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.quiet {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		color: var(--ok-text);
	}

	.quiet-text {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		color: var(--text);
	}

	.rows {
		display: flex;
		flex-direction: column;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--space-3);
		padding-block: var(--space-2);
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.row:last-child {
		border-block-end: 0;
	}

	@media (min-width: 48rem) {
		.health-head {
			grid-template-columns: auto minmax(0, 1fr) auto;
			align-items: center;
		}

		.health-meta {
			grid-column: auto;
			justify-content: flex-end;
		}

		.start {
			grid-template-columns: auto minmax(0, 1fr) auto;
		}

		.start > :global(:last-child) {
			grid-column: auto;
		}
	}
</style>
