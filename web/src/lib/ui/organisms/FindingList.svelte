<script lang="ts" module>
	import type { MenuItem, StatusKey } from '../types';

	export interface Finding {
		id: string;
		severity: 'critical' | 'warning' | 'info';
		area: 'security' | 'storage' | 'cluster' | 'config' | 'runtime';
		title: string;
		body: string;
		affected: { kind: string; id: string; href: string }[];
		since: string;
		fix: { label: string; route: string; setting_key: string | null; command: string | null };
		muted: { until: string; by: string; reason: string } | null;
		ignored?: { at: string; by: string; reason: string } | null;
	}

	export const IGNORE_REASON_MAX = 200;

	export const TOKENS = ['--finding-list-gap', '--finding-item-pad', '--finding-item-bg'] as const;

	const severityStatus: Record<Finding['severity'], StatusKey> = {
		critical: 'finding.critical',
		warning: 'finding.warning',
		info: 'finding.info'
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import Heading from '../atoms/Heading.svelte';
	import Link from '../atoms/Link.svelte';
	import RelTime from '../atoms/RelTime.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Text from '../atoms/Text.svelte';
	import Textarea from '../atoms/Textarea.svelte';
	import EmptyState from '../molecules/EmptyState.svelte';
	import FormField from '../molecules/FormField.svelte';
	import Menu from '../molecules/Menu.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		findings: Finding[];
		onfix: (finding: Finding) => void;
		onmute: (finding: Finding, durationS: number) => void;
		onunmute: (finding: Finding) => void;
		ignored?: Finding[];
		onignore?: (finding: Finding, reason: string) => void;
		onunignore?: (finding: Finding) => void;
		class?: string;
	}

	let {
		findings,
		onfix,
		onmute,
		onunmute,
		ignored = [],
		onignore,
		onunignore,
		class: className,
		...rest
	}: Props = $props();

	let mutedOpen = $state(false);
	let ignoredOpen = $state(false);
	let ignoringId = $state<string | null>(null);
	let ignoreReason = $state('');

	const muteOptions = [
		{ id: 'hour', label: 'findings.mute.hour', durationS: 3600 },
		{ id: 'day', label: 'findings.mute.day', durationS: 86400 },
		{ id: 'week', label: 'findings.mute.week', durationS: 604800 }
	];

	const isIgnored = (finding: Finding): boolean => undefined !== finding.ignored && null !== finding.ignored;
	const open = $derived(findings.filter((finding) => false === isIgnored(finding)));
	const active = $derived(open.filter((finding) => null === finding.muted));
	const muted = $derived(open.filter((finding) => null !== finding.muted));
	const ignoredList = $derived([
		...ignored,
		...findings.filter((finding) => isIgnored(finding) && false === ignored.some((entry) => entry.id === finding.id))
	]);

	function startIgnore(finding: Finding): void {
		ignoringId = finding.id;
		ignoreReason = '';
	}

	function cancelIgnore(): void {
		ignoringId = null;
		ignoreReason = '';
	}

	function confirmIgnore(finding: Finding): void {
		const reason = ignoreReason.trim();
		cancelIgnore();
		onignore?.(finding, reason);
	}

	const muteItems = (finding: Finding): MenuItem[] =>
		muteOptions.map((option) => ({
			id: `${finding.id}-${option.id}`,
			label: t(option.label),
			onselect: () => onmute(finding, option.durationS)
		}));
</script>

<section {...rest} class={['finding-list', className]} aria-label={t('findings.list.label')}>
	<Stack gap="4">
		{#if 0 === open.length && 0 === ignoredList.length}
			<EmptyState
				variant="cleared"
				icon="shield"
				title={t('findings.list.empty_title')}
				body={t('findings.list.empty_body')}
			/>
		{:else if 0 < open.length}
			{#if 0 < active.length}
				<ol class="items">
					{#each active as finding (finding.id)}
						<li class="item">
							<Stack gap="2">
								<Cluster gap="2" align="center">
									<StatusPill status={severityStatus[finding.severity]} />
									<Heading level={3}>{finding.title}</Heading>
								</Cluster>
								<Text tone="muted">{finding.body}</Text>
								<Cluster gap="2" align="center">
									<Text size="sm" tone="muted">{t('findings.list.since')} <RelTime at={finding.since} /></Text>
									{#if 0 < finding.affected.length}
										<Text size="sm" tone="muted">{t('findings.list.affected')}</Text>
										{#each finding.affected as entity}
											<Link href={entity.href}>{entity.id}</Link>
										{/each}
									{/if}
								</Cluster>
								<Cluster gap="2" align="center">
									<Button
										variant="primary"
										size="sm"
										aria-label={`${t('findings.list.fix')}: ${finding.title}`}
										onclick={() => onfix(finding)}>{t('findings.list.fix')}</Button
									>
									{#if undefined !== onignore}
										<Button
											variant="secondary"
											size="sm"
											aria-label={`${t('findings.list.ignore')}: ${finding.title}`}
											aria-expanded={finding.id === ignoringId}
											aria-controls={`finding-ignore-${finding.id}`}
											onclick={() => (finding.id === ignoringId ? cancelIgnore() : startIgnore(finding))}
											>{t('findings.list.ignore')}</Button
										>
									{/if}
									<Menu items={muteItems(finding)} label={t('findings.list.mute')}>
										{#snippet trigger(props)}
											<Button variant="ghost" size="sm" iconEnd="chevron-down" {...props}>
												{t('findings.list.mute')}
											</Button>
										{/snippet}
									</Menu>
								</Cluster>
								{#if undefined !== onignore && finding.id === ignoringId}
									<form
										class="ignore-form"
										id={`finding-ignore-${finding.id}`}
										onsubmit={(event) => {
											event.preventDefault();
											confirmIgnore(finding);
										}}
									>
										<Stack gap="3">
											<FormField
												label={t('findings.list.ignore_reason')}
												for={`finding-ignore-reason-${finding.id}`}
												help={t('findings.list.ignore_help')}
												example={t('findings.list.ignore_example')}
												optional
											>
												{#snippet control(field)}
													<Textarea
														id={field.id}
														aria-describedby={field.describedBy}
														rows={2}
														maxLength={IGNORE_REASON_MAX}
														bind:value={ignoreReason}
														onsubmit={() => confirmIgnore(finding)}
													/>
												{/snippet}
											</FormField>
											<Cluster gap="2" align="center">
												<Button variant="primary" size="sm" type="submit">{t('findings.list.ignore_confirm')}</Button>
												<Button variant="ghost" size="sm" onclick={cancelIgnore}>{t('findings.list.ignore_cancel')}</Button>
											</Cluster>
										</Stack>
									</form>
								{/if}
							</Stack>
						</li>
					{/each}
				</ol>
			{/if}
			{#if 0 < muted.length}
				<Stack gap="2">
					<div>
						<Button
							variant="ghost"
							size="sm"
							iconEnd={mutedOpen ? 'chevron-up' : 'chevron-down'}
							aria-expanded={mutedOpen}
							aria-controls="finding-muted-list"
							onclick={() => (mutedOpen = !mutedOpen)}
						>
							{mutedOpen ? t('findings.list.muted_hide') : t('findings.list.muted_show', { count: muted.length })}
						</Button>
					</div>
					{#if mutedOpen}
						<ol id="finding-muted-list" class="items">
							{#each muted as finding (finding.id)}
								<li class="item" data-muted="true">
									<Stack gap="2">
										<Cluster gap="2" align="center">
											<StatusPill status="finding.muted" />
											<Heading level={3}>{finding.title}</Heading>
										</Cluster>
										{#if finding.muted}
											<Text size="sm" tone="muted"
												>{t('findings.list.muted_by', { by: finding.muted.by })} <RelTime at={finding.muted.until} /></Text
											>
										{/if}
										<Cluster gap="2" align="center">
											<Button
												variant="secondary"
												size="sm"
												aria-label={`${t('findings.list.unmute')}: ${finding.title}`}
												onclick={() => onunmute(finding)}>{t('findings.list.unmute')}</Button
											>
										</Cluster>
									</Stack>
								</li>
							{/each}
						</ol>
					{/if}
				</Stack>
			{/if}
		{/if}
		{#if 0 < ignoredList.length}
			<Stack gap="2">
				<div>
					<Button
						variant="ghost"
						size="sm"
						iconEnd={ignoredOpen ? 'chevron-up' : 'chevron-down'}
						aria-expanded={ignoredOpen}
						aria-controls="finding-ignored-list"
						onclick={() => (ignoredOpen = !ignoredOpen)}
					>
						{ignoredOpen
							? t('findings.list.ignored_hide')
							: t('findings.list.ignored_show', { count: ignoredList.length })}
					</Button>
				</div>
				{#if ignoredOpen}
					<ol id="finding-ignored-list" class="items" aria-label={t('findings.list.ignored_label')}>
						{#each ignoredList as finding (finding.id)}
							<li class="item" data-ignored="true">
								<Stack gap="2">
									<Cluster gap="2" align="center">
										<StatusPill status="finding.ignored" />
										<Heading level={3}>{finding.title}</Heading>
									</Cluster>
									{#if finding.ignored}
										<Text size="sm" tone="muted"
											>{t('findings.list.ignored_by', { by: finding.ignored.by })} · <RelTime
												at={finding.ignored.at}
											/></Text
										>
										{#if '' !== finding.ignored.reason}
											<Text size="sm">{t('findings.list.ignored_reason', { reason: finding.ignored.reason })}</Text>
										{/if}
									{/if}
									{#if undefined !== onunignore}
										<Cluster gap="2" align="center">
											<Button
												variant="secondary"
												size="sm"
												aria-label={`${t('findings.list.unignore')}: ${finding.title}`}
												onclick={() => onunignore(finding)}>{t('findings.list.unignore')}</Button
											>
										</Cluster>
									{/if}
								</Stack>
							</li>
						{/each}
					</ol>
				{/if}
			</Stack>
		{/if}
	</Stack>
</section>

<style>
	.finding-list {
		display: grid;
		min-inline-size: 0;
	}

	.items {
		display: grid;
		gap: var(--finding-list-gap, var(--space-4));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.item[data-ignored='true'] {
		--finding-item-bg: var(--surface-sunken, var(--surface));
	}

	.ignore-form {
		min-inline-size: 0;
		max-inline-size: calc(var(--space-4) * 40);
		padding-block-start: var(--space-3);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.item {
		min-inline-size: 0;
		padding: var(--finding-item-pad, var(--card-pad));
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--finding-item-bg, var(--surface));
	}
</style>
