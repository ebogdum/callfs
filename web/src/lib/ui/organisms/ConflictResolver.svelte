<script lang="ts" module>
	export type ConflictPolicy = 'skip' | 'replace' | 'keep-both' | 'replace-if-changed';

	export interface ConflictStat {
		size: number;
		mtime: string;
	}

	export interface ConflictRow {
		path: string;
		existing: ConflictStat;
		incoming: ConflictStat;
	}

	export const TOKENS = ['--conflict-resolver-bg', '--conflict-resolver-pad', '--conflict-table-row-h'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatNumber } from '$lib/format/number';
	import { formatAbsolute } from '$lib/format/time';
	import Badge from '../atoms/Badge.svelte';
	import Select from '../atoms/Select.svelte';
	import RadioGroup from '../molecules/RadioGroup.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		conflicts: ConflictRow[];
		policy?: ConflictPolicy;
		overrides?: Record<string, ConflictPolicy>;
	}

	const POLICIES: readonly ConflictPolicy[] = ['skip', 'replace', 'keep-both', 'replace-if-changed'];
	const POLICY_LABELS: Record<ConflictPolicy, string> = {
		skip: 'transfers.conflict.policy.skip',
		replace: 'transfers.conflict.policy.replace',
		'keep-both': 'transfers.conflict.policy.keepBoth',
		'replace-if-changed': 'transfers.conflict.policy.replaceIfChanged'
	};
	const POLICY_DESCRIPTIONS: Record<ConflictPolicy, string> = {
		skip: 'transfers.conflict.policy.skip.description',
		replace: 'transfers.conflict.policy.replace.description',
		'keep-both': 'transfers.conflict.policy.keepBoth.description',
		'replace-if-changed': 'transfers.conflict.policy.replaceIfChanged.description'
	};
	const COUNT_LABELS: Record<ConflictPolicy, string> = {
		skip: 'transfers.conflict.count.skip',
		replace: 'transfers.conflict.count.replace',
		'keep-both': 'transfers.conflict.count.keepBoth',
		'replace-if-changed': 'transfers.conflict.count.replaceIfChanged'
	};

	let { conflicts, policy = $bindable('skip'), overrides = $bindable({}), class: className, ...rest }: Props = $props();

	const isConflictPolicy = (value: string): value is ConflictPolicy =>
		(POLICIES as readonly string[]).includes(value);

	const policyFor = (path: string): ConflictPolicy => {
		const overridden: ConflictPolicy | undefined = overrides[path];
		return undefined === overridden ? policy : overridden;
	};

	const choosePolicy = (next: string): void => {
		if (isConflictPolicy(next)) {
			policy = next;
		}
	};

	const setOverride = (path: string, next: string): void => {
		if (isConflictPolicy(next)) {
			overrides = { ...overrides, [path]: next };
		}
	};

	const policyOptions = POLICIES.map((value) => ({
		value,
		label: t(POLICY_LABELS[value]),
		description: t(POLICY_DESCRIPTIONS[value]),
		recommended: 'skip' === value
	}));

	const rowOptions = POLICIES.map((value) => ({ value, label: t(POLICY_LABELS[value]) }));

	const counts = $derived.by(() => {
		const result: Record<ConflictPolicy, number> = {
			skip: 0,
			replace: 0,
			'keep-both': 0,
			'replace-if-changed': 0
		};
		for (const conflict of conflicts) {
			result[policyFor(conflict.path)] += 1;
		}
		return result;
	});

	const summaryPolicies = $derived(POLICIES.filter((value) => 0 < counts[value]));
</script>

<section class={['conflict-resolver', className]} {...rest}>
	<RadioGroup
		bind:value={() => policy, choosePolicy}
		options={policyOptions}
		label={t('transfers.conflict.policy.label')}
	/>
	<Stack gap="2">
		<Cluster gap="2">
			{#each summaryPolicies as value (value)}
				<Badge tone="neutral">{t(COUNT_LABELS[value], { count: formatNumber(counts[value]) })}</Badge>
			{/each}
		</Cluster>
		<Scroll axis="x" label={t('transfers.conflict.table')}>
			<table class="table">
				<caption>{t('transfers.conflict.table')}</caption>
				<thead>
					<tr>
						<th scope="col">{t('transfers.conflict.column.file')}</th>
						<th scope="col">{t('transfers.conflict.column.existing')}</th>
						<th scope="col">{t('transfers.conflict.column.incoming')}</th>
						<th scope="col">{t('transfers.conflict.column.action')}</th>
					</tr>
				</thead>
				<tbody>
					{#each conflicts as conflict (conflict.path)}
						<tr>
							<th scope="row">{conflict.path}</th>
							<td>
								{t('transfers.conflict.stat', {
									size: formatBytes(conflict.existing.size).text,
									date: formatAbsolute(conflict.existing.mtime)
								})}
							</td>
							<td>
								{t('transfers.conflict.stat', {
									size: formatBytes(conflict.incoming.size).text,
									date: formatAbsolute(conflict.incoming.mtime)
								})}
							</td>
							<td>
								<Select
									bind:value={() => policyFor(conflict.path), (next: string) => setOverride(conflict.path, next)}
									options={rowOptions}
									size="sm"
									aria-label={t('transfers.conflict.row.action', { name: conflict.path })}
								/>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</Scroll>
	</Stack>
</section>

<style>
	.conflict-resolver {
		display: grid;
		gap: var(--space-4);
		padding: var(--conflict-resolver-pad, var(--card-pad));
		background: var(--conflict-resolver-bg, var(--surface));
		color: var(--text);
		font-family: var(--font-sans);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.table {
		inline-size: 100%;
		border-collapse: collapse;
	}

	caption {
		padding-block-end: var(--space-2);
		color: var(--text-muted);
		text-align: start;
	}

	th,
	td {
		padding-inline: var(--cell-px);
		padding-block: var(--space-2);
		text-align: start;
		vertical-align: middle;
		overflow-wrap: anywhere;
		font-size: var(--table-text);
		line-height: var(--text-body-line);
	}

	thead th {
		color: var(--text-muted);
		font-weight: var(--text-body-weight);
	}

	tbody th {
		font-weight: var(--text-body-weight);
	}

	tbody tr {
		min-block-size: var(--conflict-table-row-h, var(--row-h));
	}

	tbody tr + tr > * {
		border-block-start: var(--hairline) solid var(--border-subtle);
	}
</style>
