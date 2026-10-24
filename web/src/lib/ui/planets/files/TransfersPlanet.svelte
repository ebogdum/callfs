<script lang="ts" module>
	import type { TransferState } from '$lib/stores/transfers.svelte';
	import type { StatusKey } from '$lib/ui/types';
	import { formatBytes } from '$lib/format/bytes';
	import { t } from '$lib/copy/t';

	const TAB_STATES: Record<string, readonly TransferState[]> = {
		active: ['uploading', 'downloading', 'verifying', 'stalled'],
		waiting: ['queued', 'paused', 'conflict'],
		failed: ['failed'],
		done: ['done', 'cancelled'],
		server: []
	};

	const STATE_STATUS: Record<TransferState, StatusKey> = {
		queued: 'transfer.queued',
		uploading: 'transfer.uploading',
		downloading: 'transfer.downloading',
		paused: 'transfer.paused',
		stalled: 'transfer.stalled',
		verifying: 'transfer.verifying',
		done: 'transfer.done',
		failed: 'transfer.failed',
		cancelled: 'transfer.cancelled',
		conflict: 'transfer.conflict'
	};

	const PAUSABLE: readonly TransferState[] = ['uploading', 'downloading'];
	const RESUMABLE: readonly TransferState[] = ['paused', 'stalled'];
	const RETRYABLE: readonly TransferState[] = ['failed', 'cancelled'];
	const CANCELLABLE: readonly TransferState[] = ['queued', 'uploading', 'downloading', 'paused', 'stalled', 'verifying', 'conflict'];

	const speedText = (bytesPerSecond: number): string =>
		t('transfers-trash.transfers.speed', { value: formatBytes(bytesPerSecond).text });
</script>

<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { transfers as serverTransfers } from '$lib/api/admin/telemetry';
	import { toUiError } from '$lib/api/errors';
	import type { Transfer as ServerTransfer } from '$lib/api/types';
	import { formatAbsolute } from '$lib/format/time';
	import { formatDuration } from '$lib/format/duration';
	import { formatNumber } from '$lib/format/number';
	import { parentPath } from '$lib/format/path';
	import { session } from '$lib/stores/session.svelte';
	import { transfers, type ConflictPolicy, type Transfer } from '$lib/stores/transfers.svelte';
	import type { ColumnDef, MenuItem, QueryStatus, Selection, UiError } from '$lib/ui/types';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import NumberInput from '$lib/ui/atoms/NumberInput.svelte';
	import Select from '$lib/ui/atoms/Select.svelte';
	import SizeInput from '$lib/ui/molecules/SizeInput.svelte';
	import FormField from '$lib/ui/molecules/FormField.svelte';
	import Popover from '$lib/ui/molecules/Popover.svelte';
	import Tabs from '$lib/ui/molecules/Tabs.svelte';
	import DescriptionList from '$lib/ui/molecules/DescriptionList.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import PathCell from '$lib/ui/cells/PathCell.svelte';
	import ProgressCell from '$lib/ui/cells/ProgressCell.svelte';
	import StatusCell from '$lib/ui/cells/StatusCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Drawer from '$lib/ui/organisms/Drawer.svelte';

	const noSelection: Selection = { mode: 'none', ids: new Set<string>(), allMatching: false };

	const tabLabel = (id: string): string => t(`transfers-trash.transfers.tab.${id}`);
	const rowKey = (row: Transfer): string => row.id;
	const serverRowKey = (row: ServerTransfer): string => row.id;

	let tab = $state('active');
	let serverStatus = $state<QueryStatus>('loading');
	let serverRows = $state<ServerTransfer[]>([]);
	let serverError = $state<UiError | null>(null);
	let diagnosticsId = $state<string | null>(null);
	let diagnosticsOpen = $state(false);
	let parallel = $state<number | null>(transfers.settings.parallel);
	let partBytes = $state(transfers.settings.partBytes);
	let conflictDefault = $state<ConflictPolicy>('skip');

	const localRows = $derived(transfers.items.filter((item) => TAB_STATES[tab].includes(item.state)));
	const localStatus = $derived<QueryStatus>(0 === localRows.length ? 'empty' : 'success');
	const failedCount = $derived(transfers.items.filter((item) => 'failed' === item.state).length);
	const countOf = (id: string): number => transfers.items.filter((item) => TAB_STATES[id].includes(item.state)).length;
	const tabItems = $derived([
		{ id: 'active', label: tabLabel('active'), count: countOf('active') },
		{ id: 'waiting', label: tabLabel('waiting'), count: countOf('waiting') },
		{ id: 'failed', label: tabLabel('failed'), count: countOf('failed') },
		{ id: 'done', label: tabLabel('done'), count: countOf('done') },
		...(session.isAdmin ? [{ id: 'server', label: tabLabel('server'), count: serverRows.length }] : [])
	]);
	const diagnosticRow = $derived(transfers.items.find((item) => diagnosticsId === item.id) ?? null);
	const policyOptions = $derived([
		{ value: 'skip', label: t('transfers-trash.transfers.conflict.skip') },
		{ value: 'replace', label: t('transfers-trash.transfers.conflict.replace') },
		{ value: 'keep-both', label: t('transfers-trash.transfers.conflict.keep_both') },
		{ value: 'replace-if-changed', label: t('transfers-trash.transfers.conflict.replace_if_changed') }
	]);
	const diagnosticItems = $derived(
		null === diagnosticRow
			? []
			: [
					{ term: t('transfers-trash.transfers.diagnostics.part_size'), value: formatBytes(transfers.settings.partBytes).text },
					{ term: t('transfers-trash.transfers.diagnostics.parallel'), value: formatNumber(transfers.settings.parallel) },
					{
						term: t('transfers-trash.transfers.diagnostics.parts'),
						value: t('transfers-trash.transfers.diagnostics.parts_value', {
							done: formatNumber(diagnosticRow.partsDone ?? 0),
							total: formatNumber(diagnosticRow.partsTotal ?? 0)
						})
					},
					{
						term: t('transfers-trash.transfers.diagnostics.resumed'),
						value:
							null === diagnosticRow.resumedAt
								? t('transfers-trash.transfers.diagnostics.not_resumed')
								: formatAbsolute(diagnosticRow.resumedAt)
					},
					{
						term: t('transfers-trash.transfers.diagnostics.last_error'),
						value: null === diagnosticRow.error ? t('transfers-trash.transfers.diagnostics.no_error') : diagnosticRow.error.title
					}
				]
	);

	const menuFor = (row: Transfer): MenuItem[] => {
		const items: MenuItem[] = [];
		if (PAUSABLE.includes(row.state)) {
			items.push({ id: 'pause', label: t('transfers-trash.transfers.action.pause'), onselect: () => transfers.pause(row.id) });
		}
		if (RESUMABLE.includes(row.state)) {
			items.push({ id: 'resume', label: t('transfers-trash.transfers.action.resume'), onselect: () => transfers.resume(row.id) });
		}
		if (RETRYABLE.includes(row.state)) {
			items.push({ id: 'retry', label: t('transfers-trash.transfers.action.retry'), onselect: () => transfers.retry(row.id) });
		}
		if (CANCELLABLE.includes(row.state)) {
			items.push({
				id: 'cancel',
				label: t('transfers-trash.transfers.action.cancel'),
				tone: 'danger',
				onselect: () => void transfers.cancel(row.id)
			});
		}
		items.push({
			id: 'open-folder',
			label: t('transfers-trash.transfers.action.open_folder'),
			onselect: () => void goto(`${base}/files${parentPath(row.destination)}`)
		});
		items.push({
			id: 'diagnostics',
			label: t('transfers-trash.transfers.action.diagnostics'),
			onselect: () => {
				diagnosticsId = row.id;
				diagnosticsOpen = true;
			}
		});
		return items;
	};

	const loadServer = async (): Promise<void> => {
		serverStatus = 'loading';
		const result = await serverTransfers();
		if (!result.ok) {
			serverError = toUiError(result.error, { action: 'transfers-trash.transfers.load' });
			serverStatus = 'error';
			return;
		}
		serverError = null;
		serverRows = result.value.items;
		serverStatus = 0 === result.value.items.length ? 'empty' : 'success';
	};

	$effect(() => {
		if ('server' === tab && session.isAdmin) {
			void loadServer();
		}
	});

	$effect(() => {
		const value = parallel;
		untrack(() => {
			if (null !== value) {
				transfers.setParallel(value);
			}
		});
	});

	$effect(() => {
		const value = partBytes;
		untrack(() => transfers.setPartBytes(value));
	});

	const localColumns: ColumnDef<Transfer>[] = [
		{ id: 'name', header: t('transfers-trash.transfers.col.name'), cell: nameCell, minWidth: 200, priority: 1 },
		{ id: 'destination', header: t('transfers-trash.transfers.col.destination'), cell: destinationCell, minWidth: 160 },
		{ id: 'progress', header: t('transfers-trash.transfers.col.progress'), cell: progressCell, minWidth: 180, priority: 1 },
		{ id: 'speed', header: t('transfers-trash.transfers.col.speed'), cell: speedCell, align: 'end' },
		{ id: 'time_left', header: t('transfers-trash.transfers.col.time_left'), cell: timeLeftCell, align: 'end' },
		{ id: 'state', header: t('transfers-trash.transfers.col.state'), cell: stateCell, priority: 1 }
	];

	const serverColumns: ColumnDef<ServerTransfer>[] = [
		{ id: 'user', header: t('transfers-trash.transfers.col.user'), cell: userCell, minWidth: 120, priority: 1 },
		{ id: 'kind', header: t('transfers-trash.transfers.col.kind'), cell: kindCell },
		{ id: 'path', header: t('transfers-trash.transfers.col.path'), cell: serverPathCell, minWidth: 200, priority: 1 },
		{ id: 'progress', header: t('transfers-trash.transfers.col.progress'), cell: serverProgressCell, minWidth: 180, priority: 1 },
		{ id: 'started', header: t('transfers-trash.transfers.col.started'), cell: startedCell },
		{ id: 'client', header: t('transfers-trash.transfers.col.client'), cell: clientCell }
	];
</script>

{#snippet nameCell(row: Transfer)}
	<TextCell value={row.name} />
{/snippet}

{#snippet destinationCell(row: Transfer)}
	<PathCell path={row.destination} />
{/snippet}

{#snippet progressCell(row: Transfer)}
	<ProgressCell done={row.done} total={row.total} state={STATE_STATUS[row.state]} bytesPerSecond={row.bytesPerSecond} />
{/snippet}

{#snippet speedCell(row: Transfer)}
	<TextCell value={speedText(row.bytesPerSecond)} mono />
{/snippet}

{#snippet timeLeftCell(row: Transfer)}
	<TextCell value={formatDuration(null === row.eta ? null : row.eta * 1000, 'short')} mono />
{/snippet}

{#snippet stateCell(row: Transfer)}
	<StatusCell status={STATE_STATUS[row.state]} />
{/snippet}

{#snippet userCell(row: ServerTransfer)}
	<TextCell value={row.user_id} />
{/snippet}

{#snippet kindCell(row: ServerTransfer)}
	<TextCell value={row.kind} />
{/snippet}

{#snippet serverPathCell(row: ServerTransfer)}
	<PathCell path={row.path} />
{/snippet}

{#snippet serverProgressCell(row: ServerTransfer)}
	<ProgressCell
		done={row.bytes_done}
		total={row.bytes_total}
		state={'upload' === row.kind ? 'transfer.uploading' : 'transfer.downloading'}
	/>
{/snippet}

{#snippet startedCell(row: ServerTransfer)}
	<TimeCell value={row.started_at} />
{/snippet}

{#snippet clientCell(row: ServerTransfer)}
	<TextCell value={row.client} />
{/snippet}

{#snippet emptyLocal()}
	<EmptyState
		variant="first-use"
		icon="upload"
		title={t('transfers-trash.transfers.empty.title')}
		body={t('transfers-trash.transfers.empty.body')}
	/>
{/snippet}

{#snippet emptyServer()}
	<EmptyState
		variant="cleared"
		icon="upload"
		title={t('transfers-trash.transfers.server_empty.title')}
		body={t('transfers-trash.transfers.server_empty.body')}
	/>
{/snippet}

{#snippet retryFailedPrimary()}
	{#if 0 < failedCount}
		<Button variant="primary" onclick={() => transfers.retryFailed()}>{t('transfers-trash.transfers.action.retry_failed')}</Button>
	{/if}
{/snippet}

{#snippet settingsPopover()}
	<Popover label={t('transfers-trash.transfers.settings.title')}>
		{#snippet trigger(props)}
			<Button variant="secondary" iconStart="settings" {...props}>{t('transfers-trash.transfers.settings.button')}</Button>
		{/snippet}
		{#snippet content()}
			<FormField label={t('transfers-trash.transfers.settings.parallel')} for="transfers-parallel">
				{#snippet control({ id, describedBy, invalid })}
					<NumberInput id={id} aria-describedby={describedBy} aria-invalid={invalid} bind:value={parallel} min={1} max={6} />
				{/snippet}
			</FormField>
			<SizeInput label={t('transfers-trash.transfers.settings.part_size')} bind:value={partBytes} />
			<FormField label={t('transfers-trash.transfers.settings.conflict')} for="transfers-conflict">
				{#snippet control({ id, describedBy, invalid })}
					<Select id={id} aria-describedby={describedBy} aria-invalid={invalid} bind:value={conflictDefault} options={policyOptions} />
				{/snippet}
			</FormField>
		{/snippet}
	</Popover>
{/snippet}

{#snippet tabsRegion()}
	<Tabs items={tabItems} bind:value={tab} label={t('transfers-trash.transfers.tabs')} />
{/snippet}

<PageFrame
	title={t('transfers-trash.transfers.title')}
	job={t('transfers-trash.transfers.job')}
	primary={retryFailedPrimary}
	secondary={settingsPopover}
	tabs={tabsRegion}
>
	{#if 'server' === tab}
		<DataTable
			rows={serverRows}
			columns={serverColumns}
			rowKey={serverRowKey}
			label={t('transfers-trash.transfers.server_table')}
			status={serverStatus}
			error={serverError ?? undefined}
			onretry={() => void loadServer()}
			selection={noSelection}
			empty={emptyServer}
		/>
	{:else}
		<DataTable
			rows={localRows}
			columns={localColumns}
			{rowKey}
			label={tabLabel(tab)}
			status={localStatus}
			selection={noSelection}
			rowInline={(row) => menuFor(row).slice(0, 2)}
			rowMenu={menuFor}
			empty={emptyLocal}
		/>
	{/if}
</PageFrame>

<Drawer bind:open={diagnosticsOpen} title={t('transfers-trash.transfers.diagnostics.title')}>
	<DescriptionList items={diagnosticItems} />
</Drawer>
