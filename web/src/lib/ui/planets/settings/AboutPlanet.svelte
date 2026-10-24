<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { status as loadStatus } from '$lib/api/admin/status';
	import { API_ROUTES } from '$lib/api/routes';
	import type { Capabilities } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatDuration } from '$lib/format/duration';
	import { formatAbsolute } from '$lib/format/time';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { session } from '$lib/stores/session.svelte';
	import Badge from '../../atoms/Badge.svelte';
	import Button from '../../atoms/Button.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import PageFrame from '../../environments/PageFrame.svelte';
	import DescriptionList, { type DescriptionItem } from '../../molecules/DescriptionList.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import Scroll from '../../quarks/Scroll.svelte';
	import SettingsCard from './SettingsCard.svelte';
	import SettingsItem from './SettingsItem.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
		class?: string;
	}

	type FeatureFlag = keyof Capabilities['features'];

	const FEATURE_ROWS: readonly { id: string; flag: FeatureFlag }[] = [
		{ id: 'split_storage', flag: 'erasure' },
		{ id: 'raft', flag: 'raft' },
		{ id: 'other_servers', flag: 'cross_server' },
		{ id: 'backup_copy', flag: 'replication' },
		{ id: 'quic', flag: 'quic' },
		{ id: 'metrics_port', flag: 'metrics_listener' },
		{ id: 'console', flag: 'ui' }
	];

	const LICENSE_IDS: readonly string[] = ['inter', 'jetbrains_mono', 'lucide', 'bits_ui'];
	const STATUS_STALE_MS = 60_000;
	const MS_PER_SECOND = 1000;

	const parityRows = API_ROUTES.map((route) => ({
		method: route.method,
		path: route.path,
		id: route.id,
		screens: true === route.internal ? t('about.parity.internal') : listOrNone(route.screens),
		commands: listOrNone(route.commands)
	}));

	const parityInternal = API_ROUTES.filter((route) => true === route.internal).length;
	const parityCovered = API_ROUTES.filter(
		(route) => true !== route.internal && (0 < route.screens.length || 0 < route.commands.length)
	).length;

	function listOrNone(values: readonly string[]): string {
		return 0 === values.length ? t('about.parity.none') : values.join(', ');
	}

	const statusQuery = createQuery(() => ({
		key: ['admin', 'status'],
		fetch: () => loadStatus(),
		enabled: () => true === session.isAdmin,
		staleMs: STATUS_STALE_MS
	}));

	const { class: className, ...rest }: Props = $props();

	const caps = $derived(capabilities.data);
	const capsError = $derived(capabilities.error);
	const statusOutcome = $derived(true === session.isAdmin ? statusQuery.data : undefined);
	const adminStatus = $derived(undefined !== statusOutcome && statusOutcome.ok ? statusOutcome.value : null);
	const statusError = $derived(
		(true === session.isAdmin ? statusQuery.error : null) ?? (undefined !== statusOutcome && false === statusOutcome.ok ? statusOutcome.error : null)
	);
	const partial = $derived(null !== caps && null !== statusError);

	const versionItems = $derived<DescriptionItem[]>(
		null === caps
			? []
			: [
					{ term: t('about.field.version'), value: caps.version },
					{ term: t('about.field.commit'), value: caps.commit, copy: caps.commit },
					{ term: t('about.field.build_date'), value: formatAbsolute(caps.build_date) },
					{ term: t('about.field.go_version'), value: caps.go_version },
					{ term: t('about.field.server_id'), value: caps.instance_id, copy: caps.instance_id },
					...(null === adminStatus
						? []
						: [
								{
									term: t('about.field.uptime'),
									value: formatDuration(adminStatus.uptime_s * MS_PER_SECOND)
								}
							])
				]
	);

	const featureRows = $derived(
		null === caps ? [] : FEATURE_ROWS.map((row) => ({ id: row.id, on: true === caps.features[row.flag] }))
	);

	const licenses = LICENSE_IDS.map((id) => t(`about.license.${id}`));

	function retry(): void {
		void capabilities.load();
		void statusQuery.refetch();
	}

	$effect(() => {
		void capabilities.load();
	});
</script>

<PageFrame title={t('about.title')} job={t('about.job')} class={className} {...rest}>
	{#if null !== capsError}
		<ErrorState error={capsError} variant="region" onretry={retry} />
	{/if}
	{#if partial}
		<InlineAlert tone="warn" title={t('about.partial.title')} body={t('about.partial.body')}>
			{#snippet actions()}
				<Button variant="secondary" size="sm" onclick={retry}>{t('about.partial.retry')}</Button>
			{/snippet}
		</InlineAlert>
	{/if}
	<div class="about-grid">
		<SettingsCard title={t('about.section.version')} description={t('settings-screens.about.version.desc')}>
			{#if null === caps && null === capsError}
				<Skeleton shape="text" lines={4} />
			{:else if null !== caps}
				<DescriptionList items={versionItems} columns={2} />
			{/if}
		</SettingsCard>
		<SettingsCard title={t('about.section.features')} description={t('settings-screens.about.features.desc')} flush>
			{#if null === caps && null === capsError}
				<div class="pad"><Skeleton shape="text" lines={4} /></div>
			{:else if null !== caps}
				<ul class="list">
					{#each featureRows as row (row.id)}
						<SettingsItem label={t(`about.feature.${row.id}`)}>
							{#snippet trail()}
								<Badge tone={row.on ? 'ok' : 'neutral'}>
									{row.on ? t('about.feature.on') : t('about.feature.off')}
								</Badge>
							{/snippet}
						</SettingsItem>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
	</div>
	<SettingsCard
		title={t('settings-screens.about.parity.title')}
		description={t('settings-screens.about.parity.summary', {
			covered: parityCovered,
			total: parityRows.length - parityInternal,
			internal: parityInternal
		})}
		flush
	>
		<details class="parity">
			<summary>{t('settings-screens.about.parity.show', { count: parityRows.length })}</summary>
			<Scroll axis="x" label={t('about.parity.region')}>
				<table class="parity-table">
					<thead>
						<tr>
							<th scope="col">{t('about.parity.route')}</th>
							<th scope="col">{t('about.parity.screens')}</th>
							<th scope="col">{t('about.parity.commands')}</th>
						</tr>
					</thead>
					<tbody>
						{#each parityRows as row (row.id)}
							<tr>
								<th scope="row"><span class="method">{row.method}</span> <span class="path">{row.path}</span></th>
								<td>{row.screens}</td>
								<td>{row.commands}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</Scroll>
		</details>
	</SettingsCard>
	<SettingsCard title={t('about.section.licenses')} description={t('settings-screens.about.licenses.desc')} flush>
		<ul class="list">
			{#each licenses as license (license)}
				<SettingsItem label={license} />
			{/each}
		</ul>
	</SettingsCard>
</PageFrame>

<style>
	.about-grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-6);
		align-items: start;
	}

	@media (min-width: 1100px) {
		.about-grid {
			grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
		}
	}

	.list {
		display: flex;
		flex-direction: column;
		margin: 0;
		padding: var(--space-1) 0;
		list-style: none;
	}

	.pad {
		padding: var(--card-pad);
	}

	.parity summary {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--touch-target);
		padding-inline: var(--card-pad);
		font-weight: var(--text-label-weight);
		color: var(--text-link);
		cursor: pointer;
		list-style: none;
	}

	.parity summary::-webkit-details-marker {
		display: none;
	}

	.parity summary::before {
		content: '';
		inline-size: var(--space-1-5);
		block-size: var(--space-1-5);
		border-inline-end: var(--hairline) solid currentColor;
		border-block-end: var(--hairline) solid currentColor;
		transform: rotate(-45deg);
		transition: transform var(--dur-micro) var(--ease-standard);
	}

	.parity[open] summary::before {
		transform: rotate(45deg);
	}

	@media (hover: hover) {
		.parity summary:hover {
			background: var(--surface-hover);
		}
	}

	.parity summary:focus-visible {
		outline: var(--focus-width) solid var(--focus-ring-color);
		outline-offset: calc(var(--focus-width) * -1);
	}

	.parity[open] summary {
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.parity-table {
		inline-size: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		text-align: start;
	}

	.parity-table thead th {
		font-weight: var(--text-label-weight);
		color: var(--text-muted);
		background: var(--bg-subtle);
	}

	.parity-table th,
	.parity-table td {
		padding-block: var(--space-2);
		padding-inline: var(--cell-px);
		border-block-end: var(--hairline) solid var(--border-subtle);
		vertical-align: top;
		text-align: start;
	}

	.parity-table th:first-child,
	.parity-table td:first-child {
		padding-inline-start: var(--card-pad);
	}

	.parity-table tbody th {
		font-weight: var(--text-body-weight);
		white-space: nowrap;
	}

	.parity-table tbody tr:last-child > * {
		border-block-end: none;
	}

	.method {
		display: inline-block;
		min-inline-size: calc(var(--ch) * 7);
		font-family: var(--font-mono);
		font-weight: var(--text-label-weight);
		color: var(--text-muted);
	}

	.path {
		font-family: var(--font-mono);
		color: var(--text);
	}
</style>
