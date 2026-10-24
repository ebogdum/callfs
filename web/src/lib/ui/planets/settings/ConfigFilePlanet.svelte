<script lang="ts">
	import { exportConfig, getConfig, validate as validateConfig, type ExportFormat } from '$lib/api/admin/config';
	import { restart, status as loadStatus } from '$lib/api/admin/status';
	import type { Result, ValidateResult } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import { formatNumber } from '$lib/format/number';
	import { formatAbsolute } from '$lib/format/time';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import DescriptionList from '$lib/ui/molecules/DescriptionList.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import Tabs from '$lib/ui/molecules/Tabs.svelte';
	import AlertDialog from '$lib/ui/organisms/AlertDialog.svelte';
	import CodeView from '$lib/ui/organisms/CodeView.svelte';
	import ConfigDiff from '$lib/ui/organisms/ConfigDiff.svelte';
	import { download } from '$lib/ui/organisms/table/export';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import type { UiError } from '$lib/ui/types';
	import { entryOf, errorOf, valueOf } from './SecurityPlanet.svelte';
	import SettingsCard from './SettingsCard.svelte';

	interface Outcome {
		tone: 'ok' | 'danger';
		title: string;
		body?: string;
	}

	const RESTART_EXAMPLE = 'systemctl restart callfs\ndocker restart callfs';

	const tabItems = [
		{ id: 'yaml', label: t('security.file.tab.yaml') },
		{ id: 'env', label: t('security.file.tab.env') },
		{ id: 'json', label: t('security.file.tab.json') }
	];

	function formatOf(id: string): ExportFormat {
		if ('env' === id) {
			return 'env';
		}
		if ('json' === id) {
			return 'json';
		}
		return 'yaml';
	}

	const statusQuery = createQuery({ key: ['security-file', 'status'], fetch: loadStatus });
	const configQuery = createQuery({ key: ['security-file', 'config'], fetch: getConfig });

	let tab = $state('yaml');
	const exportQuery = createQuery(() => ({
		key: ['security-file', 'export', formatOf(tab)],
		fetch: () => exportConfig(formatOf(tab))
	}));

	let validation = $state<Result<ValidateResult, UiError> | null>(null);
	let validating = $state(false);
	let restartOpen = $state(false);
	let restartOutcome = $state<Outcome | null>(null);

	const statusResult = $derived(statusQuery.data ?? null);
	const configResult = $derived(configQuery.data ?? null);
	const exportResult = $derived(exportQuery.data ?? null);
	const statusView = $derived(valueOf(statusResult));
	const configView = $derived(valueOf(configResult));
	const exportText = $derived(valueOf(exportResult));
	const statusError = $derived(errorOf(statusResult));
	const configError = $derived(errorOf(configResult));
	const exportError = $derived(errorOf(exportResult));
	const queryError = $derived(statusQuery.error ?? configQuery.error ?? null);
	const bothFailed = $derived(null !== statusError && null !== configError);
	const restartAllowed = $derived(true === entryOf(configView, 'ui.allow_remote_restart')?.value);
	const pendingKeys = $derived(configView?.pending ?? []);
	const pendingAfter = $derived(
		Object.fromEntries(pendingKeys.map((path) => [path, entryOf(configView, path)?.value ?? null]))
	);
	const secretKeys = $derived(configView?.keys.filter((entry) => entry.secret).map((entry) => entry.path) ?? []);
	const statusItems = $derived(
		null === statusView
			? []
			: [
					{ term: t('security.file.path'), value: statusView.config_path, copy: statusView.config_path },
					{ term: t('security.file.format'), value: statusView.config_format },
					{ term: t('security.file.loaded_at'), value: formatAbsolute(statusView.started_at) },
					{ term: t('security.file.env_count'), value: formatNumber(statusView.keys_from_env.length) },
					{ term: t('security.file.default_count'), value: formatNumber(statusView.keys_at_default.length) }
				]
	);

	function outcomeOf(result: Result<ValidateResult, UiError> | null): Outcome | null {
		if (null === result) {
			return null;
		}
		if (false === result.ok) {
			return { tone: 'danger', title: result.error.title, body: result.error.body };
		}
		if (result.value.valid) {
			return { tone: 'ok', title: t('security.file.validate.ok') };
		}
		return {
			tone: 'danger',
			title: t('security.file.validate.fail'),
			body: t('security.file.validate.fail.body', {
				count: result.value.errors.length,
				first: result.value.errors[0]?.message ?? ''
			})
		};
	}

	const validationOutcome = $derived(outcomeOf(validation));

	async function runValidate(): Promise<void> {
		validating = true;
		validation = await validateConfig({});
		validating = false;
	}

	async function runRestart(): Promise<void> {
		const result = await restart(t('security.file.restart.reason'));
		restartOutcome =
			false === result.ok
				? { tone: 'danger', title: result.error.title, body: result.error.body }
				: { tone: 'ok', title: t('security.file.restart.sent') };
	}

	function downloadExport(): void {
		if (null !== exportText) {
			download(`callfs.${formatOf(tab)}`, new Blob([exportText], { type: 'text/plain' }));
		}
	}

	function retry(): void {
		void statusQuery.refetch();
		void configQuery.refetch();
		void exportQuery.refetch();
	}
</script>

<PageFrame title={t('security.file.title')} job={t('security.file.job')}>
	{#snippet primary()}
		<Button variant="primary" iconStart="check" loading={validating} onclick={() => void runValidate()}>
			{t('security.file.validate')}
		</Button>
	{/snippet}
	{#if null !== queryError}
		<ErrorState variant="page" error={queryError} onretry={retry} />
	{:else if undefined === statusQuery.data || undefined === configQuery.data}
		<Skeleton shape="rect" height="8rem" />
		<Skeleton shape="rect" height="16rem" />
	{:else if bothFailed && null !== statusError}
		<ErrorState variant="page" error={statusError} onretry={retry} />
	{:else}
		{#if null !== statusError || null !== configError}
			<InlineAlert tone="warn" title={t('security.partial.title')} body={t('security.partial.body')} />
		{/if}
		{#if null !== validationOutcome}
			<InlineAlert tone={validationOutcome.tone} title={validationOutcome.title} body={validationOutcome.body} />
		{/if}
		<SettingsCard title={t('settings-screens.file.facts')} description={t('settings-screens.file.facts.desc')}>
			{#if null !== statusError}
				<ErrorState variant="region" error={statusError} onretry={retry} />
			{:else if null !== statusView}
				<DescriptionList items={statusItems} columns={2} />
			{/if}
		</SettingsCard>
		<SettingsCard title={t('security.file.pending.title')} description={t('security.file.pending.body')}>
			{#snippet actions()}
				{#if restartAllowed}
					<Button variant="secondary" iconStart="refresh" onclick={() => (restartOpen = true)}>
						{t('security.file.restart.button')}
					</Button>
				{/if}
			{/snippet}
			{#if null !== configError}
				<ErrorState variant="region" error={configError} onretry={retry} />
			{:else if null !== configView}
				{#if 0 === pendingKeys.length}
					<Text tone="muted">{t('security.file.pending.none')}</Text>
				{:else}
					<ConfigDiff before={{}} after={pendingAfter} secretKeys={secretKeys} restartKeys={pendingKeys} />
					{#if false === restartAllowed}
						<div class="manual">
							<Text size="sm" tone="muted">{t('settings-screens.file.restart.manual')}</Text>
							<CodeView code={RESTART_EXAMPLE} language="shell" label={t('security.file.restart.title')} />
						</div>
					{/if}
				{/if}
			{/if}
			{#if null !== restartOutcome}
				<InlineAlert tone={restartOutcome.tone} title={restartOutcome.title} body={restartOutcome.body} />
			{/if}
		</SettingsCard>
		<SettingsCard title={t('security.file.export.title')} description={t('settings-screens.file.export.desc')}>
			{#snippet actions()}
				<Button variant="secondary" iconStart="download" onclick={downloadExport}>
					{t('security.file.download')}
				</Button>
			{/snippet}
			<Tabs bind:value={tab} items={tabItems} label={t('security.file.tab.label')} />
			{#if null !== exportError}
				<ErrorState variant="region" error={exportError} onretry={retry} />
			{:else if null === exportText}
				<Skeleton shape="rect" height="8rem" />
			{:else}
				<CodeView code={exportText} language={formatOf(tab)} label={`callfs.${formatOf(tab)}`} />
			{/if}
		</SettingsCard>
	{/if}
</PageFrame>

<AlertDialog
	bind:open={restartOpen}
	title={t('security.file.restart.dialog')}
	body={t('security.file.restart.body')}
	confirmLabel={t('security.file.restart.confirm')}
	tone="danger"
	onconfirm={() => void runRestart()}
/>

<style>
	.manual {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
</style>
