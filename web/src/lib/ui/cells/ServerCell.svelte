<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Badge from '../atoms/Badge.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Dot from '../quarks/Dot.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';
	import type { Tone } from '../types';

	export const TOKENS = ['--server-cell-fg', '--server-cell-font-size'] as const;

	type Reachability = 'up' | 'down' | 'unknown';

	interface ReachabilityView {
		tone: Tone;
		word: string;
	}

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		id: string;
		self?: boolean;
		reachable?: boolean | null;
	}

	const REACHABILITY: Record<Reachability, ReachabilityView> = {
		up: { tone: 'ok', word: 'status.server.up' },
		down: { tone: 'danger', word: 'status.server.down' },
		unknown: { tone: 'neutral', word: 'status.server.unknown' },
	};

	let { id, self = false, reachable = null, class: className, ...rest }: Props = $props();

	const reachability = $derived<Reachability>(
		true === reachable ? 'up' : false === reachable ? 'down' : 'unknown'
	);
	const view = $derived(REACHABILITY[reachability]);
</script>

<span class={['server-cell', className]} data-state={reachability} {...rest}>
	<Cluster as="span" gap="2" align="center" wrap={false}>
		<Dot tone={view.tone} />
		<VisuallyHidden>{t(view.word)}</VisuallyHidden>
		<MiddleTruncate text={id} mono />
		{#if self}
			<Badge tone="accent">{t('status.server.self')}</Badge>
		{/if}
	</Cluster>
</span>

<style>
	.server-cell {
		display: inline-flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
		color: var(--server-cell-fg, var(--text));
		font-size: var(--server-cell-font-size, var(--table-text, var(--text-sm)));
		white-space: nowrap;
	}
</style>
