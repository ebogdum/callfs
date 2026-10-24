<script lang="ts" module>
	export interface StepperStep {
		id: string;
		label: string;
		href?: string;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		steps: StepperStep[];
		current: number;
		class?: string;
	}

	let { steps, current, class: className, ...rest }: Props = $props();

	function stateOf(position: number): 'done' | 'current' | 'upcoming' {
		if (0 < current - position) {
			return 'done';
		}
		return current === position ? 'current' : 'upcoming';
	}
</script>

<div class={['stepper', className]} {...rest}>
	<Stack gap="2">
		<p class="position">{t('protection.stepper.position', { current, total: steps.length })}</p>
		<ol class="steps">
			{#each steps as step, index (step.id)}
				{@const position = index + 1}
				<li class="step" data-state={stateOf(position)} aria-current={current === position ? 'step' : undefined}>
					{#if 0 < current - position && step.href}
						<a class="label" href={step.href}>{step.label}</a>
					{:else}
						<span class="label">{step.label}</span>
					{/if}
				</li>
			{/each}
		</ol>
	</Stack>
</div>

<style>
	.stepper {
		min-inline-size: 0;
	}

	.position {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
	}

	.steps {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.step {
		display: inline-flex;
		align-items: center;
		min-block-size: var(--control-h);
		padding-inline: var(--control-px);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-full);
		color: var(--text-muted);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
	}

	.step[data-state='current'] {
		border-color: var(--accent);
		color: var(--text);
		font-weight: var(--text-title-sm-weight);
	}

	.step[data-state='done'] {
		color: var(--text);
	}

	.label:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
		border-radius: var(--radius-xs);
	}
</style>
