<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Button from '../atoms/Button.svelte';
	import Label from '../atoms/Label.svelte';
	import Select from '../atoms/Select.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'children'> {
		page?: number;
		pageSize?: number;
		total: number;
		sizes?: number[];
		class?: string;
	}

	let {
		page = $bindable(1),
		pageSize = $bindable(25),
		total,
		sizes = [25, 50, 100],
		class: className,
		...rest
	}: Props = $props();

	const sizeId = $props.id();
	const pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
	const current = $derived(Math.min(Math.max(1, page), pageCount));
	const from = $derived(0 === total ? 0 : (current - 1) * pageSize + 1);
	const to = $derived(Math.min(current * pageSize, total));
	const sizeOptions = $derived(sizes.map((size) => ({ value: String(size), label: String(size) })));
	const rangeText = $derived(
		0 === total
			? t('tablecontrols.pagination.empty')
			: t('tablecontrols.pagination.range', {
					from: formatNumber(from),
					to: formatNumber(to),
					total: formatNumber(total)
				})
	);

	function changeSize(event: Event): void {
		if (!(event.currentTarget instanceof HTMLSelectElement)) {
			return;
		}
		const selectedSize = Number(event.currentTarget.value);
		const nextSize = sizes.find((size) => size === selectedSize);
		if (undefined === nextSize) {
			return;
		}
		pageSize = nextSize;
		page = 1;
	}
</script>

<Cluster
	as="nav"
	gap="3"
	justify="space-between"
	class={['pagination', className]}
	aria-label={t('tablecontrols.pagination.label')}
	{...rest}
>
	<Text tone="muted" tabular>{rangeText}</Text>
	<Cluster gap="2">
		<Button size="sm" variant="ghost" disabled={1 >= current} onclick={() => { page = 1; }}>
			{t('tablecontrols.pagination.first')}
		</Button>
		<Button size="sm" variant="ghost" disabled={1 >= current} onclick={() => { page = current - 1; }}>
			{t('tablecontrols.pagination.previous')}
		</Button>
		<Button size="sm" variant="ghost" disabled={current >= pageCount} onclick={() => { page = current + 1; }}>
			{t('tablecontrols.pagination.next')}
		</Button>
		<Button size="sm" variant="ghost" disabled={current >= pageCount} onclick={() => { page = pageCount; }}>
			{t('tablecontrols.pagination.last')}
		</Button>
		<Cluster gap="1-5">
			<Label for={sizeId}>{t('tablecontrols.pagination.size_label')}</Label>
			<Select id={sizeId} value={String(pageSize)} options={sizeOptions} size="sm" onchange={changeSize} />
		</Cluster>
	</Cluster>
</Cluster>
