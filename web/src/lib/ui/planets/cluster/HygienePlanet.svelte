<script lang="ts">
	import {
		cleanStaging,
		hygiene,
		migrations,
		retag,
		staging,
		type JobAccepted,
		type StagingList
	} from '$lib/api/admin/topology';
	import { reassignOwner } from '$lib/api/admin/users';
	import { toUiError, type ApiFailure } from '$lib/api/errors';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery, type Query } from '$lib/data/query.svelte';
	import type { Hygiene, Migrations, Result } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatDuration } from '$lib/format/duration';
	import { formatNumber } from '$lib/format/number';
	import type { UiError } from '$lib/ui/types';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Badge from '$lib/ui/atoms/Badge.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList from './FactList.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import FormField from '$lib/ui/molecules/FormField.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';

	type Shown<T> = { value: T | null; error: UiError | null };
	type Feedback = { tone: 'ok' | 'danger'; title: string; body?: string };

	const LOAD_ACTION = 'hygiene.action.load';
	const CLEAN_ACTION = 'hygiene.action.clean';
	const ASSIGN_ACTION = 'hygiene.action.assign';
	const TAG_ACTION = 'hygiene.action.tag';
	const OLDER_THAN_SECONDS = 86400;
	const ROOT_PREFIX = '/';

	const NOTHING_SHOWN = { value: null, error: null } as const;

	const hygieneQuery: Query<Shown<Hygiene>> = createQuery<Shown<Hygiene>>({
		key: ['hygiene'],
		fetch: async () => settle(hygieneQuery.data ?? NOTHING_SHOWN, await hygiene(), LOAD_ACTION)
	});

	const stagingQuery: Query<Shown<StagingList>> = createQuery<Shown<StagingList>>({
		key: ['staging'],
		fetch: async () => settle(stagingQuery.data ?? NOTHING_SHOWN, await staging(), LOAD_ACTION)
	});

	const migrationsQuery: Query<Shown<Migrations>> = createQuery<Shown<Migrations>>({
		key: ['migrations'],
		fetch: async () => settle(migrationsQuery.data ?? NOTHING_SHOWN, await migrations(), LOAD_ACTION)
	});

	const hygieneShown = $derived<Shown<Hygiene>>(hygieneQuery.data ?? NOTHING_SHOWN);
	const stagingShown = $derived<Shown<StagingList>>(stagingQuery.data ?? NOTHING_SHOWN);
	const migrationsShown = $derived<Shown<Migrations>>(migrationsQuery.data ?? NOTHING_SHOWN);
	let feedback = $state<Feedback | null>(null);
	let cleanOpen = $state(false);
	let ownerOpen = $state(false);
	let ownerDraft = $state('');
	let ownerError = $state<string | null>(null);

	const hygieneReport = $derived(hygieneShown.value);
	const staged = $derived(stagingShown.value);
	const schema = $derived(migrationsShown.value);
	const staleError = $derived(hygieneShown.error ?? stagingShown.error ?? migrationsShown.error);
	const hasAnyValue = $derived(null !== hygieneReport || null !== staged || null !== schema);
	const stagingCount = $derived(staged?.items.length ?? 0);
	const oldestAgeS = $derived(staged ? staged.items.reduce((max, item) => Math.max(max, item.age_s), 0) : 0);
	const canClean = $derived(0 < stagingCount && OLDER_THAN_SECONDS <= oldestAgeS);
	const ownerCount = $derived(hygieneReport?.files_without_owner ?? 0);
	const tagCount = $derived(hygieneReport?.files_without_etag ?? 0);
	const orphanPieces = $derived(hygieneReport?.orphan_pieces ?? null);
	const staleRecords = $derived(hygieneReport?.stale_erasure_records ?? null);
	const orphanCounted = $derived(null !== orphanPieces || null !== staleRecords);
	const orphanTotal = $derived((orphanPieces ?? 0) + (staleRecords ?? 0));
	const orphanItems = $derived([
		...(null === staleRecords ? [] : [{ term: t('hygiene.term.split_records'), value: formatNumber(staleRecords) }]),
		...(null === orphanPieces ? [] : [{ term: t('hygiene.term.pieces'), value: formatNumber(orphanPieces) }])
	]);

	const stagingItems = $derived(
		staged
			? [
					{ term: t('hygiene.term.files'), value: formatNumber(staged.items.length) },
					{ term: t('hygiene.term.size'), value: formatBytes(staged.total_bytes).text },
					...(0 < staged.items.length
						? [{ term: t('hygiene.term.oldest'), value: formatDuration(oldestAgeS * 1000) }]
						: [])
				]
			: []
	);

	const schemaItems = $derived(
		schema && 'postgres' === schema.driver
			? [
					{
						term: t('hygiene.term.version'),
						value:
							null === schema.current_version
								? t('hygiene.schema.no_version')
								: formatNumber(schema.current_version)
					},
					{
						term: t('hygiene.term.available'),
						value:
							0 === schema.available.length
								? t('hygiene.schema.none_available')
								: schema.available
										.map((migration) =>
											t('hygiene.schema.available', {
												version: formatNumber(migration.version),
												name: migration.name
											})
										)
										.join(', ')
					}
				]
			: []
	);

	const clean = createMutation<number, Result<JobAccepted, ApiFailure>>({
		run: (olderThanS) => cleanStaging(olderThanS),
		invalidate: () => [['staging'], ['hygiene']],
		context: { action: CLEAN_ACTION }
	});

	const assign = createMutation<string, Result<JobAccepted, ApiFailure>>({
		run: (owner) => reassignOwner('', owner, ROOT_PREFIX),
		invalidate: () => [['hygiene']],
		context: { action: ASSIGN_ACTION }
	});

	const tagging = createMutation<string, Result<JobAccepted, ApiFailure>>({
		run: (prefix) => retag(prefix),
		invalidate: () => [['hygiene']],
		context: { action: TAG_ACTION }
	});

	function settle<T>(previous: Shown<T>, outcome: Result<T, ApiFailure>, action: string): Shown<T> {
		if (outcome.ok) {
			return { value: outcome.value, error: null };
		}
		return { value: previous.value, error: toUiError(outcome.error, { action, read: true }) };
	}

	function feedbackFor(outcome: Result<JobAccepted, ApiFailure>, success: string, action: string): Feedback {
		if (outcome.ok) {
			return { tone: 'ok', title: success };
		}
		const error = toUiError(outcome.error, { action });
		return { tone: 'danger', title: error.title, body: error.body };
	}

	async function cleanUp(): Promise<void> {
		cleanOpen = false;
		const outcome = await clean.run(OLDER_THAN_SECONDS);
		if (null !== outcome) {
			feedback = feedbackFor(outcome, t('hygiene.clean.started'), CLEAN_ACTION);
		}
	}

	async function assignOwner(): Promise<void> {
		const owner = ownerDraft.trim();
		if (0 === owner.length) {
			ownerError = t('hygiene.owner.field.required');
			return;
		}
		ownerError = null;
		const outcome = await assign.run(owner);
		if (null === outcome) {
			return;
		}
		ownerOpen = false;
		ownerDraft = '';
		feedback = feedbackFor(outcome, t('hygiene.owner.started'), ASSIGN_ACTION);
	}

	async function addTags(): Promise<void> {
		const outcome = await tagging.run(ROOT_PREFIX);
		if (null !== outcome) {
			feedback = feedbackFor(outcome, t('hygiene.tag.started'), TAG_ACTION);
		}
	}

	function refetchHygiene(): Promise<void> {
		return hygieneQuery.refetch();
	}

	function refetchStaging(): Promise<void> {
		return stagingQuery.refetch();
	}

	function refetchMigrations(): Promise<void> {
		return migrationsQuery.refetch();
	}
</script>

{#snippet fallback(error: UiError | null, retry: () => Promise<void>)}
	{#if error}
		<ErrorState error={error} onretry={retry} variant="region" />
	{:else}
		<Skeleton shape="text" lines={2} />
	{/if}
{/snippet}

{#snippet stateBadge(count: number)}
	{#if 0 === count}
		<Badge tone="ok" icon="check">{t('cluster.hygiene.clear')}</Badge>
	{:else}
		<Badge tone="warn" icon="warn">{t('cluster.hygiene.attention')}</Badge>
	{/if}
{/snippet}

{#snippet figure(value: string, label: string)}
	<p class="figure">
		<span class="figure-value">{value}</span>
		<span class="figure-label">{label}</span>
	</p>
{/snippet}

{#snippet stagingBadge()}
	{@render stateBadge(stagingCount)}
{/snippet}

{#snippet stagingFooter()}
	<Text size="sm" tone="muted">
		{0 === stagingCount ? t('hygiene.staging.none') : canClean ? t('cluster.hygiene.staging.old') : t('cluster.hygiene.staging.recent')}
	</Text>
{/snippet}

{#snippet ownerBadge()}
	{@render stateBadge(ownerCount)}
{/snippet}

{#snippet ownerFooter()}
	{#if 0 < ownerCount}
		<Button variant="secondary" size="sm" iconStart="user" onclick={() => (ownerOpen = true)}>
			{t('hygiene.owner.assign')}
		</Button>
	{:else}
		<Text size="sm" tone="muted">{t('hygiene.owner.none')}</Text>
	{/if}
{/snippet}

{#snippet tagBadge()}
	{@render stateBadge(tagCount)}
{/snippet}

{#snippet tagFooter()}
	{#if 0 < tagCount}
		<Button variant="secondary" size="sm" iconStart="tag" loading={tagging.pending} onclick={addTags}>
			{t('hygiene.tag.run')}
		</Button>
	{:else}
		<Text size="sm" tone="muted">{t('hygiene.tag.none')}</Text>
	{/if}
{/snippet}

{#snippet orphanBadge()}
	{@render stateBadge(orphanTotal)}
{/snippet}

{#snippet orphanFooter()}
	<Text size="sm" tone="muted">{t('hygiene.orphan.none')}</Text>
{/snippet}

<PageFrame title={t('hygiene.title')} job={t('hygiene.job')}>
	{#snippet primary()}
		{#if canClean}
			<Button variant="primary" iconStart="trash" loading={clean.pending} onclick={() => (cleanOpen = true)}>
				{t('hygiene.clean.label')}
			</Button>
		{/if}
	{/snippet}
	<Stack gap="6">
		{#if feedback}
			<InlineAlert tone={feedback.tone} title={feedback.title} body={feedback.body} />
		{/if}
		{#if staleError && hasAnyValue}
			<InlineAlert tone="danger" title={staleError.title} body={staleError.body} />
		{/if}
		<div class="cards">
			<ClusterCard
				title={t('hygiene.staging.title')}
				description={t('hygiene.staging.body')}
				icon="clock"
				tone={0 < stagingCount ? 'warn' : 'neutral'}
				meta={null === staged ? undefined : stagingBadge}
				footer={null === staged ? undefined : stagingFooter}
			>
				{#if null === staged}
					{@render fallback(stagingShown.error, refetchStaging)}
				{:else}
					{@render figure(formatNumber(stagingCount), t('cluster.hygiene.unit.files', { count: stagingCount }))}
					{#if 0 < stagingCount}
						<FactList items={stagingItems.slice(1)} />
					{/if}
				{/if}
			</ClusterCard>
			<ClusterCard
				title={t('hygiene.owner.title')}
				description={t('hygiene.owner.body')}
				icon="user"
				tone={0 < ownerCount ? 'warn' : 'neutral'}
				meta={null === hygieneReport ? undefined : ownerBadge}
				footer={null === hygieneReport ? undefined : ownerFooter}
			>
				{#if null === hygieneReport}
					{@render fallback(hygieneShown.error, refetchHygiene)}
				{:else}
					{@render figure(formatNumber(ownerCount), t('cluster.hygiene.unit.files', { count: ownerCount }))}
				{/if}
			</ClusterCard>
			<ClusterCard
				title={t('hygiene.tag.title')}
				description={t('hygiene.tag.body')}
				icon="tag"
				tone={0 < tagCount ? 'warn' : 'neutral'}
				meta={null === hygieneReport ? undefined : tagBadge}
				footer={null === hygieneReport ? undefined : tagFooter}
			>
				{#if null === hygieneReport}
					{@render fallback(hygieneShown.error, refetchHygiene)}
				{:else}
					{@render figure(formatNumber(tagCount), t('cluster.hygiene.unit.files', { count: tagCount }))}
				{/if}
			</ClusterCard>
			<ClusterCard
				title={t('hygiene.orphan.title')}
				description={t('hygiene.orphan.body')}
				icon="pieces"
				tone={0 < orphanTotal ? 'warn' : 'neutral'}
				meta={null !== hygieneReport && orphanCounted ? orphanBadge : undefined}
				footer={null !== hygieneReport && orphanCounted && 0 === orphanTotal ? orphanFooter : undefined}
			>
				{#if null === hygieneReport}
					{@render fallback(hygieneShown.error, refetchHygiene)}
				{:else if orphanCounted}
					{#if null === orphanPieces && null !== staleRecords}
						{@render figure(formatNumber(staleRecords), t('cluster.hygiene.unit.records', { count: staleRecords }))}
					{:else if null === staleRecords && null !== orphanPieces}
						{@render figure(formatNumber(orphanPieces), t('cluster.hygiene.unit.pieces', { count: orphanPieces }))}
					{:else}
						<FactList items={orphanItems} />
					{/if}
				{:else}
					<Text size="sm" tone="muted">{t('hygiene.orphan.not_counted')}</Text>
				{/if}
			</ClusterCard>
			<ClusterCard
				class="wide"
				title={t('hygiene.schema.title')}
				description={null !== schema && 'none' === schema.driver ? t('hygiene.schema.none') : t('cluster.hygiene.schema.body')}
				icon="database"
				tone={true === schema?.dirty ? 'danger' : 'neutral'}
			>
				{#snippet meta()}
					{#if null !== schema && 'none' !== schema.driver}
						{#if schema.dirty}
							<Badge tone="danger" icon="danger">{t('cluster.hygiene.schema.dirty')}</Badge>
						{:else}
							<Badge tone="ok" icon="check">{t('cluster.hygiene.schema.ok')}</Badge>
						{/if}
					{:else if null !== schema}
						<Badge>{t('cluster.hygiene.schema.not_used')}</Badge>
					{/if}
				{/snippet}
				{#if null === schema}
					{@render fallback(migrationsShown.error, refetchMigrations)}
				{:else if 'none' !== schema.driver}
					{#if schema.dirty}
						<InlineAlert tone="danger" title={t('hygiene.schema.dirty.title')} body={t('hygiene.schema.dirty.body')} />
					{/if}
					<FactList items={schemaItems} />
				{/if}
			</ClusterCard>
		</div>
	</Stack>
</PageFrame>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, calc(var(--space-4) * 20)), 1fr));
		gap: var(--space-4);
	}

	.cards > :global(.wide) {
		grid-column: 1 / -1;
	}

	.figure {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
		margin: 0;
	}

	.figure-value {
		font-size: var(--text-heading-size);
		line-height: var(--text-heading-line);
		font-weight: var(--text-heading-weight);
		letter-spacing: var(--text-heading-tracking);
		font-variant-numeric: tabular-nums;
		color: var(--text);
	}

	.figure-label {
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}
</style>

<Dialog bind:open={ownerOpen} title={t('hygiene.owner.dialog.title')} description={t('hygiene.owner.dialog.body')} size="sm">
	<FormField label={t('hygiene.owner.field.label')} for="owner-draft" help={t('hygiene.owner.field.help')} error={ownerError ?? undefined}>
		{#snippet control({ describedBy, invalid })}
			<Input id="owner-draft" aria-describedby={describedBy} {invalid} bind:value={ownerDraft} autocomplete="off" />
		{/snippet}
	</FormField>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (ownerOpen = false)}>{t('hygiene.owner.cancel')}</Button>
			<Button variant="primary" loading={assign.pending} onclick={assignOwner}>{t('hygiene.owner.confirm')}</Button>
		</Cluster>
	{/snippet}
</Dialog>

<AlertDialog
	bind:open={cleanOpen}
	title={t('hygiene.clean.title')}
	body={t('hygiene.clean.body')}
	confirmLabel={t('hygiene.clean.confirm')}
	tone="danger"
	onconfirm={cleanUp}
/>
