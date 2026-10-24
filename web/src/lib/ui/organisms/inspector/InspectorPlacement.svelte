<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { base } from '$app/paths';
	import type { Locks, ServerInfo, Stat } from '$lib/api/types';
	import DescriptionList from '../../molecules/DescriptionList.svelte';
	import type { DescriptionItem } from '../../molecules/DescriptionList.svelte';
	import Link from '../../atoms/Link.svelte';
	import { t } from '$lib/copy/t';
	import { formatRelative } from '$lib/format/time';
	import { storageLabel } from './InspectorDetails.svelte';

	type LockInfo = Locks['held'][number];

	interface Props extends Omit<HTMLAttributes<HTMLDListElement>, 'class'> {
		stat: Stat;
		server: ServerInfo | null;
		lock: LockInfo | null;
		cache: { hit: boolean } | null;
		class?: string;
	}

	let { stat, server, lock, cache, class: className, ...rest }: Props = $props();

	const route = $derived(
		stat.is_local ? t('inspector.route.local') : null === server ? t('inspector.route.legacy') : t('inspector.route.other')
	);

	const lockText = $derived(
		null === lock
			? t('inspector.lock.none')
			: t('inspector.lock.held', {
					holder: lock.owner_hint,
					since: formatRelative(lock.acquired_at),
					expires: formatRelative(lock.expires_at)
				})
	);

	const cacheText = $derived(
		null === cache ? t('common.unknown') : cache.hit ? t('inspector.cache.hit') : t('inspector.cache.miss')
	);

	const items: DescriptionItem[] = $derived([
		{ term: t('inspector.placement.storage'), value: storageLabel(stat.storage) },
		{
			term: t('inspector.placement.server'),
			value: null === server ? t('common.unknown') : serverLink
		},
		{ term: t('inspector.placement.route'), value: route },
		{ term: t('inspector.placement.lock'), value: lockText },
		{ term: t('inspector.placement.cache'), value: cacheText }
	]);
</script>

{#snippet serverLink()}
	<Link href={`${base}/cluster/servers`}>{server?.instance_id ?? ''}</Link>
{/snippet}

<DescriptionList {items} class={className} {...rest} />
