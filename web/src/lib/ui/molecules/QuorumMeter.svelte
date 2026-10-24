<script lang="ts" module>
	export const TOKENS = ['--quorum-meter-up', '--quorum-meter-down'] as const;

	export type QuorumState = 'can_lose' | 'at_risk' | 'lost';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		voters: number;
		healthy: number;
		class?: string;
	}

	let { voters, healthy, class: className, ...rest }: Props = $props();

	const glyphTones = { can_lose: 'ok', at_risk: 'warn', lost: 'danger' } as const;

	const up = $derived(Math.min(voters, Math.max(0, healthy)));
	const margin = $derived(Math.floor((voters - 1) / 2) - (voters - up));
	const state: QuorumState = $derived(0 > margin ? 'lost' : 0 === margin ? 'at_risk' : 'can_lose');
	const members = $derived(Array.from({ length: voters }, (_, index) => index < up));

	function sentenceFor(current: QuorumState): string {
		const params = { healthy: up, voters, count: margin };
		if ('lost' === current) {
			return t('protection.quorum.lost', params);
		}
		if ('at_risk' === current) {
			return t('protection.quorum.at_risk', params);
		}
		return t('protection.quorum.can_lose', params);
	}

	const sentence = $derived(sentenceFor(state));
</script>

<div class={['quorum-meter', className]} data-state={state} {...rest}>
	<Stack gap="2">
		<Cluster as="ul" gap="1" class="quorum-members" aria-hidden="true">
			{#each members as isUp, index (index)}
				<li class="member" data-up={isUp}>
					<StatusGlyph tone={isUp ? 'ok' : 'danger'} />
				</li>
			{/each}
		</Cluster>
		<Cluster gap="2" align="start">
			<StatusGlyph tone={glyphTones[state]} />
			<p class="sentence">{sentence}</p>
		</Cluster>
	</Stack>
</div>

<style>
	.quorum-meter {
		min-inline-size: 0;
	}

	:global(.quorum-members),
	:global(.quorum-members) li {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sentence {
		margin: 0;
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
		text-wrap: pretty;
	}

	.member[data-up='true'] {
		color: var(--quorum-meter-up, var(--ok));
	}

	.member[data-up='false'] {
		color: var(--quorum-meter-down, var(--danger));
	}
</style>
