<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getLink, revokeLink } from '$lib/api/links';
	import type { Link } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { invalidate } from '$lib/data/cache';
	import { createQuery } from '$lib/data/query.svelte';
	import { toasts } from '$lib/stores/toasts.svelte';
	import type { StatusKey, UiError } from '$lib/ui/types';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import IPCell from '$lib/ui/cells/IPCell.svelte';
	import StatusCell from '$lib/ui/cells/StatusCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import DescriptionList, { type DescriptionItem } from '$lib/ui/molecules/DescriptionList.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import Icon from '$lib/ui/quarks/Icon.svelte';
	import Spinner from '$lib/ui/quarks/Spinner.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import Surface from '$lib/ui/quarks/Surface.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import Timeline from '$lib/ui/organisms/Timeline.svelte';

	type TimelineItem = ComponentProps<typeof Timeline>['items'][number];
	type Attempt = NonNullable<Link['attempts']>[number];

	const LINK_STATUS_KEY: Record<Link['status'], StatusKey> = {
		active: 'link.active',
		used: 'link.used',
		expired: 'link.expired',
		revoked: 'link.revoked'
	};

	const ATTEMPT_TITLE: Record<Attempt['result'], string> = {
		success: 'sharing.tl.attempt.success',
		expired: 'sharing.tl.attempt.expired',
		invalid: 'sharing.tl.attempt.invalid',
		not_found: 'sharing.tl.attempt.not_found'
	};

	const ATTEMPT_TONE: Record<Attempt['result'], TimelineItem['tone']> = {
		success: 'ok',
		expired: 'danger',
		invalid: 'danger',
		not_found: 'danger'
	};

	function fileName(path: string): string {
		return path.split('/').filter(Boolean).at(-1) ?? path;
	}

	function parseLinkId(value: string): number | null {
		const numeric = Number(value);
		return Number.isSafeInteger(numeric) && 0 < numeric ? numeric : null;
	}

	function timelineFor(entry: Link): TimelineItem[] {
		const items: TimelineItem[] = [{ at: entry.created_at, title: t('sharing.tl.created'), tone: 'neutral' }];
		for (const attempt of entry.attempts ?? []) {
			items.push({
				at: attempt.at,
				title: t(ATTEMPT_TITLE[attempt.result]),
				detail: t('sharing.tl.from', { ip: attempt.ip }),
				tone: ATTEMPT_TONE[attempt.result]
			});
		}
		if ('revoked' === entry.status && undefined !== entry.updated_at) {
			items.push({ at: entry.updated_at, title: t('sharing.tl.revoked'), tone: 'warn' });
		}
		if ('expired' === entry.status) {
			items.push({ at: entry.expires_at, title: t('sharing.tl.expired'), tone: 'neutral' });
		}
		return items.sort((first, second) => Date.parse(second.at) - Date.parse(first.at));
	}

	interface Props {
		id: string;
		class?: string;
	}

	let { id, class: className }: Props = $props();

	const linkId = $derived(parseLinkId(id));

	const linkQuery = createQuery(() => ({
		key: ['link', linkId],
		fetch: () => getLink(Number(id)),
		enabled: () => null !== linkId,
		staleMs: 5000,
		revalidateOnFocus: true
	}));

	$effect(() => linkQuery.attach());

	const linkResult = $derived(linkQuery.data);
	const link = $derived(true === linkResult?.ok ? linkResult.value : null);
	const loadError = $derived.by((): UiError | null => {
		if (null === linkId) {
			return {
				title: t('sharing.detail.invalid.title'),
				body: t('sharing.detail.invalid.body'),
				retryable: false,
				fixAction: { label: t('sharing.detail.back'), href: `${base}/sharing` }
			};
		}
		return false === linkResult?.ok ? linkResult.error : null;
	});
	const timeline = $derived(null === link ? [] : timelineFor(link));
	const facts = $derived.by((): DescriptionItem[] => [
		{ term: t('sharing.col.file'), value: fileFact },
		{ term: t('sharing.col.status'), value: statusFact },
		{ term: t('sharing.col.expires'), value: expiresFact },
		{ term: t('sharing.col.created'), value: createdFact },
		{ term: t('sharing.col.created_by'), value: createdByFact },
		{ term: t('sharing.col.used'), value: usedFact },
		{ term: t('sharing.col.used_from'), value: usedFromFact },
		{ term: t('sharing.col.link_id'), value: linkIdFact }
	]);

	let revokeOpen = $state(false);
	let actionError = $state<UiError | null>(null);

	async function confirmRevoke(): Promise<void> {
		if (null === linkId) {
			return;
		}
		const result = await revokeLink(linkId);
		revokeOpen = false;
		if (false === result.ok) {
			actionError = result.error;
			return;
		}
		actionError = null;
		invalidate(['link', linkId]);
		invalidate(['links']);
		invalidate(['link-stats']);
		toasts.push({ tone: 'ok', title: t('sharing.revoked', { count: 1 }) });
	}

	function navigate(href: string): void {
		goto(`${base}${href}`).catch(() => toasts.push({ tone: 'danger', title: t('sharing.navigate_failed') }));
	}

	function retry(): void {
		void linkQuery.refetch();
	}
</script>

{#snippet fileFact()}
	{#if null !== link}
		<a class="path" href={`${base}/files${link.file_path}`}>{link.file_path}</a>
	{/if}
{/snippet}

{#snippet statusFact()}
	{#if null !== link}<StatusCell status={LINK_STATUS_KEY[link.status]} />{/if}
{/snippet}

{#snippet expiresFact()}
	{#if null !== link}<TimeCell value={link.expires_at} />{/if}
{/snippet}

{#snippet createdFact()}
	{#if null !== link}<TimeCell value={link.created_at} />{/if}
{/snippet}

{#snippet createdByFact()}
	{#if null !== link}<TextCell value={link.created_by} />{/if}
{/snippet}

{#snippet usedFact()}
	{#if null !== link}<TimeCell value={link.used_at} />{/if}
{/snippet}

{#snippet usedFromFact()}
	{#if null !== link}<IPCell value={link.used_by_ip} />{/if}
{/snippet}

{#snippet linkIdFact()}
	{#if null !== link}<TextCell value={'' === link.token_prefix ? t('sharing.none') : link.token_prefix} mono={'' !== link.token_prefix} />{/if}
{/snippet}

<PageFrame title={null === link ? t('sharing.detail.title') : fileName(link.file_path)} job={t('sharing.detail.job')} class={className}>
	{#snippet breadcrumbs()}
		<a class="back" href={`${base}/sharing`}>
			<Icon name="chevron-left" size="sm" />
			{t('sharing.detail.back')}
		</a>
	{/snippet}

	{#snippet secondary()}
		{#if null !== link}
			<Button variant="secondary" iconStart="file" onclick={() => navigate(`/files${link.file_path}`)}>
				{t('sharing.detail.open_file')}
			</Button>
			<Button
				variant="secondary"
				iconStart="link"
				onclick={() => navigate(`/sharing?new=1&path=${encodeURIComponent(link.file_path)}`)}
			>
				{t('sharing.detail.new_link')}
			</Button>
		{/if}
	{/snippet}

	{#snippet primary()}
		{#if null !== link && 'active' === link.status}
			<Button variant="danger" iconStart="link-off" onclick={() => (revokeOpen = true)}>
				{t('sharing.detail.revoke')}
			</Button>
		{/if}
	{/snippet}

	{#if null !== loadError}
		<ErrorState error={loadError} onretry={retry} />
	{:else if null === link}
		<Surface level="raised" pad="card" radius="lg" border>
			<div class="loading"><Spinner label={t('sharing.loading')} /></div>
		</Surface>
	{:else}
		{#if null !== actionError}
			<InlineAlert tone="danger" title={actionError.title} body={actionError.body} />
		{/if}

		<div class="layout">
			<Surface level="raised" pad="card" radius="lg" border>
				<Stack gap="4">
					<Heading level={2} size="title-sm">{t('sharing.detail.facts')}</Heading>
					<DescriptionList items={facts} columns={2} />
				</Stack>
			</Surface>

			<Surface level="raised" pad="card" radius="lg" border>
				<Stack gap="4">
					<Heading level={2} size="title-sm">{t('sharing.detail.timeline')}</Heading>
					{#if undefined === link.attempts}
						<InlineAlert tone="info" title={t('sharing.detail.partial')} />
					{/if}
					<Timeline items={timeline} />
				</Stack>
			</Surface>
		</div>
	{/if}
</PageFrame>

<AlertDialog
	bind:open={revokeOpen}
	title={t('sharing.revoke.title')}
	body={t('sharing.revoke.body')}
	confirmLabel={t('sharing.revoke.confirm')}
	tone="danger"
	onconfirm={() => void confirmRevoke()}
/>

<style>
	.back {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		text-decoration: none;
		border-radius: var(--radius-sm);
	}

	@media (hover: hover) {
		.back:hover {
			color: var(--text);
		}
	}

	.back:focus-visible,
	.path:focus-visible {
		box-shadow: var(--focus-ring);
		outline: none;
	}

	.path {
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		color: var(--text-link);
		overflow-wrap: anywhere;
		text-decoration: none;
		border-radius: var(--radius-sm);
	}

	@media (hover: hover) {
		.path:hover {
			text-decoration: underline;
		}
	}

	.loading {
		display: flex;
		justify-content: center;
		padding-block: var(--space-8);
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-6);
		align-items: start;
	}

	@media (min-width: 1024px) {
		.layout {
			grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
		}
	}
</style>
