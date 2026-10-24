<script lang="ts">
	import { audit, logs } from '$lib/api/admin/telemetry';
	import type { ApiFailure } from '$lib/api/errors';
	import { nodePath } from '$lib/api/http';
	import type { AuditEntry, LogEntry, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { DASH } from '$lib/format/number';
	import { session } from '$lib/stores/session.svelte';
	import Badge from '../../atoms/Badge.svelte';
	import CopyButton from '../../atoms/CopyButton.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import BytesCell from '../../cells/BytesCell.svelte';
	import DurationCell from '../../cells/DurationCell.svelte';
	import PathCell from '../../cells/PathCell.svelte';
	import TextCell from '../../cells/TextCell.svelte';
	import TimeCell from '../../cells/TimeCell.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import StaleIndicator from '../../molecules/StaleIndicator.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import Drawer from '../../organisms/Drawer.svelte';
	import DataTable from '../../organisms/table/DataTable.svelte';
	import QueryBar from '../../organisms/QueryBar.svelte';
	import * as queryGrammar from '../../organisms/query-grammar';
	import type { ColumnDef } from '../../types';
	import ActivityRange, { activityRange, rangeKey, rangeSince, rangeUntil } from './ActivityRange.svelte';

	interface AuditPage {
		items: AuditEntry[];
		next_cursor: string | null;
	}

	interface LogPage {
		items: LogEntry[];
	}

	const PAGE_SIZE = 50;
	const LOG_LIMIT = 50;
	const POLL_MS = 5000;
	const AUDIT_FIELDS: readonly string[] = ['user', 'op', 'path', 'status'];
	const FILTER_FACETS: string[] = ['user', 'op', 'path', 'request_id'];
	const AUDIT_GRAMMAR: queryGrammar.QueryGrammar = {
		fields: queryGrammar.grammar.fields.filter((field) => AUDIT_FIELDS.includes(field.key))
	};
	const OP_KEYS: Record<string, string | undefined> = {
		read: 'activity.audit.op.read',
		list: 'activity.audit.op.list',
		stat: 'activity.audit.op.stat',
		upload: 'activity.audit.op.upload',
		replace: 'activity.audit.op.replace',
		rename: 'activity.audit.op.rename',
		move: 'activity.audit.op.move',
		delete: 'activity.audit.op.delete',
		trash: 'activity.audit.op.trash',
		restore: 'activity.audit.op.restore',
		link_create: 'activity.audit.op.link_create',
		link_open: 'activity.audit.op.link_open',
		session: 'activity.audit.op.session',
		admin: 'activity.audit.op.admin'
	};

	const range = activityRange();

	function toFilters(text: string): Record<string, string> {
		const filters: Record<string, string> = {};
		for (const token of queryGrammar.parse(text).tokens) {
			if ('status' === token.field) {
				filters.status = ':' === token.comparator ? token.value : `${token.comparator}${token.value}`;
			} else if (FILTER_FACETS.includes(token.field)) {
				filters[token.field] = token.value;
			}
		}
		return filters;
	}

	function actionKey(op: string): string {
		return OP_KEYS[op] ?? 'activity.audit.op.other';
	}

	function userLabel(userId: string | null): string {
		if (null === userId) {
			return t('activity.audit.noUser');
		}
		return '' === userId ? DASH : userId;
	}

	function applyQuery(): void {
		applied = queryText;
		limit = PAGE_SIZE;
	}

	function openRow(row: AuditEntry): void {
		selected = row;
		drawerOpen = true;
	}

	function exportHref(format: 'csv' | 'json'): string {
		const params = new URLSearchParams({ ...filters, since: rangeSince(range.value), format });
		const until = rangeUntil(range.value);
		if (undefined !== until) {
			params.set('until', until);
		}
		return nodePath(`/v1/admin/audit?${params}`);
	}

	function download(format: 'csv' | 'json'): void {
		const anchor = document.createElement('a');
		anchor.href = exportHref(format);
		anchor.download = `audit.${format}`;
		document.body.appendChild(anchor);
		anchor.click();
		anchor.remove();
	}

	let queryText = $state('');
	let applied = $state('');
	let limit = $state(PAGE_SIZE);
	let selected = $state<AuditEntry | null>(null);
	let drawerOpen = $state(false);

	const live = $derived(range.value.live);
	const filters = $derived(toFilters(applied));

	const auditQuery = createQuery<Result<AuditPage, ApiFailure>>(() => ({
		key: ['audit', rangeKey(range.value), applied, limit],
		fetch: () => audit({ ...filters, since: rangeSince(range.value), until: rangeUntil(range.value), limit }),
		refetchMs: live ? POLL_MS : undefined,
		enabled: () => true === session.isAdmin,
		isEmpty: (result) => true === result.ok && 0 === result.value.items.length
	}));

	const linkedLogs = createQuery<Result<LogPage, ApiFailure>>(() => ({
		key: ['logs', selected?.request_id ?? ''],
		fetch: () => logs({ request_id: selected?.request_id ?? '', limit: LOG_LIMIT }),
		enabled: () => drawerOpen && null !== selected && null !== selected.request_id,
		isEmpty: (result) => true === result.ok && 0 === result.value.items.length
	}));

	const auditPage = $derived(true === auditQuery.data?.ok ? auditQuery.data.value : undefined);
	const logPage = $derived(true === linkedLogs.data?.ok ? linkedLogs.data.value : undefined);
	const rows = $derived(auditPage?.items ?? []);
	const nextCursor = $derived(auditPage?.next_cursor ?? null);
	const logEntries = $derived(logPage?.items ?? []);
	const indicatorState = $derived(null !== auditQuery.error ? 'offline' : false === live ? 'paused' : 'live');

	const columns: ColumnDef<AuditEntry>[] = [
		{ id: 'time', header: t('activity.audit.col.time'), cell: timeCell, width: 150, priority: 1 },
		{ id: 'user', header: t('activity.audit.col.user'), cell: userCell, width: 120, priority: 2 },
		{ id: 'action', header: t('activity.audit.col.action'), cell: actionCell, width: 140, priority: 1 },
		{ id: 'path', header: t('activity.audit.col.path'), cell: pathCell, minWidth: 220, priority: 1 },
		{ id: 'result', header: t('activity.audit.col.result'), cell: resultCell, width: 120, priority: 1 },
		{ id: 'duration', header: t('activity.audit.col.duration'), cell: durationCell, align: 'end', width: 100, priority: 2 },
		{ id: 'server', header: t('activity.audit.col.server'), cell: serverCell, width: 110, priority: 3, hidden: true },
		{ id: 'requestId', header: t('activity.audit.col.requestId'), cell: requestIdCell, width: 210, priority: 3 }
	];
</script>

{#snippet timeCell(row: AuditEntry)}
	<TimeCell value={row.at} />
{/snippet}

{#snippet userCell(row: AuditEntry)}
	<TextCell value={userLabel(row.user_id)} />
{/snippet}

{#snippet actionCell(row: AuditEntry)}
	<TextCell value={t(actionKey(row.op))} />
{/snippet}

{#snippet pathCell(row: AuditEntry)}
	<PathCell path={row.path} />
{/snippet}

{#snippet resultCell(row: AuditEntry)}
	<Badge tone={400 > row.status ? 'ok' : 'danger'}>
		{t(400 > row.status ? 'activity.audit.result.done' : 'activity.audit.result.failed')}
		{row.status}
	</Badge>
{/snippet}

{#snippet durationCell(row: AuditEntry)}
	<DurationCell value={`${row.duration_ms}ms`} />
{/snippet}

{#snippet serverCell(row: AuditEntry)}
	<TextCell value={row.instance_id} />
{/snippet}

{#snippet requestIdCell(row: AuditEntry)}
	{#if null !== row.request_id}
		<Cluster gap="1" align="center">
			<TextCell value={row.request_id} mono />
			<CopyButton value={row.request_id} variant="icon" label={t('activity.audit.copyRequestId')} />
		</Cluster>
	{/if}
{/snippet}

<PageFrame title={t('activity.audit.title')} job={t('activity.audit.job')}>
	{#snippet status()}
		<StaleIndicator
			updatedAt={auditQuery.updatedAt}
			intervalMs={POLL_MS}
			state={indicatorState}
			onrefresh={() => auditQuery.refetch()}
		/>
	{/snippet}
	{#snippet toolbar()}
		<div class="filters">
			<QueryBar
				bind:value={queryText}
				grammar={AUDIT_GRAMMAR}
				label={t('activity.audit.query')}
				placeholder={t('activity.audit.placeholder')}
				hint={t('activity.audit.query_hint')}
				onsubmit={applyQuery}
			/>
			<ActivityRange store={range} />
		</div>
	{/snippet}

	{#if false === session.isAdmin}
		<EmptyState
			variant="no-permission"
			icon="shield"
			title={t('activity.audit.noPermission.title')}
			body={t('activity.audit.noPermission.body')}
		/>
	{:else}
		<DataTable
			{rows}
			{columns}
			rowKey={(row: AuditEntry) =>
				[row.at, row.request_id, row.method, row.route, row.path, row.client].join('|')}
			label={t('activity.audit.table')}
			status={auditQuery.status}
			error={auditQuery.error ?? undefined}
			hasMore={null !== nextCursor}
			onloadmore={() => {
				limit += PAGE_SIZE;
			}}
			onopen={openRow}
			onretry={() => auditQuery.refetch()}
			onexport={download}
		>
			{#snippet empty()}
				<EmptyState
					variant="no-results"
					icon="search"
					title={t('activity.audit.empty.title')}
					body={t('activity.audit.empty.body')}
				/>
			{/snippet}
		</DataTable>
	{/if}
</PageFrame>

<Drawer bind:open={drawerOpen} title={t('activity.audit.drawer.title')}>
	{#if null !== selected}
		<Stack gap="4">
			<dl class="fields">
				<div class="field">
					<dt>{t('activity.audit.field.at')}</dt>
					<dd><TimeCell value={selected.at} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.user')}</dt>
					<dd><TextCell value={userLabel(selected.user_id)} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.action')}</dt>
					<dd><TextCell value={t(actionKey(selected.op))} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.path')}</dt>
					<dd><PathCell path={selected.path} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.method')}</dt>
					<dd><TextCell value={selected.method} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.route')}</dt>
					<dd><TextCell value={selected.route} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.result')}</dt>
					<dd>
						<Badge tone={400 > selected.status ? 'ok' : 'danger'}>
							{t(400 > selected.status ? 'activity.audit.result.done' : 'activity.audit.result.failed')}
							{selected.status}
						</Badge>
					</dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.bytesIn')}</dt>
					<dd><BytesCell value={selected.bytes_in} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.bytesOut')}</dt>
					<dd><BytesCell value={selected.bytes_out} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.duration')}</dt>
					<dd><DurationCell value={`${selected.duration_ms}ms`} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.server')}</dt>
					<dd><TextCell value={selected.instance_id} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.client')}</dt>
					<dd><TextCell value={selected.client} /></dd>
				</div>
				<div class="field">
					<dt>{t('activity.audit.field.requestId')}</dt>
					<dd>
						<Cluster gap="1" align="center">
							<TextCell value={selected.request_id ?? ''} />
							{#if null !== selected.request_id}
								<CopyButton value={selected.request_id} variant="icon" label={t('activity.audit.copyRequestId')} />
							{/if}
						</Cluster>
					</dd>
				</div>
			</dl>
			<Stack gap="2">
				<h2 class="heading">{t('activity.audit.logs.title')}</h2>
				{#if 'loading' === linkedLogs.status}
					<Skeleton shape="rect" height="var(--space-10)" />
				{:else if null !== linkedLogs.error}
					<ErrorState error={linkedLogs.error} variant="region" onretry={() => linkedLogs.refetch()} />
				{:else if 0 === logEntries.length}
					<p class="note">{t('activity.audit.logs.empty')}</p>
				{:else}
					<ol class="logs">
						{#each logEntries as entry (entry.id)}
							<li class="log">
								<TimeCell value={entry.at} />
								<span class="message">{entry.message}</span>
							</li>
						{/each}
					</ol>
				{/if}
			</Stack>
		</Stack>
	{/if}
</Drawer>

<style>
	.filters {
		display: grid;
		flex: 1 1 auto;
		gap: var(--space-3);
		min-inline-size: 0;
	}
	.fields {
		display: grid;
		gap: var(--space-4);
		padding: 0;
	}
	.field {
		display: grid;
		gap: var(--space-1-5);
	}
	dt {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
	}
	dd {
		margin: 0;
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}
	.heading {
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
	}
	.note {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}
	.logs {
		display: grid;
		gap: var(--space-1-5);
		padding: 0;
		list-style: none;
	}
	.log {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2);
	}
	.message {
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}
</style>
