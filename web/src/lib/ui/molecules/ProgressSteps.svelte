<script lang="ts" module>
	export interface ProgressStep {
		id: string;
		label: string;
		state: StatusKey;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { STATUS, type StatusKey } from '$lib/status/catalog';
	import Cluster from '../quarks/Cluster.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLOListElement>, 'class'> {
		steps: ProgressStep[];
		class?: string;
	}

	let { steps, class: className, ...rest }: Props = $props();
</script>

<ol class={['progress-steps', className]} {...rest}>
	{#each steps as step (step.id)}
		{@const entry = STATUS[step.state]}
		<li class="step" data-state={step.state} data-tone={entry.tone} aria-current={'progress' === entry.tone ? 'step' : undefined}>
			<Cluster gap="2" align="center">
				<StatusGlyph tone={entry.tone} />
				<span class="label">{step.label}</span>
				<span class="word">{t(entry.word)}</span>
			</Cluster>
		</li>
	{/each}
</ol>

<style>
	.progress-steps {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.step {
		min-inline-size: 0;
		color: var(--text-muted);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
	}

	.step[data-tone='progress'] {
		color: var(--text);
	}

	.label {
		color: var(--text);
		font-weight: var(--text-caption-weight);
	}

	.word {
		color: var(--text-muted);
	}
</style>
