<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import HelpText from '../atoms/HelpText.svelte';
	import Num from '../atoms/Num.svelte';
	import Stack from '../quarks/Stack.svelte';
	import type { Chip, Facet } from '../types';

	type FacetValue = Facet['values'][number];
	type ValueState = 'included' | 'excluded' | 'off';

	interface Props extends HTMLAttributes<HTMLElement> {
		facets: Facet[];
		include?: Chip[];
		label: string;
	}

	let { facets, include = $bindable([]), label, class: className, ...rest }: Props = $props();

	const hintId = $props.id();

	function chipFor(facet: Facet, value: FacetValue): Chip | undefined {
		return include.find((chip) => chip.facet === facet.key && chip.value === value.value);
	}

	function stateOf(facet: Facet, value: FacetValue): ValueState {
		const chip = chipFor(facet, value);
		if (undefined === chip) {
			return 'off';
		}
		return '!' === chip.op ? 'excluded' : 'included';
	}

	function choose(event: MouseEvent, facet: Facet, value: FacetValue) {
		event.preventDefault();
		const desired: Chip['op'] = event.altKey ? '!' : ':';
		const current = chipFor(facet, value);
		const remaining = include.filter((chip) => !(chip.facet === facet.key && chip.value === value.value));
		include =
			undefined !== current && current.op === desired
				? remaining
				: [...remaining, { facet: facet.key, op: desired, value: value.value, label: value.label }];
	}
</script>

<section class={['facet-sidebar', className]} aria-label={label} {...rest}>
	<Stack gap="4">
		<h2 class="title">{label}</h2>
		<HelpText id={hintId}>{t('u095query.exclude_hint')}</HelpText>
		{#if 0 === facets.length}
			<p class="empty">{t('u095query.no_filters')}</p>
		{:else}
			<Stack gap="3">
				{#each facets as facet (facet.key)}
					<details class="group" open>
						<summary>{facet.label}</summary>
						<Stack gap="2">
							{#if 0 === facet.values.length}
								<p class="empty">{t('u095query.no_values')}</p>
							{:else}
								<ul class="values" aria-describedby={hintId}>
									{#each facet.values as value (value.value)}
										<li>
											<label class="value" data-state={stateOf(facet, value)}>
												<input
													type="checkbox"
													checked={'included' === stateOf(facet, value)}
													onclick={(event) => choose(event, facet, value)}
												/>
												<span class="value-label">{value.label}</span>
												{#if 'number' === typeof value.count}
													<Num value={value.count} />
												{/if}
												{#if 'excluded' === stateOf(facet, value)}
													<span class="excluded">{t('u095query.excluded')}</span>
												{/if}
											</label>
										</li>
									{/each}
								</ul>
							{/if}
						</Stack>
					</details>
				{/each}
			</Stack>
		{/if}
	</Stack>
</section>

<style>
	.facet-sidebar {
		inline-size: 100%;
		min-inline-size: 0;
	}

	.title {
		margin: 0;
		font: inherit;
		font-weight: 600;
	}

	.group > summary {
		display: flex;
		align-items: center;
		min-block-size: var(--control-h);
		padding-inline: var(--space-1);
		border-radius: var(--radius-sm);
		font-weight: 600;
		cursor: pointer;
	}

	.group > summary:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.values {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.value {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-1);
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	@media (hover: hover) {
		.value:hover {
			background: var(--surface-hover);
		}
	}

	.value[data-state='included'] {
		background: var(--surface-selected);
	}

	.value input {
		inline-size: var(--space-4);
		block-size: var(--space-4);
		margin: 0;
		accent-color: var(--accent);
	}

	.value input:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.value-label {
		flex: 1 1 auto;
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	.excluded {
		color: var(--danger-text);
	}

	.empty {
		margin: 0;
		color: var(--text-muted);
	}
</style>
