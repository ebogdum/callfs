<script lang="ts" module>
	export type FeedLevel = 'info' | 'warn' | 'error';
	export type FeedEntityKind = 'file' | 'folder' | 'link' | 'member' | 'server' | 'job' | 'setting';

	export interface FeedEvent {
		id: string;
		at: string;
		type: string;
		level: FeedLevel;
		actor: string | null;
		instance_id: string;
		message: string;
		entities: { kind: FeedEntityKind; id: string }[];
		fields: Record<string, unknown>;
	}

	export const EVENT_GROUPS = [
		{ id: 'files', prefixes: ['file.'] },
		{ id: 'links', prefixes: ['link.'] },
		{ id: 'cluster', prefixes: ['cluster.', 'server.'] },
		{ id: 'config', prefixes: ['config.'] },
		{ id: 'jobs', prefixes: ['job.'] },
		{ id: 'health', prefixes: ['health.', 'finding.', 'log.'] },
		{ id: 'security', prefixes: ['security.'] },
	] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Tone } from '../types';
	import type { Snippet } from 'svelte';
	import { base } from '$app/paths';
	import { t } from '$lib/copy/t';
	import Heading from '../atoms/Heading.svelte';
	import Link from '../atoms/Link.svelte';
	import RelTime from '../atoms/RelTime.svelte';
	import Text from '../atoms/Text.svelte';
	import ToggleButton from '../atoms/ToggleButton.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Dot from '../quarks/Dot.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		events: FeedEvent[];
		types?: string[];
		live: boolean;
		empty?: Snippet;
		class?: string;
	}

	let { events, types = $bindable([]), live, empty, class: className, ...rest }: Props = $props();

	const MINUTE_MS = 60_000;
	const LEVEL_TONE: Record<FeedLevel, Tone> = { info: 'info', warn: 'warn', error: 'danger' };
	const ENTITY_HREF: Record<FeedEntityKind, (id: string) => string> = {
		file: (id) => `/files/${encodeURI(id.replace(/^\/+/, ''))}`,
		folder: (id) => `/files/${encodeURI(id.replace(/^\/+/, ''))}`,
		link: (id) => `/sharing/${encodeURIComponent(id)}`,
		member: (id) => `/cluster/nodes/${encodeURIComponent(id)}`,
		server: () => '/cluster/servers',
		job: (id) => `/activity/jobs/${encodeURIComponent(id)}`,
		setting: () => '/settings',
	};

	interface MinuteGroup {
		key: number;
		at: string;
		events: FeedEvent[];
	}

	const sorted = $derived([...events].sort((a, b) => Date.parse(b.at) - Date.parse(a.at)));
	const selectedPrefixes = $derived(
		EVENT_GROUPS.filter((group) => types.includes(group.id)).flatMap((group) => group.prefixes)
	);
	const visible = $derived(
		sorted.filter((event) => 0 === selectedPrefixes.length || selectedPrefixes.some((prefix) => event.type.startsWith(prefix)))
	);
	const groups = $derived.by(() => {
		const result: MinuteGroup[] = [];
		for (const event of visible) {
			const key = Math.floor(Date.parse(event.at) / MINUTE_MS);
			const last = result.at(-1);
			if (undefined !== last && last.key === key) {
				last.events.push(event);
			} else {
				result.push({ key, at: event.at, events: [event] });
			}
		}
		return result;
	});

	function setGroup(id: string, pressed: boolean): void {
		types = pressed ? [...types.filter((item) => item !== id), id] : types.filter((item) => item !== id);
	}
</script>

<div class={['event-feed', className]} {...rest}>
	<Cluster gap="2" role="group" aria-label={t('u106logs.events.types.label')}>
		{#each EVENT_GROUPS as group (group.id)}
			<ToggleButton
				label={t(`u106logs.group.${group.id}`)}
				bind:pressed={() => types.includes(group.id), (pressed) => setGroup(group.id, pressed)}
			/>
		{/each}
	</Cluster>

	<Cluster gap="2">
		<Dot tone={live ? 'ok' : 'neutral'} pulse={live} />
		<Text tone="muted" size="sm">{live ? t('u106logs.events.live') : t('u106logs.events.paused')}</Text>
	</Cluster>

	{#if 0 === groups.length}
		{#if undefined !== empty}
			{@render empty()}
		{:else}
			<Text tone="muted">{t('u106logs.events.empty')}</Text>
		{/if}
	{/if}

	{#each groups as group, index (index)}
		<section class="minute">
			<Heading level={3}><RelTime at={group.at} /></Heading>
			<ol class="events">
				{#each group.events as event (event.id)}
					<li class="event">
						<StatusGlyph tone={LEVEL_TONE[event.level]} />
						<div class="body">
							<Text>{event.message}</Text>
							<Cluster gap="2">
								<Text tone="muted" size="caption">{t(`u106logs.level.${event.level}`)}</Text>
								{#each event.entities as entity, entityIndex (entityIndex)}
									<Link href={`${base}${ENTITY_HREF[entity.kind](entity.id)}`}>{entity.id}</Link>
								{/each}
							</Cluster>
						</div>
						<RelTime at={event.at} />
					</li>
				{/each}
			</ol>
		</section>
	{/each}
</div>

<style>
	.event-feed {
		display: grid;
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.minute {
		display: grid;
		gap: var(--space-2);
	}

	.events {
		display: grid;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.event {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: start;
		gap: var(--space-2);
		padding-block: var(--space-2);
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.body {
		display: grid;
		gap: var(--space-1);
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}
</style>
