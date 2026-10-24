<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLLIElement> {
		label: string;
		detail?: string;
		mono?: boolean;
		lead?: Snippet;
		trail?: Snippet;
		below?: Snippet;
	}

	let { label, detail, mono = false, lead, trail, below, class: className, ...rest }: Props = $props();
</script>

<li class={['settings-item', className]} {...rest}>
	{#if lead}<span class="lead">{@render lead()}</span>{/if}
	<div class="text">
		<span class="label" class:mono>{label}</span>
		{#if detail}<span class="detail">{detail}</span>{/if}
		{#if below}<div class="below">{@render below()}</div>{/if}
	</div>
	{#if trail}<div class="trail">{@render trail()}</div>{/if}
</li>

<style>
	.settings-item {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		min-inline-size: 0;
		padding: var(--space-3) var(--card-pad);
	}

	.settings-item + :global(.settings-item) {
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.lead {
		display: inline-flex;
		flex: none;
		align-items: center;
		min-block-size: var(--text-body-line);
	}

	.text {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.label {
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-label-weight);
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.label.mono {
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
	}

	.detail {
		max-inline-size: var(--read-max);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.below {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-block-start: var(--space-2);
	}

	.trail {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		min-block-size: var(--text-body-line);
	}

	@media (max-width: 639px) {
		.settings-item {
			flex-wrap: wrap;
		}

		.trail {
			flex-basis: 100%;
			justify-content: flex-start;
		}
	}
</style>
