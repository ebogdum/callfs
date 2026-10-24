<script lang="ts" module>
	import type { ConfigView, Result } from '$lib/api/types';
	import type { UiError } from '$lib/ui/types';

	export function valueOf<T>(result: Result<T, UiError> | null): T | null {
		return null !== result && true === result.ok ? result.value : null;
	}

	export function errorOf<T>(result: Result<T, UiError> | null): UiError | null {
		return null !== result && false === result.ok ? result.error : null;
	}

	export function entryOf(view: ConfigView | null, path: string): ConfigView['keys'][number] | null {
		if (null === view) {
			return null;
		}
		return view.keys.find((entry) => path === entry.path) ?? null;
	}
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getConfig, save, type ConfigChanges } from '$lib/api/admin/config';
	import { findings, type FindingPage } from '$lib/api/admin/findings';
	import { status as loadStatus } from '$lib/api/admin/status';
	import { tls } from '$lib/api/admin/users';
	import { toUiError } from '$lib/api/errors';
	import { linkStats } from '$lib/api/links';
	import type { AdminStatus, Finding, LinkStats, TlsInfo } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatAbsolute } from '$lib/format/time';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Badge from '$lib/ui/atoms/Badge.svelte';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Switch from '$lib/ui/atoms/Switch.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import DescriptionList from '$lib/ui/molecules/DescriptionList.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import SecretInput from '$lib/ui/molecules/SecretInput.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import CodeView from '$lib/ui/organisms/CodeView.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import StatusGlyph from '$lib/ui/quarks/StatusGlyph.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import SettingsCard from './SettingsCard.svelte';
	import SettingsItem from './SettingsItem.svelte';

	type CheckTone = 'ok' | 'warn' | 'danger' | 'info' | 'unused';

	interface Check {
		id: string;
		label: string;
		findingIds: string[];
		route: string;
	}

	interface CheckRow {
		id: string;
		label: string;
		tone: CheckTone;
		word: string;
		detail: string;
		route: string;
	}

	interface SecurityData {
		tls: Result<TlsInfo, UiError>;
		findings: Result<FindingPage, UiError>;
		config: Result<ConfigView, UiError>;
		links: Result<LinkStats, UiError>;
		status?: Result<AdminStatus, UiError>;
	}

	interface Outcome {
		tone: 'ok' | 'danger';
		title: string;
		body?: string;
	}

	interface PendingChange {
		title: string;
		body: string;
		confirmLabel: string;
		tone: 'danger' | 'warn';
		typedWord?: string;
		changes: ConfigChanges;
	}

	const MIN_SECRET_LENGTH = 16;
	const INTERNAL_SECRET = 'auth.internal_proxy_secret';
	const LINK_SECRET = 'auth.single_use_link_secret';
	const VERIFY_KEY = 'backend.internal_proxy_skip_tls_verify';
	const SECTION_COUNT = 4;

	const severityOrder: Finding['severity'][] = ['critical', 'warning', 'info'];
	const toneOfSeverity: Record<Finding['severity'], CheckTone> = { critical: 'danger', warning: 'warn', info: 'info' };
	const wordOf: Record<CheckTone, string> = {
		ok: t('security.word.ok'),
		warn: t('security.word.warn'),
		danger: t('security.word.danger'),
		info: t('security.word.info'),
		unused: t('security.word.unused')
	};

	const checks: Check[] = [
		{ id: 'https', label: t('security.check.https.label'), findingIds: ['tls.plain_http'], route: '/settings/network' },
		{
			id: 'cert',
			label: t('security.check.cert.label'),
			findingIds: ['tls.cert_expiring', 'tls.cert_expired'],
			route: '/settings/security'
		},
		{ id: 'key', label: t('security.check.key.label'), findingIds: ['tls.key_world_readable'], route: '/settings/security' },
		{ id: 'metrics', label: t('security.check.metrics.label'), findingIds: ['metrics.public_port'], route: '/settings/network' },
		{
			id: 'internal',
			label: t('security.check.internal.label'),
			findingIds: ['peers.http', 'peers.skip_verify'],
			route: '/settings/security'
		},
		{ id: 'cluster', label: t('security.check.cluster.label'), findingIds: ['nats.plain_transport'], route: '/settings/security' },
		{ id: 'index', label: t('security.check.index.label'), findingIds: ['index.sslmode_disable'], route: '/settings/index' },
		{ id: 'redis', label: t('security.check.redis.label'), findingIds: ['redis.no_password'], route: '/settings/coordination' },
		{ id: 's3_acl', label: t('security.check.s3_acl.label'), findingIds: ['s3.public_acl'], route: '/settings/storage' },
		{ id: 's3_sse', label: t('security.check.s3_sse.label'), findingIds: ['s3.no_encryption'], route: '/settings/storage' },
		{
			id: 'secret_default',
			label: t('security.check.secret_default.label'),
			findingIds: ['secret.default'],
			route: '/settings/security'
		},
		{
			id: 'secret_length',
			label: t('security.check.secret_length.label'),
			findingIds: ['secret.short', 'secret.same'],
			route: '/settings/security'
		},
		{ id: 'headers', label: t('security.check.headers.label'), findingIds: [], route: '/settings/security' }
	];

	async function loadTls(): Promise<Result<TlsInfo, UiError>> {
		const result = await tls();
		return true === result.ok ? result : { ok: false, error: toUiError(result.error, { action: 'security.load' }) };
	}

	async function loadSecurity(): Promise<SecurityData> {
		const [tlsResult, findingsResult, configResult, linksResult, statusResult] = await Promise.all([
			loadTls(),
			findings('security'),
			getConfig(),
			linkStats('24h'),
			loadStatus()
		]);
		return { tls: tlsResult, findings: findingsResult, config: configResult, links: linksResult, status: statusResult };
	}

	const securityQuery = createQuery({ key: ['security-page'], fetch: loadSecurity });

	const data = $derived(securityQuery.data);
	const tlsResult = $derived(undefined === data ? null : data.tls);
	const findingsResult = $derived(undefined === data ? null : data.findings);
	const configResult = $derived(undefined === data ? null : data.config);
	const linksResult = $derived(undefined === data ? null : data.links);
	const adminStatus = $derived(valueOf(data?.status ?? null));
	const tlsInfo = $derived(valueOf(tlsResult));
	const configView = $derived(valueOf(configResult));
	const findingList = $derived(valueOf(findingsResult)?.items ?? []);
	const linkCount = $derived(valueOf(linksResult)?.active ?? 0);
	const tlsError = $derived(errorOf(tlsResult));
	const findingsError = $derived(errorOf(findingsResult));
	const configError = $derived(errorOf(configResult));
	const failures = $derived(
		[tlsError, findingsError, configError, errorOf(linksResult)].filter((error): error is UiError => null !== error)
	);
	const allFailed = $derived(SECTION_COUNT === failures.length);
	const leadFailure = $derived(failures[0] ?? null);
	const checklistError = $derived(tlsError ?? findingsError);

	function sortBySeverity(list: Finding[]): Finding[] {
		return [...list].sort((a, b) => severityOrder.indexOf(a.severity) - severityOrder.indexOf(b.severity));
	}

	function worstFinding(list: Finding[], ids: string[]): Finding | null {
		return sortBySeverity(list.filter((finding) => ids.includes(finding.id)))[0] ?? null;
	}

	function gapOf(id: string, info: TlsInfo): boolean {
		if ('headers' === id) {
			return '' === info.headers.csp;
		}
		if ('cert' === id) {
			return null === info.cert;
		}
		return false;
	}

	function gapDetail(id: string): string {
		return 'cert' === id ? t('security.check.detail.cert_none') : t('security.check.detail.headers_missing');
	}

	function okDetail(id: string, info: TlsInfo): string {
		if ('cert' === id && null !== info.cert) {
			return t('security.check.detail.cert', {
				date: formatAbsolute(info.cert.not_after),
				days: info.cert.days_remaining
			});
		}
		return t('security.check.detail.ok');
	}

	function isUnused(id: string, state: AdminStatus | null): boolean {
		if (null === state) {
			return false;
		}
		if ('redis' === id) {
			return 'redis' !== state.file_index && 'redis' !== state.lock_type;
		}
		if ('s3_acl' === id || 's3_sse' === id) {
			return false === state.storages.s3;
		}
		return false;
	}

	function rowFor(check: Check, list: Finding[], info: TlsInfo, state: AdminStatus | null): CheckRow {
		if (isUnused(check.id, state)) {
			return {
				id: check.id,
				label: check.label,
				tone: 'unused',
				word: wordOf.unused,
				detail: t('security.check.detail.unused'),
				route: check.route
			};
		}
		const finding = worstFinding(list, check.findingIds);
		if (null !== finding) {
			const tone = toneOfSeverity[finding.severity];
			return { id: check.id, label: check.label, tone, word: wordOf[tone], detail: finding.title, route: finding.fix.route };
		}
		const gap = gapOf(check.id, info);
		const tone: CheckTone = gap ? 'warn' : 'ok';
		return {
			id: check.id,
			label: check.label,
			tone,
			word: wordOf[tone],
			detail: gap ? gapDetail(check.id) : okDetail(check.id, info),
			route: check.route
		};
	}

	const checkRows = $derived(null === tlsInfo ? [] : checks.map((check) => rowFor(check, findingList, tlsInfo, adminStatus)));
	const badgeTone: Record<CheckTone, 'ok' | 'warn' | 'danger' | 'info' | 'neutral'> = {
		ok: 'ok',
		warn: 'warn',
		danger: 'danger',
		info: 'info',
		unused: 'neutral'
	};
	const attentionCount = $derived(checkRows.filter((row) => 'ok' !== row.tone && 'info' !== row.tone && 'unused' !== row.tone).length);
	const checklistSummary = $derived(
		null === tlsInfo
			? undefined
			: 0 === attentionCount
			? t('settings-screens.security.checklist.ok')
			: t('settings-screens.security.checklist.bad', { count: attentionCount })
	);
	const topFinding = $derived(sortBySeverity(findingList)[0] ?? null);
	const internalSet = $derived(true === entryOf(configView, INTERNAL_SECRET)?.set);
	const linkSet = $derived(true === entryOf(configView, LINK_SECRET)?.set);
	const verifyOn = $derived(true !== entryOf(configView, VERIFY_KEY)?.value);
	const certItems = $derived(
		null === tlsInfo
			? []
			: [
					{ term: t('security.config.cert_file'), value: tlsInfo.cert?.path ?? t('security.config.unset') },
					{
						term: t('security.config.key_file'),
						value: tlsInfo.key?.path ?? t('security.config.unset'),
						help: true === tlsInfo.key?.world_readable ? t('security.config.key_readable') : undefined
					}
				]
	);
	const headerCode = $derived(null === tlsInfo ? '' : headerLines(tlsInfo));

	function headerLines(info: TlsInfo): string {
		return [
			`Content-Security-Policy: ${info.headers.csp}`,
			`X-Frame-Options: ${info.headers.frame_options}`,
			`Cross-Origin-Opener-Policy: ${info.headers.coop}`,
			`Cross-Origin-Embedder-Policy: ${info.headers.coep}`
		].join('\n');
	}

	let internalDraft = $state('');
	let linkDraft = $state('');
	let outcome = $state<Outcome | null>(null);
	let pending = $state<PendingChange | null>(null);
	let dialogOpen = $state(false);

	const internalTooShort = $derived(MIN_SECRET_LENGTH > internalDraft.length);
	const linkTooShort = $derived(MIN_SECRET_LENGTH > linkDraft.length);

	function revisionOf(view: ConfigView | null): string {
		if (null === view) {
			return '';
		}
		const { revision } = view as ConfigView & { revision?: string };
		return 'string' === typeof revision ? revision : '';
	}

	async function saveChanges(changes: ConfigChanges): Promise<void> {
		const result = await save(changes, revisionOf(configView));
		if (false === result.ok) {
			outcome = { tone: 'danger', title: result.error.title, body: result.error.body };
			return;
		}
		outcome = {
			tone: 'ok',
			title: 0 < result.value.restart_required.length ? t('security.saved.restart') : t('security.saved.now')
		};
		internalDraft = '';
		linkDraft = '';
		void securityQuery.refetch();
	}

	function askFor(change: PendingChange): void {
		pending = change;
		dialogOpen = true;
	}

	function confirmPending(): void {
		const change = pending;
		pending = null;
		if (null !== change) {
			void saveChanges(change.changes);
		}
	}

	function reviewInternal(): void {
		askFor({
			title: t('security.secret.dialog.title', { name: t('security.secret.internal.label') }),
			body: t('security.secret.internal.help'),
			confirmLabel: t('security.secret.dialog.confirm'),
			tone: 'danger',
			typedWord: t('security.secret.word'),
			changes: { [INTERNAL_SECRET]: internalDraft }
		});
	}

	function reviewLink(): void {
		askFor({
			title: t('security.secret.dialog.title', { name: t('security.secret.link.label') }),
			body: t('security.secret.link.help', { count: linkCount }),
			confirmLabel: t('security.secret.dialog.confirm'),
			tone: 'danger',
			typedWord: t('security.secret.word'),
			changes: { [LINK_SECRET]: linkDraft }
		});
	}

	function onVerifyChange(on: boolean): void {
		const changes: ConfigChanges = { [VERIFY_KEY]: !on };
		if (on) {
			void saveChanges(changes);
			return;
		}
		askFor({
			title: t('security.config.verify.off_title'),
			body: t('security.config.verify.help'),
			confirmLabel: t('security.config.verify.confirm'),
			tone: 'danger',
			changes
		});
	}

	function hrefOf(route: string): string {
		return route.startsWith(base) ? route : `${base}${route}`;
	}

	function fixTop(route: string): void {
		void goto(hrefOf(route));
	}

	function retry(): void {
		void securityQuery.refetch();
	}
</script>

<PageFrame title={t('security.title')} job={t('security.job')}>
	{#snippet primary()}
		{#if null !== topFinding && 'info' !== topFinding.severity}
			<Button variant="primary" iconStart="shield" onclick={() => fixTop(topFinding.fix.route)}>{t('security.fix_top')}</Button>
		{/if}
	{/snippet}
	{#if null !== securityQuery.error}
		<ErrorState variant="page" error={securityQuery.error} onretry={retry} />
	{:else if undefined === data}
		<Skeleton shape="rect" height="12rem" />
		<Skeleton shape="rect" height="8rem" />
	{:else if allFailed && null !== leadFailure}
		<ErrorState variant="page" error={leadFailure} onretry={retry} />
	{:else}
		{#if 0 < failures.length}
			<InlineAlert tone="warn" title={t('security.partial.title')} body={t('security.partial.body')} />
		{/if}
		{#if null !== outcome}
			<InlineAlert tone={outcome.tone} title={outcome.title} body={outcome.body} />
		{/if}
		<SettingsCard title={t('security.checklist.title')} description={checklistSummary} flush>
			{#if null !== checklistError}
				<div class="pad">
					<ErrorState variant="region" error={checklistError} onretry={retry} />
				</div>
			{:else if null !== tlsInfo}
				<ul class="list" aria-label={t('security.checklist.list')}>
					{#each checkRows as row (row.id)}
						<SettingsItem label={row.label} detail={row.detail}>
							{#snippet lead()}
								<StatusGlyph tone={'unused' === row.tone ? 'info' : row.tone} />
							{/snippet}
							{#snippet trail()}
								<Badge tone={badgeTone[row.tone]}>{row.word}</Badge>
								{#if 'ok' !== row.tone && 'unused' !== row.tone}
									<Link href={hrefOf(row.route)} aria-label={`${t('security.check.fix')}: ${row.label}`}>{t('security.check.fix')}</Link>
								{/if}
							{/snippet}
						</SettingsItem>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
		{#if null !== configError}
			<ErrorState variant="region" error={configError} onretry={retry} />
		{:else if null !== configView}
			<SettingsCard title={t('security.secrets.title')} description={t('settings-screens.security.secrets.desc')} flush>
				<ul class="list">
					<li class="secret">
						<SecretInput
							label={t('security.secret.internal.label')}
							isSet={internalSet}
							bind:value={internalDraft}
							generate
							minLength={MIN_SECRET_LENGTH}
						/>
						<Text size="sm" tone="muted">{t('security.secret.internal.help')}</Text>
						{#if 0 < internalDraft.length}
							<Cluster gap="2">
								<Button
									variant="danger"
									disabledReason={internalTooShort ? t('security.secret.too_short') : undefined}
									onclick={reviewInternal}>{t('security.secret.review')}</Button
								>
							</Cluster>
						{/if}
					</li>
					<li class="secret">
						<SecretInput
							label={t('security.secret.link.label')}
							isSet={linkSet}
							bind:value={linkDraft}
							generate
							minLength={MIN_SECRET_LENGTH}
						/>
						<Text size="sm" tone="muted">{t('security.secret.link.help', { count: linkCount })}</Text>
						{#if 0 < linkDraft.length}
							<Cluster gap="2">
								<Button
									variant="danger"
									disabledReason={linkTooShort ? t('security.secret.too_short') : undefined}
									onclick={reviewLink}>{t('security.secret.review')}</Button
								>
							</Cluster>
						{/if}
					</li>
				</ul>
			</SettingsCard>
		{/if}
		{#if null !== tlsInfo}
			<SettingsCard title={t('security.config.title')} description={t('settings-screens.security.config.desc')}>
				<DescriptionList items={certItems} columns={2} />
				{#if null !== configView}
					<Switch
						checked={verifyOn}
						label={t('security.config.verify.label')}
						description={t('security.config.verify.desc')}
						onchange={onVerifyChange}
					/>
				{/if}
				{#if false === verifyOn}
					<InlineAlert tone="danger" title={t('security.config.verify.label')} body={t('security.config.verify.help')} />
				{/if}
				<div class="headers">
					<Text weight={600}>{t('security.headers.label')}</Text>
					<Text size="sm" tone="muted">{t('settings-screens.security.headers.desc')}</Text>
					<div class="header-code">
						<CodeView code={headerCode} language="http" label={t('security.headers.label')} />
					</div>
				</div>
			</SettingsCard>
		{/if}
	{/if}
</PageFrame>

<AlertDialog
	bind:open={dialogOpen}
	title={pending?.title ?? ''}
	body={pending?.body ?? ''}
	confirmLabel={pending?.confirmLabel ?? ''}
	tone={pending?.tone ?? 'danger'}
	typedWord={pending?.typedWord}
	onconfirm={confirmPending}
/>

<style>
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

	.secret {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-4) var(--card-pad);
	}

	.secret + .secret {
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.header-code :global(.line) {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}

	.headers {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-block-start: var(--space-4);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}
</style>
