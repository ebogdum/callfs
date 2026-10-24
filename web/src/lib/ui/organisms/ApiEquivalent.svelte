<script lang="ts" module>
	export const TOKENS = ['--api-equivalent-gap', '--api-equivalent-border', '--api-equivalent-radius'] as const;

	export interface ApiRequest {
		method: string;
		url: string;
		headers?: Record<string, string>;
		body?: unknown;
	}

	function quoteSingle(value: string): string {
		return `'${value.replaceAll("'", "'\\''")}'`;
	}

	function bodyJson(request: ApiRequest): string | null {
		if (undefined === request.body) {
			return null;
		}
		return JSON.stringify(request.body) ?? null;
	}

	function buildCurl(request: ApiRequest): string {
		const lines = [`curl -X ${request.method} ${quoteSingle(request.url)}`, '  -H "Authorization: Bearer $CALLFS_KEY"'];
		for (const [name, value] of Object.entries(request.headers ?? {})) {
			lines.push(`  -H ${quoteSingle(`${name}: ${value}`)}`);
		}
		const json = bodyJson(request);
		if (null !== json) {
			lines.push("  -H 'Content-Type: application/json'", `  -d ${quoteSingle(json)}`);
		}
		return lines.join(' \\\n');
	}

	function buildHttp(request: ApiRequest): string {
		const lines = [`${request.method} ${request.url} HTTP/1.1`, 'Authorization: Bearer $CALLFS_KEY'];
		for (const [name, value] of Object.entries(request.headers ?? {})) {
			lines.push(`${name}: ${value}`);
		}
		const json = bodyJson(request);
		if (null !== json) {
			lines.push('Content-Type: application/json', '', JSON.stringify(request.body, null, 2));
		}
		return lines.join('\n');
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { Collapsible } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		request: ApiRequest;
		class?: string;
	}

	let { request, class: className, ...rest }: Props = $props();

	const curl = $derived(buildCurl(request));
	const http = $derived(buildHttp(request));
	const json = $derived(bodyJson(request));
</script>

<div class={['api-equivalent', className]} {...rest}>
	<Collapsible.Root>
		<Collapsible.Trigger class="api-trigger">{t('support.api.summary')}</Collapsible.Trigger>
		<Collapsible.Content class="api-panel">
			<Stack gap="4">
				<Text size="sm" tone="muted">{t('support.api.key_help')}</Text>
				<Stack gap="2">
					<Cluster justify="space-between" align="center" wrap gap="2">
						<Text size="sm" weight={600}>{t('support.api.command_label')}</Text>
						<CopyButton value={curl} variant="button" />
					</Cluster>
					<pre class="api-code"><code>{curl}</code></pre>
				</Stack>
				<Stack gap="2">
					<Text size="sm" weight={600}>{t('support.api.http_label')}</Text>
					<pre class="api-code"><code>{http}</code></pre>
				</Stack>
				{#if null !== json}
					<Stack gap="2">
						<Cluster justify="space-between" align="center" wrap gap="2">
							<Text size="sm" weight={600}>{t('support.api.body_label')}</Text>
							<CopyButton value={json} variant="button" />
						</Cluster>
						<pre class="api-code"><code>{json}</code></pre>
					</Stack>
				{/if}
			</Stack>
		</Collapsible.Content>
	</Collapsible.Root>
</div>

<style>
	.api-equivalent {
		min-inline-size: 0;
	}

	:global(.api-trigger) {
		display: inline-flex;
		align-items: center;
		min-block-size: var(--target-min, var(--control-h));
		padding-inline: var(--space-2);
		border: var(--hairline) solid var(--api-equivalent-border, var(--border-subtle));
		border-radius: var(--api-equivalent-radius, var(--radius-md));
		background: transparent;
		color: var(--text);
		font: inherit;
		cursor: pointer;
	}

	:global(.api-trigger:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	:global(.api-panel) {
		padding-block-start: var(--api-equivalent-gap, var(--space-4));
		min-inline-size: 0;
	}

	.api-code {
		margin: 0;
		padding: var(--space-3);
		overflow-x: auto;
		background: var(--code-bg);
		border-radius: var(--radius-sm);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		white-space: pre;
	}
</style>
