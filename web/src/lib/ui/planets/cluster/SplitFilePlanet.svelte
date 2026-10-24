<script lang="ts">
	import { getSplitFile, manifest, pieceUrl, repair, verify, type ChunkManifest } from '$lib/api/erasure';
	import type { SplitFileDetail, VerifyReport, Result } from '$lib/api/types';
	import { createQuery } from '$lib/data/query.svelte';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import { notifications } from '$lib/stores/notifications.svelte';
	import { jobs } from '$lib/stores/jobs.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { StatusKey } from '$lib/status/catalog';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import BytesCell from '$lib/ui/cells/BytesCell.svelte';
	import PieceCell from '$lib/ui/cells/PieceCell.svelte';
	import ServerCell from '$lib/ui/cells/ServerCell.svelte';
	import StatusCell from '$lib/ui/cells/StatusCell.svelte';
	import StorageCell from '$lib/ui/cells/StorageCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import DurabilityMeter from '$lib/ui/molecules/DurabilityMeter.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import CodeView from '$lib/ui/organisms/CodeView.svelte';
	import PieceMap, { type Piece } from '$lib/ui/organisms/PieceMap.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import type { ColumnDef, UiError } from '$lib/ui/types';

	type Shard = SplitFileDetail['shards'][number];
	type ShardState = Shard['state'];

	interface Props {
		path: string;
	}

	let { path }: Props = $props();

	const PIECE_STATUS: Record<ShardState, StatusKey> = {
		ok: 'piece.ok',
		missing: 'piece.missing',
		corrupt: 'piece.corrupt',
		unreachable: 'piece.unreachable',
		unknown: 'piece.unreachable'
	};

	const target = $derived(`/${path}`);
	const fileName = $derived(target.split('/').pop() ?? target);

	const detailQuery = createQuery<Result<SplitFileDetail, UiError>>(() => {
		const current = target;
		return {
			key: ['erasure', 'file', current],
			fetch: () => getSplitFile(current),
			liveTopics: ['job.*']
		};
	});

	const manifestQuery = createQuery<Result<ChunkManifest, UiError>>(() => {
		const current = target;
		return {
			key: ['erasure', 'manifest', current],
			fetch: () => manifest(current)
		};
	});

	let verifyReport = $state<VerifyReport | null>(null);
	let actionError = $state<UiError | null>(null);
	let verifying = $state(false);
	let repairing = $state(false);

	const detailResult = $derived(detailQuery.data);
	const detail = $derived(detailResult?.ok ? detailResult.value : undefined);
	const detailError = $derived(detailResult && !detailResult.ok ? detailResult.error : null);
	const shards = $derived(detail?.shards ?? []);
	const healthyCount = $derived(shards.filter((shard) => 'ok' === shard.state).length);
	const unknownCount = $derived(shards.filter((shard) => 'unknown' === shard.state).length);
	const damagedCount = $derived(shards.length - healthyCount);
	const pieces = $derived<Piece[]>(
		shards.map((shard) => ({
			index: shard.index,
			kind: pieceKindOf(shard.kind),
			server: shard.instance_id,
			state: PIECE_STATUS[shard.state]
		}))
	);
	const servers = $derived([...new Set(shards.map((shard) => shard.instance_id))]);

	const manifestResult = $derived(manifestQuery.data);
	const manifestValue = $derived(manifestResult?.ok ? manifestResult.value : undefined);
	const manifestError = $derived(manifestResult && !manifestResult.ok ? manifestResult.error : null);

	const columns = $derived<ColumnDef<Shard>[]>([
		{ id: 'piece', header: t('durability.piece.col.piece'), cell: pieceColumn },
		{ id: 'kind', header: t('durability.piece.col.kind'), cell: kindColumn },
		{ id: 'server', header: t('durability.piece.col.server'), cell: serverColumn },
		{ id: 'storage', header: t('durability.piece.col.storage'), cell: storageColumn },
		{ id: 'size', header: t('durability.piece.col.size'), cell: sizeColumn, align: 'end' },
		{ id: 'checksum', header: t('durability.piece.col.checksum'), cell: checksumColumn },
		{ id: 'state', header: t('durability.piece.col.state'), cell: stateColumn },
		{ id: 'download', header: '', cell: downloadColumn }
	]);

	function pieceKindOf(kind: Shard['kind']): Piece['kind'] {
		return 'parity' === kind ? 'spare' : 'data';
	}

	function reportFailure(error: UiError): void {
		actionError = error;
		notifications.add({
			tone: 'danger',
			category: 'error',
			title: error.title,
			body: error.body,
			requestId: error.requestId ?? null
		});
	}

	function refresh(): void {
		void detailQuery.refetch();
		void manifestQuery.refetch();
	}

	async function verifyPieces(): Promise<void> {
		verifying = true;
		const response = await verify(target);
		verifying = false;
		if (!response.ok) {
			reportFailure(response.error);
			return;
		}
		actionError = null;
		verifyReport = response.value;
	}

	async function repairPieces(): Promise<void> {
		repairing = true;
		const response = await repair(target);
		repairing = false;
		if (!response.ok) {
			reportFailure(response.error);
			return;
		}
		actionError = null;
		jobs.watch(response.value.job_id);
		toasts.push({ tone: 'info', title: t('durability.repair.file_started') });
	}
</script>

{#snippet pieceColumn(shard: Shard)}
	<PieceCell index={shard.index} kind={pieceKindOf(shard.kind)} state={PIECE_STATUS[shard.state]} />
{/snippet}

{#snippet kindColumn(shard: Shard)}
	<TextCell value={'parity' === shard.kind ? t('durability.kind.spare') : t('durability.kind.data')} />
{/snippet}

{#snippet serverColumn(shard: Shard)}
	<ServerCell id={shard.instance_id} />
{/snippet}

{#snippet storageColumn(shard: Shard)}
	<StorageCell value={shard.storage} />
{/snippet}

{#snippet sizeColumn(shard: Shard)}
	<BytesCell value={shard.size} />
{/snippet}

{#snippet checksumColumn(shard: Shard)}
	<TextCell value={shard.checksum} mono />
{/snippet}

{#snippet stateColumn(shard: Shard)}
	<StatusCell status={PIECE_STATUS[shard.state]} />
{/snippet}

{#snippet downloadColumn(shard: Shard)}
	<Link href={pieceUrl(target, shard.index)} download>
		{t('durability.piece.download', { index: formatNumber(shard.index) })}
	</Link>
{/snippet}

{#snippet noPieces()}
	<EmptyState
		variant="no-results"
		icon="pieces"
		title={t('durability.piece.empty.title')}
		body={t('durability.piece.empty.body')}
	/>
{/snippet}

{#snippet primary()}
	{#if detail}
		{#if 0 < damagedCount}
			<Button variant="primary" loading={repairing} onclick={repairPieces}>{t('durability.file.primary.repair')}</Button>
		{:else}
			<Button variant="primary" loading={verifying} onclick={verifyPieces}>{t('durability.file.primary.verify')}</Button>
		{/if}
	{/if}
{/snippet}

{#snippet secondary()}
	<Button variant="ghost" iconStart="refresh" onclick={refresh}>{t('durability.refresh')}</Button>
{/snippet}

<PageFrame title={fileName} job={t('durability.file.job')} {primary} {secondary}>
	{#if undefined === detailResult}
		<Skeleton shape="rect" height="var(--space-12)" />
	{:else if detailError}
		<ErrorState error={detailError} onretry={refresh} />
	{:else if detail}
		<Stack gap="8">
			{#if actionError}
				<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
			{/if}
			{#if verifyReport}
				{#if !verifyReport.readable}
					<InlineAlert tone="danger" title={t('durability.verify.unreadable')} />
				{:else if verifyReport.healthy < verifyReport.shards.length}
					<InlineAlert
						tone="warn"
						title={t('durability.verify.partial', {
							healthy: formatNumber(verifyReport.healthy),
							total: formatNumber(verifyReport.shards.length)
						})}
					/>
				{:else}
					<InlineAlert
						tone="ok"
						title={t('durability.verify.ok', { total: formatNumber(verifyReport.shards.length) })}
					/>
				{/if}
			{/if}
			{#if 0 < unknownCount}
				<InlineAlert tone="warn" title={t('durability.file.partial.title')} body={t('durability.file.partial.body')} />
			{/if}
			<Stack gap="4">
				<Heading level={2}>{t('durability.section.map')}</Heading>
				<DurabilityMeter data={detail.profile.data_shards} spare={detail.profile.parity_shards} healthy={healthyCount} />
				<PieceMap {pieces} {servers} />
			</Stack>
			<Stack gap="4">
				<Heading level={2}>{t('durability.section.pieces')}</Heading>
				<DataTable
					rows={shards}
					columns={columns}
					rowKey={(row: Shard) => String(row.index)}
					label={t('durability.piece.table.label')}
					status={0 === shards.length ? 'empty' : 'success'}
					empty={noPieces}
				/>
			</Stack>
			<Stack gap="4">
				<Heading level={2}>{t('durability.section.manifest')}</Heading>
				{#if manifestValue}
					<CodeView code={JSON.stringify(manifestValue, null, 2)} language="json" label={t('durability.manifest.label')} />
				{:else if manifestError}
					<ErrorState error={manifestError} onretry={refresh} />
				{:else}
					<Skeleton shape="rect" height="var(--space-12)" />
				{/if}
			</Stack>
		</Stack>
	{/if}
</PageFrame>
