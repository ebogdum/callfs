<script lang="ts" module>
	export interface BannerAction {
		label: string;
		run: () => void;
	}

	export const TOKENS = ['--banner-bg', '--banner-border', '--banner-radius', '--banner-pad'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import type { Tone } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		tone: Tone;
		title: string;
		body?: string;
		action?: BannerAction;
		dismissible?: boolean;
		ondismiss?: () => void;
	}

	let { tone, title, body, action, dismissible = false, ondismiss, class: className, ...rest }: Props = $props();

	let dismissed = $state(false);

	const dismiss = (): void => {
		dismissed = true;
		ondismiss?.();
	};
</script>

{#if !dismissed}
	<div
		{...rest}
		class={['banner', className]}
		role={'danger' === tone ? 'alert' : 'status'}
		data-tone={tone}
	>
		<span class="glyph"><StatusGlyph {tone} /></span>
		<div class="text">
			<p class="title">{title}</p>
			{#if undefined !== body}<p class="body">{body}</p>{/if}
		</div>
		{#if undefined !== action || dismissible}
			<div class="actions">
				{#if undefined !== action}
					<Button variant="secondary" size="sm" onclick={action.run}>{action.label}</Button>
				{/if}
				{#if dismissible}
					<Button variant="ghost" size="sm" iconStart="x" onclick={dismiss}>{t('common.close')}</Button>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<style>
	.banner {
		--tone: var(--text-muted);
		--tone-soft: var(--surface-sunken);
		--tone-border: var(--border-subtle);
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-2);
		padding: var(--banner-pad, var(--space-3));
		background: var(--banner-bg, var(--tone-soft));
		border: var(--hairline) solid var(--banner-border, var(--tone-border));
		border-radius: var(--banner-radius, var(--radius-md));
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
	}

	.banner[data-tone='accent'] {
		--tone: var(--accent-text);
		--tone-soft: var(--accent-soft);
		--tone-border: var(--accent-border);
	}

	.banner[data-tone='ok'] {
		--tone: var(--ok);
		--tone-soft: var(--ok-soft);
		--tone-border: var(--ok-border);
	}

	.banner[data-tone='warn'] {
		--tone: var(--warn);
		--tone-soft: var(--warn-soft);
		--tone-border: var(--warn-border);
	}

	.banner[data-tone='danger'] {
		--tone: var(--danger);
		--tone-soft: var(--danger-soft);
		--tone-border: var(--danger-border);
	}

	.banner[data-tone='info'] {
		--tone: var(--info);
		--tone-soft: var(--info-soft);
		--tone-border: var(--info-border);
	}

	.glyph {
		display: inline-flex;
		color: var(--tone);
	}

	.text {
		display: grid;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.title {
		margin: 0;
		font-weight: 600;
	}

	.body {
		margin: 0;
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
</style>
