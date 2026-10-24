<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
		footer?: Snippet;
	}

	let { children, footer, class: className, ...rest }: Props = $props();
</script>

<div class={['auth-layout', className]} {...rest}>
	<main class="stage">
		<div class="brand">
			<span class="mark" aria-hidden="true"><Icon name="hard-drive" size="lg" /></span>
			<span class="wordmark">{t('shell.brand.name')}</span>
		</div>
		<section class="card">
			{@render children()}
		</section>
		<footer class="auth-footer">
			{#if footer}
				{@render footer()}
			{:else}
				<span>{t('shell.login.footer')}</span>
			{/if}
		</footer>
	</main>
</div>

<style>
	.auth-layout {
		display: grid;
		place-items: center;
		min-block-size: 100dvh;
		padding-block: var(--space-10);
		padding-inline: var(--gutter);
		background:
			radial-gradient(
				calc(var(--space-4) * 60) calc(var(--space-4) * 30) at 50% -10%,
				color-mix(in srgb, var(--accent) 14%, transparent),
				transparent 70%
			),
			var(--bg);
		color: var(--text);
	}

	.stage {
		display: flex;
		flex-direction: column;
		align-items: stretch;
		gap: var(--space-6);
		inline-size: 100%;
		max-inline-size: calc(var(--space-4) * 24);
	}

	.brand {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
	}

	.mark {
		display: grid;
		place-items: center;
		inline-size: var(--space-8);
		block-size: var(--space-8);
		border-radius: var(--radius-md);
		background: var(--accent);
		color: var(--text-on-accent);
		box-shadow: var(--shadow-sm), var(--highlight-top);
	}

	.wordmark {
		font-size: var(--text-heading-size);
		line-height: var(--text-heading-line);
		font-weight: var(--text-title-weight);
		letter-spacing: var(--text-title-tracking);
	}

	.card {
		padding: var(--space-8);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-xl);
		background: var(--surface-raised);
		box-shadow: var(--shadow-md);
	}

	.auth-footer {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--space-4);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-faint);
		text-align: center;
	}

	@media (max-width: 639px) {
		.card {
			padding: var(--space-6);
		}
	}
</style>
