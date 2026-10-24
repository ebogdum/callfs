<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ComponentProps } from 'svelte';
	import type { SplitFileDetail } from '$lib/api/types';
	import type { StatusKey } from '../../types';
	import PieceMap from '../PieceMap.svelte';
	import DurabilityMeter from '../../molecules/DurabilityMeter.svelte';
	import Button from '../../atoms/Button.svelte';
	import Text from '../../atoms/Text.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';

	type Pieces = ComponentProps<typeof PieceMap>['pieces'];
	type ShardState = SplitFileDetail['shards'][number]['state'];

	const PIECE_STATUS: Record<ShardState, StatusKey> = {
		ok: 'piece.ok',
		missing: 'piece.missing',
		corrupt: 'piece.corrupt',
		unreachable: 'piece.unreachable',
		unknown: 'piece.unreachable'
	};

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		erasure: SplitFileDetail | null;
		onverify?: () => void;
		onrepair?: () => void;
		busy: boolean;
		class?: string;
	}

	let { erasure, onverify, onrepair, busy, class: className, ...rest }: Props = $props();

	const pieces: Pieces = $derived(
		null === erasure
			? []
			: erasure.shards.map((shard) => ({
					index: shard.index,
					kind: 'data' === shard.kind ? 'data' : 'spare',
					server: shard.instance_id,
					state: PIECE_STATUS[shard.state]
				}))
	);

	const servers: string[] = $derived(
		null === erasure ? [] : Array.from(new Set(erasure.shards.map((shard) => shard.instance_id)))
	);

	const healthy = $derived(
		null === erasure ? 0 : erasure.shards.filter((shard) => 'ok' === shard.state).length
	);

	const resultText = $derived(
		t('inspector.durability.result', {
			healthy: formatNumber(healthy, 'int'),
			total: formatNumber(null === erasure ? 0 : erasure.shards.length, 'int')
		})
	);
</script>

<div class={['inspector-durability', className]} {...rest}>
	{#if null === erasure}
		<Text tone="muted">{t('inspector.durability.not_split')}</Text>
	{:else}
		<Stack gap="3">
			<DurabilityMeter data={erasure.profile.data_shards} spare={erasure.profile.parity_shards} {healthy} />
			<Text tone="muted">{resultText}</Text>
			<PieceMap {pieces} {servers} />
			<Cluster gap="2">
				<Button variant="secondary" iconStart="shield" loading={busy} onclick={onverify}>
					{t('inspector.durability.verify')}
				</Button>
				<Button variant="secondary" iconStart="fix" loading={busy} onclick={onrepair}>
					{t('inspector.durability.repair')}
				</Button>
			</Cluster>
		</Stack>
	{/if}
</div>
