<script lang="ts" module>
	export const TOKENS = [
		'--connection-pill-radius',
		'--connection-pill-fg',
		'--connection-pill-bg',
		'--connection-pill-border'
	] as const;

	export type ConnectionState = 'live' | 'stale' | 'reconnecting' | 'offline' | 'paused';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import StatusGlyph from '$lib/ui/quarks/StatusGlyph.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		state: ConnectionState;
	}

	const CONNECTIONS = {
		live: { tone: 'ok', word: 'u080molecules.connection.live' },
		stale: { tone: 'warn', word: 'u080molecules.connection.stale' },
		reconnecting: { tone: 'progress', word: 'u080molecules.connection.reconnecting' },
		offline: { tone: 'danger', word: 'u080molecules.connection.offline' },
		paused: { tone: 'muted', word: 'u080molecules.connection.paused' }
	} as const;

	let { state, class: className, ...rest }: Props = $props();

	const connection = $derived(CONNECTIONS[state]);
</script>

<span
	class={['connection-pill', className]}
	data-state={state}
	data-tone={connection.tone}
	aria-live="polite"
	{...rest}
>
	<StatusGlyph tone={connection.tone} />
	<span>{t(connection.word)}</span>
</span>

<style>
	.connection-pill {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		padding-inline: var(--space-2);
		padding-block: var(--space-0-5);
		border: var(--hairline) solid var(--connection-pill-border, var(--border-subtle));
		border-radius: var(--connection-pill-radius, var(--radius-full));
		background: var(--connection-pill-bg, var(--surface-sunken));
		color: var(--connection-pill-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		font-weight: 600;
		white-space: nowrap;
	}

	.connection-pill[data-tone='ok'] {
		background: var(--connection-pill-bg, var(--ok-soft));
		border-color: var(--connection-pill-border, var(--ok-border));
		color: var(--connection-pill-fg, var(--ok-text));
	}

	.connection-pill[data-tone='warn'] {
		background: var(--connection-pill-bg, var(--warn-soft));
		border-color: var(--connection-pill-border, var(--warn-border));
		color: var(--connection-pill-fg, var(--warn-text));
	}

	.connection-pill[data-tone='danger'] {
		background: var(--connection-pill-bg, var(--danger-soft));
		border-color: var(--connection-pill-border, var(--danger-border));
		color: var(--connection-pill-fg, var(--danger-text));
	}

	.connection-pill[data-tone='progress'] {
		background: var(--connection-pill-bg, var(--accent-soft));
		border-color: var(--connection-pill-border, var(--accent-border));
		color: var(--connection-pill-fg, var(--accent-text));
	}
</style>
