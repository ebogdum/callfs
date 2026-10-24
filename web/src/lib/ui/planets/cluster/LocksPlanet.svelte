<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { forceRelease, locks } from '$lib/api/admin/runtime';
	import { getContext, untrack } from 'svelte';
	import { base } from '$app/paths';
	import type { ApiFailure } from '$lib/api/errors';
	import type { Locks, Result } from '$lib/api/types';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { createMutation } from '$lib/data/mutation.svelte';
	import { createQuery } from '$lib/data/query.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Duration from '$lib/ui/atoms/Duration.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import Label from '$lib/ui/atoms/Label.svelte';
	import FieldError from '$lib/ui/atoms/FieldError.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import StaleIndicator from '$lib/ui/molecules/StaleIndicator.svelte';
	import ClusterCard from './ClusterCard.svelte';
	import FactList from './FactList.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import PathInput from '$lib/ui/molecules/PathInput.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import type { ColumnDef } from '$lib/ui/types';

	type LockRow = Locks['held'][number];

	const COMMANDS_CONTEXT = 'callfs.commands';

	const initialPath = page.url.searchParams.get('path') ?? '';
	let filter = $state(initialPath);
	let applied = $state(initialPath);

	async function loadLocks(path: string): Promise<Locks> {
		const outcome: Result<Locks, ApiFailure> = await locks(path);
		return outcome.ok ? outcome.value : Promise.reject(outcome.error);
	}

	const locksQuery = createQuery(() => ({
		key: ['locks', applied],
		fetch: () => loadLocks(applied),
		refetchMs: 2000
	}));

	const releaseMutation = createMutation({
		run: (vars: { key: string; path: string; reason: string }) => forceRelease(vars.key, vars.reason),
		invalidate: () => [['locks']],
		context: { action: 'runtime.locks.release', object: (vars) => vars.path }
	});

	const releaseWord = t('runtime.locks.release.word');
	let releaseOpen = $state(false);
	let target = $state<LockRow | null>(null);
	let reason = $state('');
	let typedWord = $state('');
	let reasonMissing = $state(false);
	let wordWrong = $state(false);

	const now = $derived(locksQuery.updatedAt ?? Date.now());
	const held = $derived(locksQuery.data?.held ?? []);
	const releaseTitle = $derived(
		null === target
			? t('runtime.locks.release')
			: t('runtime.locks.release.title', { path: target.path })
	);

	function typeWord(type: string): string {
		if ('redis' === type) return t('runtime.locks.type.redis');
		if ('nats' === type) return t('runtime.locks.type.nats');
		return t('runtime.locks.type.local');
	}

	function heldMs(lock: LockRow): number {
		return Math.max(0, now - Date.parse(lock.acquired_at));
	}

	function expiresMs(lock: LockRow): number {
		return Math.max(0, Date.parse(lock.expires_at) - now);
	}

	function openRelease(lock: LockRow): void {
		target = lock;
		reason = '';
		typedWord = '';
		reasonMissing = false;
		wordWrong = false;
		releaseOpen = true;
	}

	async function confirmRelease(): Promise<void> {
		if (null === target) {
			return;
		}
		const trimmed = reason.trim();
		reasonMissing = 0 === trimmed.length;
		wordWrong = releaseWord !== typedWord;
		if (reasonMissing || wordWrong) {
			return;
		}
		const result = await releaseMutation.run({ key: target.key, path: target.path, reason: trimmed });
		if (null !== result) {
			releaseOpen = false;
		}
	}

	function submitRelease(event: SubmitEvent): void {
		event.preventDefault();
		void confirmRelease();
	}

	function applyFilter(): void {
		applied = filter.trim();
		const search = 0 === applied.length ? '' : `?${new URLSearchParams({ path: applied }).toString()}`;
		void goto(`${page.url.pathname}${search}`, { replaceState: true, keepFocus: true, noScroll: true });
	}

	function retry(): void {
		void locksQuery.refetch();
	}

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);
	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return untrack(() => registry.register([
			{
				id: 'cluster.locks.force_release',
				title: t('runtime.locks.command.release'),
				group: 'Cluster',
				keywords: [t('runtime.locks.command.keywords')],
				run: () => {
					const first = held[0];
					if (undefined !== first) {
						openRelease(first);
					}
				},
				disabledReason: () => (0 === held.length ? t('runtime.locks.command.none') : null)
			}
		]));
	});

	const rowMenu = (lock: LockRow) => [
		{
			id: 'force-release',
			label: t('runtime.locks.release'),
			icon: 'unlock' as const,
			tone: 'danger' as const,
			onselect: () => openRelease(lock)
		}
	];

	const columns: ColumnDef<LockRow>[] = [
		{ id: 'path', header: t('runtime.locks.col.path'), cell: pathCell, sortKey: 'path' },
		{ id: 'held_for', header: t('runtime.locks.col.held_for'), cell: heldCell, align: 'end' },
		{ id: 'expires_in', header: t('runtime.locks.col.expires_in'), cell: expiresCell, align: 'end' },
		{ id: 'holder', header: t('runtime.locks.col.holder'), cell: holderCell },
		{ id: 'type', header: t('runtime.locks.col.type'), cell: typeCell }
	];
</script>

{#snippet pathCell(lock: LockRow)}
	<Text mono truncate>{lock.path}</Text>
{/snippet}

{#snippet heldCell(lock: LockRow)}
	<Duration value={heldMs(lock)} style="short" />
{/snippet}

{#snippet expiresCell(lock: LockRow)}
	<Duration value={expiresMs(lock)} style="short" />
{/snippet}

{#snippet holderCell(lock: LockRow)}
	<Text mono>{lock.owner_hint}</Text>
{/snippet}

{#snippet typeCell(lock: LockRow)}
	<Text>{typeWord(locksQuery.data?.type ?? 'local')}</Text>
{/snippet}

<PageFrame title={t('runtime.locks.title')} job={t('runtime.locks.job')}>
	{#snippet status()}
		<StaleIndicator
			updatedAt={locksQuery.updatedAt}
			intervalMs={2000}
			state={locksQuery.error ? 'offline' : 'live'}
			onrefresh={retry}
		/>
	{/snippet}
	<Stack gap="8">
		{#if undefined !== locksQuery.data}
			{@const data = locksQuery.data}
			<Stack gap="4">
				{#if locksQuery.error}
					<InlineAlert tone="danger" title={locksQuery.error.title} body={locksQuery.error.body} />
				{/if}
				<KpiStrip
					tiles={[
						{ label: t('runtime.locks.kpi.held'), value: data.held.length, unit: 'count' },
						{ label: t('runtime.locks.kpi.timeouts'), value: data.stats.timeouts, unit: 'count' },
						{ label: t('runtime.locks.kpi.lost'), value: data.stats.lost, unit: 'count' },
						{ label: t('runtime.locks.kpi.wait_p95'), value: data.stats.wait_p95_ms, unit: 'ms', help: null === data.stats.wait_p95_ms ? t('runtime.locks.kpi.wait_p95_none') : undefined }
					]}
				/>
			</Stack>
			<section class="region" aria-labelledby="locks-held">
				<Heading level={2} size="title-sm" id="locks-held">{t('cluster.locks.held.title')}</Heading>
				<DataTable
					rows={held}
					{columns}
					rowKey={(lock: LockRow) => lock.key}
					label={t('runtime.locks.table')}
					status={locksQuery.status}
					error={locksQuery.error ?? undefined}
					{rowMenu}
				>
					{#snippet toolbar()}
						<div class="filter">
							<PathInput bind:value={filter} label={t('runtime.locks.filter')} placeholder={t('runtime.locks.filter.placeholder')} onsubmit={applyFilter} />
						</div>
					{/snippet}
					{#snippet empty()}
						<EmptyState variant="cleared" icon="lock" title={t('runtime.locks.none.title')} body={t('runtime.locks.none.body')} />
					{/snippet}
				</DataTable>
			</section>
			<ClusterCard title={t('cluster.locks.settings.title')} description={t('cluster.locks.settings.body')} icon="settings">
				<FactList
					items={[
						{ term: t('cluster.locks.settings.wait'), value: waitValue },
						{ term: t('cluster.locks.settings.type'), value: typeWord(data.type) }
					]}
				/>
				{#snippet footer()}
					<Link href={`${base}/settings/coordination`}>{t('runtime.locks.settings.link')}</Link>
				{/snippet}
			</ClusterCard>
		{:else if locksQuery.error}
			<ErrorState variant="page" error={locksQuery.error} onretry={retry} />
		{:else}
			<Stack gap="4">
				<Skeleton shape="rect" height="var(--space-16)" />
				<Skeleton shape="rect" height="12rem" />
			</Stack>
		{/if}
	</Stack>
</PageFrame>

{#snippet waitValue()}
	<Duration value={locksQuery.data?.wait_timeout_ms ?? 0} />
{/snippet}

<Dialog bind:open={releaseOpen} title={releaseTitle} size="sm">
	<form onsubmit={submitRelease}>
		<Stack gap="4">
			<Text>{t('runtime.locks.release.body')}</Text>
			<Stack gap="1-5">
				<Label for="release-reason">{t('runtime.locks.release.reason')}</Label>
				<Input id="release-reason" bind:value={reason} invalid={reasonMissing} aria-describedby="release-reason-help release-reason-error" />
				<Text id="release-reason-help" tone="muted" size="sm">{t('runtime.locks.release.reason_help')}</Text>
				{#if reasonMissing}
					<FieldError id="release-reason-error" message={t('runtime.locks.release.reason_required')} />
				{/if}
			</Stack>
			<Stack gap="1-5">
				<Label for="release-word">{t('runtime.locks.release.word_label', { word: releaseWord })}</Label>
				<Input id="release-word" bind:value={typedWord} mono invalid={wordWrong} aria-describedby="release-word-error" />
				{#if wordWrong}
					<FieldError id="release-word-error" message={t('runtime.locks.release.word_mismatch', { word: releaseWord })} />
				{/if}
			</Stack>
			{#if releaseMutation.error}
				<InlineAlert tone="danger" title={releaseMutation.error.title} body={releaseMutation.error.body} />
			{/if}
			<Cluster gap="2" justify="end">
				<Button variant="ghost" onclick={() => (releaseOpen = false)}>{t('runtime.locks.release.cancel')}</Button>
				<Button type="submit" variant="danger" loading={releaseMutation.pending}>{t('runtime.locks.release.confirm')}</Button>
			</Cluster>
		</Stack>
	</form>
</Dialog>

<style>
	.region {
		display: grid;
		gap: var(--space-3);
	}

	.filter {
		inline-size: min(100%, calc(var(--space-4) * 22));
	}
</style>
