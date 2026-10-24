<script lang="ts" module>
	import type { AuthFailure, UserInfo } from '$lib/api/types';

	const DAY_MS = 86_400_000;
	const FAILURE_PAGE = 25;
	const QUERY_STALE_MS = 5000;

	const REASON_KEY: Record<AuthFailure['reason'], string> = {
		missing_key: 'logs.signins.reason.missing_key',
		wrong_key: 'logs.signins.reason.wrong_key',
		expired_session: 'logs.signins.reason.expired_session',
		csrf: 'logs.signins.reason.csrf'
	};

	function failureKey(failure: AuthFailure): string {
		return [failure.at, failure.client, failure.path, failure.reason].join('|');
	}

	function userKey(user: UserInfo): string {
		return user.user_id;
	}
</script>

<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import { authFailures, users } from '$lib/api/admin/users';
	import { toUiError } from '$lib/api/errors';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import type { ColumnDef, QueryStatus, UiError } from '$lib/ui/types';
	import Heading from '$lib/ui/atoms/Heading.svelte';
	import NumberCell from '$lib/ui/cells/NumberCell.svelte';
	import PathCell from '$lib/ui/cells/PathCell.svelte';
	import TextCell from '$lib/ui/cells/TextCell.svelte';
	import TimeCell from '$lib/ui/cells/TimeCell.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import EmptyState from '$lib/ui/molecules/EmptyState.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import KpiStrip from '$lib/ui/organisms/KpiStrip.svelte';
	import DataTable from '$lib/ui/organisms/table/DataTable.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	type Tile = ComponentProps<typeof KpiStrip>['tiles'][number];

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	let failureLimit = $state(FAILURE_PAGE);

	const failuresQuery = createQuery({
		key: ['auth-failures', '24h'],
		fetch: () => authFailures(new Date(Date.now() - DAY_MS).toISOString()),
		staleMs: QUERY_STALE_MS
	});

	const usersQuery = createQuery({
		key: ['users'],
		fetch: () => users(),
		staleMs: QUERY_STALE_MS
	});

	const failuresResult = $derived(failuresQuery.data);
	const failures = $derived(true === failuresResult?.ok ? failuresResult.value.items : []);
	const totalFailed = $derived(true === failuresResult?.ok ? failuresResult.value.total_24h : null);
	const failuresError = $derived.by((): UiError | null =>
		false === failuresResult?.ok ? toUiError(failuresResult.error, { action: 'logs.signins.read' }) : null
	);
	const failuresStatus = $derived.by((): QueryStatus => {
		if (undefined === failuresResult) {
			return 'loading';
		}
		if (0 < failures.length) {
			return 'success';
		}
		return false === failuresResult.ok ? 'error' : 'empty';
	});

	const usersResult = $derived(usersQuery.data);
	const userRows = $derived(true === usersResult?.ok ? usersResult.value.items : []);
	const usersError = $derived.by((): UiError | null =>
		false === usersResult?.ok ? toUiError(usersResult.error, { action: 'logs.signins.keys.read' }) : null
	);
	const usersStatus = $derived.by((): QueryStatus => {
		if (undefined === usersResult) {
			return 'loading';
		}
		if (0 < userRows.length) {
			return 'success';
		}
		return false === usersResult.ok ? 'error' : 'empty';
	});

	const requestsLastDay = $derived(userRows.reduce((sum, user) => sum + user.requests_24h, 0));
	const attempts = $derived(
		null !== totalFailed && true === usersResult?.ok ? totalFailed + requestsLastDay : null
	);

	const visibleFailures = $derived(failures.slice(0, failureLimit));
	const failuresLoaded = $derived(true === failuresResult?.ok);
	const usersLoaded = $derived(true === usersResult?.ok);
	const clientCount = $derived(new Set(failures.map((failure) => failure.client)).size);
	const wrongKeyCount = $derived(failures.filter((failure) => 'wrong_key' === failure.reason).length);
	const activeUsers = $derived(userRows.filter((user) => 0 < user.requests_24h).length);

	const failuresLoading = $derived(undefined === failuresResult && null === failuresQuery.error);
	const usersLoading = $derived(undefined === usersResult && null === usersQuery.error);
	const failuresReason = $derived((failuresError ?? failuresQuery.error)?.title);
	const usersReason = $derived((usersError ?? usersQuery.error)?.title);

	const tiles = $derived.by((): Tile[] => [
		{
			label: t('logs.signins.kpi.failed'),
			value: totalFailed,
			unit: 'count',
			base: null === totalFailed || null === attempts ? undefined : { num: totalFailed, den: attempts },
			help: null === totalFailed ? failuresReason : undefined,
			loading: failuresLoading
		},
		{
			label: t('activity.signins.kpi.wrongKey'),
			value: failuresLoaded ? wrongKeyCount : null,
			unit: 'count',
			help: failuresReason,
			loading: failuresLoading
		},
		{
			label: t('activity.signins.kpi.clients'),
			value: failuresLoaded ? clientCount : null,
			unit: 'count',
			help: failuresReason,
			loading: failuresLoading
		},
		{
			label: t('activity.signins.kpi.activeUsers'),
			value: usersLoaded ? activeUsers : null,
			unit: 'count',
			base: usersLoaded ? { num: activeUsers, den: userRows.length } : undefined,
			help: usersReason,
			loading: usersLoading
		}
	]);
	function retryFailures(): void {
		void failuresQuery.refetch();
	}

	function retryUsers(): void {
		void usersQuery.refetch();
	}

	const failureColumns: ColumnDef<AuthFailure>[] = [
		{ id: 'time', header: t('logs.signins.col.time'), cell: failureTimeCell, priority: 1 },
		{ id: 'client', header: t('logs.signins.col.client'), cell: failureClientCell, priority: 1 },
		{ id: 'reason', header: t('logs.signins.col.reason'), cell: failureReasonCell, priority: 1 },
		{ id: 'path', header: t('logs.signins.col.path'), cell: failurePathCell, priority: 2 }
	];

	const userColumns: ColumnDef<UserInfo>[] = [
		{ id: 'user', header: t('logs.signins.keys.col.user'), cell: userNameCell, priority: 1 },
		{ id: 'key', header: t('logs.signins.keys.col.key'), cell: userKeyCell, priority: 2 },
		{ id: 'last', header: t('logs.signins.keys.col.last'), cell: userLastCell, priority: 1 },
		{ id: 'failed', header: t('logs.signins.keys.col.failed'), cell: userFailedCell, align: 'end', priority: 2 }
	];
</script>

{#snippet failureTimeCell(failure: AuthFailure)}
	<TimeCell value={failure.at} />
{/snippet}

{#snippet failureClientCell(failure: AuthFailure)}
	<TextCell value={failure.client} mono />
{/snippet}

{#snippet failureReasonCell(failure: AuthFailure)}
	<TextCell value={t(REASON_KEY[failure.reason])} />
{/snippet}

{#snippet failurePathCell(failure: AuthFailure)}
	<PathCell path={failure.path} />
{/snippet}

{#snippet userNameCell(user: UserInfo)}
	<TextCell value={user.user_id} />
{/snippet}

{#snippet userKeyCell(user: UserInfo)}
	<TextCell value={user.key_hint} mono />
{/snippet}

{#snippet userLastCell(user: UserInfo)}
	<TimeCell value={user.last_auth_at} />
{/snippet}

{#snippet userFailedCell(user: UserInfo)}
	<NumberCell value={user.auth_failures_24h} />
{/snippet}

<PageFrame title={t('logs.signins.title')} job={t('logs.signins.job')} class={className}>
	<Stack gap="8">
		{#if null !== failuresError}
			<ErrorState error={failuresError} onretry={retryFailures} />
		{/if}

		<KpiStrip {tiles} />

		<Stack gap="4">
			<Heading level={2} size="title-sm">{t('activity.signins.failures.title')}</Heading>
			<DataTable
				rows={visibleFailures}
				columns={failureColumns}
				rowKey={failureKey}
				label={t('logs.signins.table.label')}
				status={failuresStatus}
				total={failures.length}
				hasMore={failureLimit < failures.length}
				onloadmore={() => (failureLimit += FAILURE_PAGE)}
			>
			{#snippet empty()}
				<EmptyState
					variant="no-results"
					icon="shield"
					title={t('logs.signins.empty.title')}
					body={t('logs.signins.empty.body')}
				/>
			{/snippet}
		</DataTable>
		</Stack>

		<Stack gap="4">
			<Heading level={2} size="title-sm">{t('logs.signins.keys.title')}</Heading>

			{#if null !== usersError}
				<ErrorState error={usersError} onretry={retryUsers} />
			{/if}

			<DataTable
				rows={userRows}
				columns={userColumns}
				rowKey={userKey}
				label={t('logs.signins.keys.label')}
				status={usersStatus}
			>
				{#snippet empty()}
					<EmptyState
						variant="no-results"
						icon="users"
						title={t('logs.signins.keys.empty.title')}
						body={t('logs.signins.keys.empty.body')}
					/>
				{/snippet}
			</DataTable>
		</Stack>
	</Stack>
</PageFrame>
