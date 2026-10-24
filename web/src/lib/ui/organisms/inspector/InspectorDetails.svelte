<script lang="ts" module>
	import type { Stat } from '$lib/api/types';
	import { t } from '$lib/copy/t';

	const STORAGE_KEYS: Record<Stat['storage'], string> = {
		localfs: 'placement.storage.localfs',
		s3: 'placement.storage.s3',
		erasure: 'placement.storage.erasure'
	};

	export function storageLabel(storage: Stat['storage']): string {
		return t(STORAGE_KEYS[storage]);
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import DescriptionList from '../../molecules/DescriptionList.svelte';
	import type { DescriptionItem } from '../../molecules/DescriptionList.svelte';
	import { formatNumber } from '$lib/format/number';
	import { formatTimestamp } from '$lib/format/time';

	interface Props extends Omit<HTMLAttributes<HTMLDListElement>, 'class'> {
		stat: Stat;
		class?: string;
	}

	let { stat, class: className, ...rest }: Props = $props();

	const items: DescriptionItem[] = $derived([
		{ term: t('inspector.details.path'), value: stat.path, copy: stat.path },
		{
			term: t('inspector.details.type'),
			value: 'file' === stat.type ? t('inspector.type.file') : t('inspector.type.directory')
		},
		{
			term: t('inspector.details.size'),
			value: t('inspector.details.size_exact', { bytes: formatNumber(stat.size, 'int') })
		},
		{ term: t('inspector.details.modified'), value: formatTimestamp(stat.mtime) },
		{ term: t('inspector.details.created'), value: formatTimestamp(stat.created_at) },
		{ term: t('inspector.details.changed'), value: formatTimestamp(stat.ctime) },
		{ term: t('inspector.details.accessed'), value: formatTimestamp(stat.atime) },
		{
			term: t('inspector.details.owner'),
			value: '' === stat.owner ? t('status.file.no_owner') : stat.owner
		},
		{ term: t('inspector.details.mode'), value: stat.mode },
		{
			term: t('inspector.details.version'),
			value: '' === stat.etag ? t('cells.etag.not_set') : stat.etag,
			copy: '' === stat.etag ? undefined : stat.etag
		},
		{ term: t('inspector.details.storage'), value: storageLabel(stat.storage) },
		{ term: t('inspector.details.server'), value: stat.instance_id },
		{
			term: t('inspector.details.split'),
			value: stat.erasure_coded ? t('inspector.value.yes') : t('inspector.value.no')
		},
		{
			term: t('inspector.details.link_target'),
			value: null === stat.symlink_target ? t('inspector.value.none') : stat.symlink_target
		}
	]);
</script>

<DescriptionList {items} class={className} {...rest} />
