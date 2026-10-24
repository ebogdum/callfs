<script lang="ts" module>
	export const TOKENS = [
		'--code-view-bg',
		'--code-view-key',
		'--code-view-string',
		'--code-view-number',
		'--code-view-keyword',
		'--code-view-comment',
	] as const;

	export type CodeLanguage = 'yaml' | 'json' | 'env' | 'shell' | 'http';
	export type CodeTokenKind = 'plain' | 'comment' | 'key' | 'string' | 'number' | 'keyword' | 'punct' | 'variable' | 'option' | 'url';

	export interface CodeToken {
		kind: CodeTokenKind;
		text: string;
	}

	const RULES: Record<CodeLanguage, [CodeTokenKind, RegExp][]> = {
		yaml: [
			['comment', /#.*/],
			['key', /(?:"(?:[^"\\]|\\.)*"|'[^']*'|[A-Za-z_][\w.-]*)(?=\s*:(?:\s|$))/],
			['string', /"(?:[^"\\]|\\.)*"|'[^']*'/],
			['keyword', /\b(?:true|false|null|yes|no|on|off)\b/],
			['number', /\b\d+(?:\.\d+)?(?:ms|[smhd])?\b/],
		],
		json: [
			['key', /"(?:[^"\\]|\\.)*"(?=\s*:)/],
			['string', /"(?:[^"\\]|\\.)*"/],
			['keyword', /\b(?:true|false|null)\b/],
			['number', /-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/],
			['punct', /[{}[\],:]/],
		],
		env: [
			['comment', /#.*/],
			['key', /^[A-Za-z_]\w*(?==)/],
			['string', /"(?:[^"\\]|\\.)*"|'[^']*'/],
			['variable', /\$\{?[A-Za-z_]\w*\}?/],
			['number', /\b\d+(?:\.\d+)?\b/],
			['punct', /=/],
		],
		shell: [
			['comment', /#.*/],
			['string', /"(?:[^"\\]|\\.)*"|'[^']*'/],
			['variable', /\$\{?[A-Za-z_]\w*\}?/],
			['option', /(?:^|(?<=\s))--?[A-Za-z][\w-]*/],
			['keyword', /\b(?:export|if|then|else|fi|for|do|done|case|esac|while|function)\b/],
		],
		http: [
			['keyword', /^(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|HTTP\/[\d.]+)\b/],
			['url', /https?:\/\/\S+/],
			['key', /^[A-Za-z][\w-]*(?=:)/],
			['string', /"(?:[^"\\]|\\.)*"/],
			['number', /\b\d+\b/],
		],
	};

	export function tokenizeLine(line: string, language: CodeLanguage): CodeToken[] {
		const rules = RULES[language];
		const pattern = new RegExp(rules.map(([, source]) => `(${source.source})`).join('|'), 'g');
		const tokens: CodeToken[] = [];
		let cursor = 0;
		for (const match of line.matchAll(pattern)) {
			const start = match.index;
			if (cursor < start) {
				tokens.push({ kind: 'plain', text: line.slice(cursor, start) });
			}
			const ruleIndex = match.findIndex((group, position) => 0 < position && undefined !== group) - 1;
			tokens.push({ kind: rules[ruleIndex][0], text: match[0] });
			cursor = start + match[0].length;
		}
		if (cursor < line.length) {
			tokens.push({ kind: 'plain', text: line.slice(cursor) });
		}
		return tokens;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		code: string;
		language: CodeLanguage;
		label: string;
		lineNumbers?: boolean;
		maxHeight?: string;
		class?: string;
	}

	let { code, language, label, lineNumbers = false, maxHeight, class: className, ...rest }: Props = $props();

	const lines = $derived(code.split('\n'));
</script>

<div class={['code-view', className]} {...rest}>
	<Stack gap="2">
		<Cluster gap="2" justify="space-between" align="center">
			<Text size="caption" tone="muted">{label}</Text>
			<CopyButton value={code} variant="button" label={t('settingsui.code.copy')} />
		</Cluster>
		<Scroll axis="both" label={label}>
			<pre class="code" style:max-block-size={maxHeight}>{#each lines as line, index (index)}<span class="line">{#if lineNumbers}<span class="gutter" aria-hidden="true">{index + 1}</span>{/if}{#each tokenizeLine(line, language) as token, tokenIndex (tokenIndex)}<span class="token {token.kind}">{token.text}</span>{/each}</span>{/each}</pre>
		</Scroll>
	</Stack>
</div>

<style>
	.code-view {
		min-inline-size: 0;
	}

	.code {
		margin: 0;
		padding: var(--space-3);
		border-radius: var(--radius-md);
		background: var(--code-view-bg, var(--code-bg));
		color: var(--text);
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		tab-size: 2;
	}

	.line {
		display: block;
		white-space: pre;
	}

	.gutter {
		display: inline-block;
		min-inline-size: var(--space-6);
		padding-inline-end: var(--space-3);
		color: var(--text-muted);
		text-align: end;
		user-select: none;
	}

	.key {
		color: var(--code-view-key, var(--text-link));
	}

	.string,
	.url {
		color: var(--code-view-string, var(--ok-text));
	}

	.number {
		color: var(--code-view-number, var(--warn-text));
	}

	.keyword,
	.option {
		color: var(--code-view-keyword, var(--accent-text));
	}

	.comment {
		color: var(--code-view-comment, var(--text-muted));
		font-style: italic;
	}

	.punct,
	.variable {
		color: var(--text-muted);
	}
</style>
