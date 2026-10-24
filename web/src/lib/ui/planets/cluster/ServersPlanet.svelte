<script lang="ts">
	import { findings as listFindings } from '$lib/api/admin/findings';
	import { instances, probeInstance, removeEndpoint, setEndpoint } from '$lib/api/admin/topology';
	import { roleOf, type NodeInfo, type NodeRole } from '$lib/api/nodes';
	import type { Finding, ServerInfo } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatDuration } from '$lib/format/duration';
	import { formatBytes } from '$lib/format/bytes';
	import { DASH } from '$lib/format/number';
	import type { StatusKey } from '$lib/status/catalog';
	import { controlNode, nodes } from '$lib/stores/nodes.svelte';
	import Badge from '$lib/ui/atoms/Badge.svelte';
	import Bytes from '$lib/ui/atoms/Bytes.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import Num from '$lib/ui/atoms/Num.svelte';
	import StatusPill from '$lib/ui/atoms/StatusPill.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import FormField from '$lib/ui/molecules/FormField.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import ApiEquivalent from '$lib/ui/organisms/ApiEquivalent.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import type { ColumnDef, MenuItem, QueryStatus, SortState } from '$lib/ui/types';

	interface ServerRow {
		id: string;
		endpoint: string;
		self: boolean;
		source: ServerInfo['source'] | null;
		secure: boolean;
		up: boolean | null;
		latencyMs: number | null;
		role: NodeRole;
		version: string | null;
		uptimeS: number | null;
		files: number | null;
		bytes: number | null;
		pieces: number | null;
		node: NodeInfo | null;
		instance: ServerInfo | null;
	}

	const SERVER_REFRESH_MS = 5000;
	const NODE_REFRESH_MS = 10000;
	const FINDINGS_REFRESH_MS = 30000;
	const CONNECTION_MIN_PX = 660;
	const VERSION_MIN_PX = 760;
	const DATA_MIN_PX = 855;
	const PIECES_MIN_PX = 935;
	const DISK_MIN_PX = 860;
	const WIDE_MIN_PX = 920;

	const serversQuery = createQuery({
		key: ['instances'],
		fetch: () => instances(),
		refetchMs: SERVER_REFRESH_MS,
		isEmpty: (result) => result.ok && 0 === result.value.items.length,
		isPartial: (result) => result.ok && result.value.items.some((server) => false === server.reachable)
	});

	const findingsQuery = createQuery({
		key: ['findings', 'cluster'],
		fetch: () => listFindings('cluster', false),
		refetchMs: FINDINGS_REFRESH_MS
	});

	const probeMutation = createMutation({
		run: (instanceId: string) => probeInstance(instanceId),
		invalidate: () => [['instances'], ['health']],
		success: () => t('servers-storage.action.checked'),
		context: { action: 'servers-storage.action.check' }
	});

	const saveMutation = createMutation({
		run: (vars: { id: string; endpoint: string }) => setEndpoint(vars.id, vars.endpoint, true),
		invalidate: () => [['instances'], ['health']],
		success: () => t('servers-storage.action.saved'),
		context: { action: 'servers-storage.action.change' }
	});

	const removeMutation = createMutation({
		run: (instanceId: string) => removeEndpoint(instanceId),
		invalidate: () => [['instances'], ['health']],
		success: () => t('servers-storage.action.removed'),
		context: { action: 'servers-storage.action.remove' }
	});

	$effect(() => {
		void nodes.load();
		const timer = setInterval(() => {
			if (false === document.hidden) {
				void nodes.load();
			}
		}, NODE_REFRESH_MS);
		const onVisible = (): void => {
			if (false === document.hidden) {
				void nodes.load();
			}
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	const servers = $derived(true === serversQuery.data?.ok ? serversQuery.data.value.items : []);
	const findingItems = $derived(true === findingsQuery.data?.ok ? findingsQuery.data.value.items : []);
	const mutationError = $derived(probeMutation.error ?? saveMutation.error ?? removeMutation.error ?? null);
	const staleState = $derived('error' === serversQuery.status && null !== nodes.error ? 'offline' : 'live');

	const sourceKeys: Record<ServerInfo['source'], string> = {
		config: 'servers-storage.source.config',
		joined: 'servers-storage.source.joined',
		self: 'servers-storage.source.self'
	};

	function instanceUp(server: ServerInfo): boolean | null {
		if (true === server.reachable || (server.is_self && null === server.reachable)) {
			return true;
		}
		return server.reachable;
	}

	function rowFromNode(node: NodeInfo, instance: ServerInfo | null, all: readonly NodeInfo[]): ServerRow {
		const up = 'up' === node.status;
		return {
			id: node.id,
			endpoint: node.endpoint,
			self: node.self,
			source: instance?.source ?? null,
			secure: null === instance ? node.endpoint.startsWith('https://') : 'https' === instance.scheme,
			up,
			latencyMs: up && false === node.self ? node.latency_ms : null,
			role: roleOf(node, all),
			version: node.version ?? null,
			uptimeS: node.uptime_s ?? null,
			files: instance?.files ?? null,
			bytes: instance?.bytes ?? null,
			pieces: instance?.pieces ?? null,
			node,
			instance
		};
	}

	function rowFromInstance(server: ServerInfo): ServerRow {
		return {
			id: server.instance_id,
			endpoint: server.endpoint,
			self: server.is_self,
			source: server.source,
			secure: 'https' === server.scheme,
			up: instanceUp(server),
			latencyMs: server.rtt_ms,
			role: 'none',
			version: null,
			uptimeS: null,
			files: server.files,
			bytes: server.bytes,
			pieces: server.pieces,
			node: null,
			instance: server
		};
	}

	const rows = $derived.by((): ServerRow[] => {
		const byId = new Map(servers.map((server) => [server.instance_id, server]));
		const known = new Set(nodes.items.map((node) => node.id));
		return [
			...nodes.items.map((node) => rowFromNode(node, byId.get(node.id) ?? null, nodes.items)),
			...servers.filter((server) => false === known.has(server.instance_id)).map((server) => rowFromInstance(server))
		];
	});

	let sort = $state<SortState | null>(null);
	let tableWidth = $state(0);

	function sortValue(row: ServerRow, key: string): string | number {
		const value = (row as unknown as Record<string, unknown>)[key];
		if ('number' === typeof value || 'string' === typeof value) {
			return value;
		}
		if ('boolean' === typeof value) {
			return value ? 1 : 0;
		}
		return '';
	}

	const sortedRows = $derived.by(() => {
		const current = sort;
		if (null === current) {
			return rows;
		}
		const direction = 'asc' === current.dir ? 1 : -1;
		return [...rows].sort((left, right) => {
			const a = sortValue(left, current.key);
			const b = sortValue(right, current.key);
			if (a === b) {
				return 0;
			}
			return (a < b ? -1 : 1) * direction;
		});
	});

	const tableStatus = $derived.by((): QueryStatus => {
		if (0 < rows.length) {
			return rows.some((row) => false === row.up) ? 'partial' : 'success';
		}
		if ('loading' === serversQuery.status) {
			return 'loading';
		}
		if ('error' === serversQuery.status && null !== nodes.error) {
			return 'error';
		}
		return 'empty';
	});
	const upCount = $derived(rows.filter((row) => true === row.up).length);

	let formOpen = $state(false);
	let formMode = $state<'add' | 'change'>('add');
	let formId = $state('');
	let formEndpoint = $state('');
	let removeOpen = $state(false);
	let removeTarget = $state<ServerInfo | null>(null);

	const isHttpUrl = (value: string) =>
		URL.canParse(value) && (value.startsWith('https://') || value.startsWith('http://'));

	const endpointValue = $derived(formEndpoint.trim());
	const endpointInvalid = $derived(0 < endpointValue.length && !isHttpUrl(endpointValue));
	const submitReason = $derived.by(() => {
		if ('add' === formMode && 0 === formId.trim().length) {
			return t('servers-storage.form.invalid_id');
		}
		if (!isHttpUrl(endpointValue)) {
			return t('servers-storage.form.invalid_url');
		}
		return undefined;
	});

	const requestPreview = $derived({
		method: 'PUT' as const,
		url: `/v1/admin/instances/${encodeURIComponent(formId.trim())}/endpoint`,
		body: { endpoint: endpointValue, persist: true }
	});

	const statusOf = (row: ServerRow): StatusKey => {
		if (true === row.up) {
			return 'server.up';
		}
		return false === row.up ? 'server.down' : 'server.unknown';
	};

	const statusDetail = (row: ServerRow): string => {
		const parts: string[] = [];
		if ('none' !== row.role) {
			parts.push(t(`nodes.role.${row.role}`));
		}
		if (null !== row.latencyMs) {
			parts.push(t('nodes.picker.latency', { ms: row.latencyMs }));
		}
		return 0 === parts.length ? '' : parts.join(' · ');
	};

	const isActive = (row: ServerRow): boolean =>
		null === row.node ? row.self && null === nodes.selected : nodes.isActive(row.node);

	const toneFor = (severity: Finding['severity']) => {
		if ('critical' === severity) {
			return 'danger';
		}
		if ('warning' === severity) {
			return 'warn';
		}
		return 'info';
	};

	const refreshAll = async () => {
		await Promise.all([serversQuery.refetch(), nodes.load()]);
	};

	const openAdd = () => {
		formMode = 'add';
		formId = '';
		formEndpoint = '';
		formOpen = true;
	};

	const openChange = (server: ServerInfo) => {
		formMode = 'change';
		formId = server.instance_id;
		formEndpoint = server.endpoint;
		formOpen = true;
	};

	const submitForm = async () => {
		if (undefined !== submitReason) {
			return;
		}
		const saved = await saveMutation.run({ id: formId.trim(), endpoint: endpointValue });
		if (null !== saved) {
			formOpen = false;
		}
	};

	const confirmRemove = async () => {
		if (null === removeTarget) {
			return;
		}
		const removed = await removeMutation.run(removeTarget.instance_id);
		if (null !== removed) {
			removeOpen = false;
			removeTarget = null;
		}
	};

	const rowMenu = (row: ServerRow): MenuItem[] => {
		const items: MenuItem[] = [];
		const node = row.node;
		if (null !== node && false === isActive(row)) {
			items.push({
				id: 'control',
				label: t('nodes.servers.control'),
				icon: 'arrow-right',
				disabledReason: 'up' === node.status ? undefined : t('nodes.command.down'),
				onselect: () => controlNode(node.id)
			});
		}
		const server = row.instance;
		if (null === server) {
			return items;
		}
		items.push({
			id: 'check',
			label: t('servers-storage.action.check'),
			onselect: () => {
				void probeMutation.run(server.instance_id);
			}
		});
		if (true === server.is_self) {
			return items;
		}
		items.push(
			{
				id: 'change',
				label: t('servers-storage.action.change'),
				onselect: () => openChange(server)
			},
			{
				id: 'remove',
				label: t('servers-storage.action.remove'),
				tone: 'danger',
				onselect: () => {
					removeTarget = server;
					removeOpen = true;
				}
			}
		);
		return items;
	};

	const noPieces = $derived(rows.every((row) => null === row.pieces || 0 === row.pieces));
	const fits = (minPx: number): boolean => 0 === tableWidth || minPx <= tableWidth;

	const allColumns: ColumnDef<ServerRow>[] = $derived([
		{ id: 'server', header: t('nodes.servers.column.server'), cell: serverCell, sortKey: 'id', width: fits(WIDE_MIN_PX) ? 260 : 220, priority: 1 },
		{ id: 'status', header: t('nodes.servers.column.status'), cell: statusCell, sortKey: 'up', width: 140, priority: 1 },
		{
			id: 'connection',
			header: t('nodes.servers.column.connection'),
			cell: connectionCell,
			sortKey: 'secure',
			width: 115,
			priority: 1,
			hidden: false === fits(CONNECTION_MIN_PX)
		},
		{
			id: 'version',
			header: t('nodes.servers.column.version'),
			cell: versionCell,
			sortKey: 'version',
			width: 100,
			hidden: false === fits(VERSION_MIN_PX)
		},
		{
			id: 'data',
			header: t('nodes.servers.column.data'),
			cell: dataCell,
			align: 'end',
			sortKey: 'bytes',
			width: 95,
			hidden: false === fits(DATA_MIN_PX)
		},
		{
			id: 'pieces',
			header: t('servers-storage.servers.column.pieces'),
			cell: piecesCell,
			align: 'end',
			sortKey: 'pieces',
			width: 80,
			hidden: noPieces || false === fits(PIECES_MIN_PX)
		},
		{
			id: 'disk',
			header: t('nodes.servers.column.disk'),
			cell: diskCell,
			align: 'end',
			width: 120,
			hidden: false === fits(DISK_MIN_PX)
		},
		{ id: 'control', header: t('nodes.servers.column.action'), cell: controlCell, width: 110, priority: 1 }
	]);
	const columns = $derived(allColumns.filter((column) => true !== column.hidden));
	const connectionInline = $derived(false === fits(CONNECTION_MIN_PX));
</script>

{#snippet serverCell(row: ServerRow)}
	<span class="stacked">
		<Cluster gap="1" align="center">
			<span class="mono strong">{row.id}</span>
			{#if row.self}
				<span class="fixed"><Badge tone="neutral" variant="soft">{t('nodes.picker.this_server')}</Badge></span>
			{:else if null !== row.source}
				<span class="sub">{t(sourceKeys[row.source])}</span>
			{/if}
		</Cluster>
		<span class="sub mono wrapped" title={row.endpoint}>{row.endpoint}</span>
		{#if connectionInline && false === row.secure}
			<span class="fixed"><StatusPill status="tls.off" size="sm" /></span>
		{/if}
	</span>
{/snippet}

{#snippet statusCell(row: ServerRow)}
	{@const detail = statusDetail(row)}
	<span class="stacked">
		<span class="fixed"><StatusPill status={statusOf(row)} detail={row.node?.detail} size="sm" /></span>
		{#if '' !== detail}
			<span class="sub">{detail}</span>
		{/if}
	</span>
{/snippet}

{#snippet connectionCell(row: ServerRow)}
	<span class="fixed"><StatusPill status={row.secure ? 'tls.on' : 'tls.off'} size="sm" /></span>
{/snippet}

{#snippet versionCell(row: ServerRow)}
	<span class="stacked">
		<span class="mono">{row.version ?? DASH}</span>
		{#if null !== row.uptimeS}
			<span class="sub">{t('nodes.servers.up_for', { time: formatDuration(row.uptimeS * 1000, 'short') })}</span>
		{/if}
	</span>
{/snippet}

{#snippet dataCell(row: ServerRow)}
	{#if null === row.bytes || null === row.files}
		<span class="sub">{DASH}</span>
	{:else}
		<span class="stacked end">
			<Bytes value={row.bytes} />
			<span class="sub">{t('nodes.servers.files', { count: row.files })}</span>
		</span>
	{/if}
{/snippet}

{#snippet diskCell(row: ServerRow)}
	{#if null === row.instance || null === row.instance.disk_free_bytes || null === row.instance.disk_total_bytes}
		<span class="sub">{DASH}</span>
	{:else}
		<span class="stacked end">
			<Bytes value={row.instance.disk_free_bytes} />
			<span class="sub">{t('nodes.servers.disk_of', { total: formatBytes(row.instance.disk_total_bytes).text })}</span>
		</span>
	{/if}
{/snippet}

{#snippet piecesCell(row: ServerRow)}
	{#if null === row.pieces}
		<span class="sub">{DASH}</span>
	{:else}
		<Num value={row.pieces} />
	{/if}
{/snippet}

{#snippet controlCell(row: ServerRow)}
	{#if isActive(row)}
		<span class="fixed"><Badge tone="accent" variant="soft">{t('nodes.servers.active')}</Badge></span>
	{:else if null !== row.node}
		{@const target = row.node.id}
		<Button
			variant="secondary"
			size="sm"
			aria-label={t('nodes.servers.control_named', { node: target })}
			disabledReason={true === row.up ? undefined : t('nodes.command.down')}
			onclick={() => controlNode(target)}
		>
			{t('nodes.servers.control_short')}
		</Button>
	{:else}
		<span class="sub">{DASH}</span>
	{/if}
{/snippet}

<PageFrame title={t('servers-storage.servers.title')} job={t('nodes.servers.job')}>
	{#snippet primary()}
		<Button variant="primary" iconStart="plus" onclick={openAdd}>{t('servers-storage.servers.add')}</Button>
	{/snippet}
	{#snippet status()}
		<StaleIndicator
			updatedAt={serversQuery.updatedAt ?? nodes.updatedAt}
			intervalMs={SERVER_REFRESH_MS}
			state={staleState}
			onrefresh={refreshAll}
		/>
	{/snippet}

	<Stack gap="4">
		{#if 'partial' === tableStatus}
			<InlineAlert tone="warn" title={t('servers-storage.servers.partial')} />
		{/if}
		{#if null !== mutationError}
			<InlineAlert tone="danger" title={mutationError.title} body={mutationError.body} />
		{/if}
		{#each findingItems as finding (finding.id)}
			<InlineAlert tone={toneFor(finding.severity)} title={finding.title} body={finding.body} />
		{/each}
		{#if 0 < rows.length}
			<Text size="sm" tone="muted">{t('nodes.servers.summary', { up: upCount, total: rows.length })}</Text>
		{/if}
		<div class="table-fit" bind:clientWidth={tableWidth}>
			<DataTable
				rows={sortedRows}
				{columns}
				rowKey={(row) => row.id}
				label={t('nodes.servers.table_label')}
				status={tableStatus}
				error={serversQuery.error ?? nodes.error ?? undefined}
				onretry={() => void refreshAll()}
				bind:sort
				{rowMenu}
				controls={false}
			>
				{#snippet empty()}
					<EmptyState
						variant="first-use"
						icon="server"
						title={t('servers-storage.servers.empty.title')}
						body={t('servers-storage.servers.empty.body')}
						primary={{ label: t('servers-storage.servers.add'), onclick: openAdd }}
					/>
				{/snippet}
			</DataTable>
		</div>
	</Stack>
</PageFrame>

<Dialog
	bind:open={formOpen}
	title={'add' === formMode ? t('servers-storage.form.add.title') : t('servers-storage.form.change.title')}
	description={'add' === formMode
		? t('servers-storage.form.add.description')
		: t('servers-storage.form.change.description', { id: formId })}
	size="md"
>
	<Stack gap="4">
		{#if 'add' === formMode}
			<FormField label={t('servers-storage.form.server_id.label')} for="servers-form-id" help={t('servers-storage.form.server_id.help')}>
				{#snippet control({ id, describedBy, invalid })}
					<Input id={id} aria-describedby={describedBy} {invalid} bind:value={formId} />
				{/snippet}
			</FormField>
		{/if}
		<FormField
			label={t('servers-storage.form.address.label')}
			for="servers-form-address"
			help={t('servers-storage.form.address.help')}
			example={t('servers-storage.form.address.example')}
			error={endpointInvalid ? t('servers-storage.form.address.invalid') : undefined}
		>
			{#snippet control({ id, describedBy, invalid })}
				<Input id={id} type="url" mono aria-describedby={describedBy} {invalid} bind:value={formEndpoint} />
			{/snippet}
		</FormField>
		<ApiEquivalent request={requestPreview} />
	</Stack>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (formOpen = false)}>{t('common.cancel')}</Button>
			<Button
				variant="primary"
				disabledReason={submitReason}
				loading={saveMutation.pending}
				onclick={submitForm}
			>
				{'add' === formMode ? t('servers-storage.form.submit.add') : t('servers-storage.form.submit.change')}
			</Button>
		</Cluster>
	{/snippet}
</Dialog>

<AlertDialog
	bind:open={removeOpen}
	title={t('servers-storage.remove.title')}
	body={t('servers-storage.remove.body')}
	confirmLabel={t('servers-storage.remove.confirm')}
	tone="danger"
	onconfirm={confirmRemove}
/>

<style>
	.table-fit {
		min-inline-size: 0;
	}

	.fixed {
		display: inline-flex;
		flex: none;
		white-space: nowrap;
	}

	.stacked {
		display: grid;
		justify-items: start;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.stacked.end {
		justify-items: end;
	}

	.sub {
		font-size: var(--text-caption-size);
		color: var(--text-muted);
		white-space: nowrap;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		white-space: nowrap;
	}

	.sub.mono {
		font-size: var(--text-caption-size);
	}

	.wrapped {
		white-space: nowrap;
	}

	.strong {
		font-weight: 600;
	}
</style>
