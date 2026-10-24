<script lang="ts" module>
	import type { LogEntry } from '$lib/api/types';
	import type { LogLevel } from '$lib/api/admin/telemetry';

	const LOG_STREAM_PATH = '/v1/admin/logs/stream';
	const LEVEL_ORDER: readonly LogLevel[] = ['debug', 'info', 'warn', 'error'];
	const CONFIG_STALE_MS = 60_000;
	const STREAM_LIMIT = 500;

	function lowestLevel(levels: readonly string[]): LogLevel {
		return LEVEL_ORDER.find((level) => levels.includes(level)) ?? 'error';
	}

	function matchesSearch(entry: LogEntry, search: string): boolean {
		return '' === search || entry.message.toLowerCase().includes(search.toLowerCase());
	}

	function uniqueEntries(entries: LogEntry[]): LogEntry[] {
		const seen = new Set<string>();
		return entries.filter((entry) => {
			if (seen.has(entry.id)) {
				return false;
			}
			seen.add(entry.id);
			return true;
		});
	}

	function parseLogEntry(data: unknown): LogEntry | null {
		if ('string' !== typeof data) {
			return null;
		}
		let parsed: unknown;
		try {
			parsed = JSON.parse(data);
		} catch {
			return null;
		}
		if ('object' !== typeof parsed || null === parsed) {
			return null;
		}
		const candidate = parsed as Record<string, unknown>;
		const id = candidate.id;
		if (('string' !== typeof id && 'number' !== typeof id) || 'string' !== typeof candidate.level || 'string' !== typeof candidate.message) {
			return null;
		}
		return { ...(candidate as unknown as LogEntry), id: String(id) };
	}
</script>

<script lang="ts">
	import { base } from '$app/paths';
	import { getConfig } from '$lib/api/admin/config';
	import { logs } from '$lib/api/admin/telemetry';
	import { toUiError } from '$lib/api/errors';
	import { nodePath } from '$lib/api/http';
	import { t } from '$lib/copy/t';
	import { createQuery } from '$lib/data/query.svelte';
	import type { QueryStatus, UiError } from '$lib/ui/types';
	import Link from '$lib/ui/atoms/Link.svelte';
	import Text from '$lib/ui/atoms/Text.svelte';
	import PageFrame from '$lib/ui/environments/PageFrame.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import LogViewer from '$lib/ui/organisms/LogViewer.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();

	let live = $state(true);
	let level = $state<string[]>(['info', 'warn', 'error']);
	let query = $state('');
	let streamed = $state<LogEntry[]>([]);
	let streamFailed = $state(false);
	let older = $state<LogEntry[]>([]);
	let olderCursor = $state<string | null | undefined>(undefined);
	let olderFailure = $state<UiError | null>(null);

	const minLevel = $derived(lowestLevel(level));
	const serverQuery = $derived(query.trim());
	const resetKey = $derived(JSON.stringify([minLevel, serverQuery]));

	const logsQuery = createQuery(() => ({
		key: ['logs', minLevel, serverQuery],
		fetch: () => logs({ level: minLevel, q: '' === serverQuery ? undefined : serverQuery }),
		staleMs: 5000
	}));

	const configQuery = createQuery({
		key: ['config'],
		fetch: () => getConfig(),
		staleMs: CONFIG_STALE_MS
	});

	const pageResult = $derived(logsQuery.data);
	const pageEntries = $derived(true === pageResult?.ok ? pageResult.value.items : []);
	const pageCursor = $derived(true === pageResult?.ok ? pageResult.value.next_cursor : null);
	const pageUiError = $derived(
		false === pageResult?.ok ? toUiError(pageResult.error, { action: 'logs.read' }) : null
	);
	const cursor = $derived(undefined === olderCursor ? pageCursor : olderCursor);

	const streamVisible = $derived(streamed.filter((entry) => matchesSearch(entry, serverQuery)));
	const entries = $derived(
		uniqueEntries([...streamVisible, ...pageEntries, ...older]).filter((entry) => level.includes(entry.level))
	);

	const status = $derived.by((): QueryStatus => {
		if (0 < entries.length) {
			return 'success';
		}
		if (undefined === pageResult) {
			return 'loading';
		}
		return false === pageResult.ok ? 'error' : 'empty';
	});

	const configuredLevel = $derived.by((): string | null => {
		const result = configQuery.data;
		if (undefined === result || false === result.ok) {
			return null;
		}
		const setting = result.value.keys.find((key) => 'log.level' === key.path);
		return undefined !== setting && 'string' === typeof setting.value ? setting.value : null;
	});
	const levelSentence = $derived(
		null === configuredLevel ? t('logs.level.unknown') : t('logs.level.current', { level: configuredLevel })
	);

	$effect(() => {
		if (false === live) {
			return;
		}
		const source = new EventSource(nodePath(LOG_STREAM_PATH));
		const onLog = (event: Event): void => {
			const entry = parseLogEntry(event instanceof MessageEvent ? event.data : null);
			if (null === entry) {
				streamFailed = true;
				return;
			}
			streamed = [entry, ...streamed].slice(0, STREAM_LIMIT);
		};
		source.addEventListener('log', onLog);
		source.onopen = (): void => {
			streamFailed = false;
		};
		source.onerror = (): void => {
			streamFailed = true;
		};
		return (): void => {
			source.removeEventListener('log', onLog);
			source.close();
		};
	});

	$effect(() => {
		void resetKey;
		older = [];
		olderCursor = undefined;
		olderFailure = null;
	});

	async function loadOlder(): Promise<void> {
		if (null === cursor) {
			return;
		}
		const result = await logs({ level: minLevel, q: '' === serverQuery ? undefined : serverQuery, cursor });
		if (false === result.ok) {
			olderFailure = toUiError(result.error, { action: 'logs.read' });
			return;
		}
		olderFailure = null;
		older = [...older, ...result.value.items];
		olderCursor = result.value.next_cursor;
	}

	function retry(): void {
		void logsQuery.refetch();
	}
</script>

<PageFrame title={t('logs.title')} job={t('logs.job')} class={className}>
	{#snippet scope()}
		<Cluster gap="2" align="center">
			<Text tone="muted" size="sm">{levelSentence}</Text>
			<Link href={`${base}/settings/logging`}>{t('logs.level.change')}</Link>
		</Cluster>
	{/snippet}

	<Stack gap="4">
		{#if null !== pageUiError}
			<ErrorState error={pageUiError} onretry={retry} />
		{/if}

		{#if streamFailed}
			<InlineAlert tone="warn" title={t('logs.stream.error.title')} body={t('logs.stream.error.body')} />
		{/if}

		{#if null !== olderFailure}
			<InlineAlert tone="danger" title={olderFailure.title} body={olderFailure.body} />
		{/if}

		<LogViewer
			{entries}
			{status}
			bind:live
			bind:level
			bind:query
			onloadolder={() => void loadOlder()}
		/>
	</Stack>
</PageFrame>
