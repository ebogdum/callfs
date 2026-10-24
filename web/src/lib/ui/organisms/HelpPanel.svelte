<script lang="ts" module>
	import { GLOSSARY, type GlossaryKey } from '$lib/copy/glossary';

	export const TOKENS = ['--help-panel-bg', '--help-panel-border', '--help-panel-width', '--help-panel-pad'] as const;

	export interface HelpShortcut {
		label: string;
		keys: string[];
	}

	function topicKey(topic: string | undefined): GlossaryKey | null {
		if (undefined === topic) {
			return null;
		}
		const key = topic.replaceAll('-', '_');
		return Object.hasOwn(GLOSSARY, key) ? (key as GlossaryKey) : null;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Input from '../atoms/Input.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import Text from '../atoms/Text.svelte';
	import FormField from '../molecules/FormField.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Pick<HTMLAttributes<HTMLDivElement>, 'id' | 'style'> {
		open?: boolean;
		topic?: string;
		ontopic: (key: GlossaryKey) => void;
		diagnostics: string;
		shortcuts?: HelpShortcut[];
		class?: string;
	}

	let {
		open = $bindable(false),
		topic,
		ontopic,
		diagnostics,
		shortcuts = [],
		class: className,
		...rest
	}: Props = $props();

	let query = $state('');
	const trimmedQuery = $derived(query.trim().toLowerCase());
	const selectedKey = $derived(topicKey(topic));
	const matches = $derived(
		(Object.keys(GLOSSARY) as GlossaryKey[]).filter((key) => {
			const entry = GLOSSARY[key];
			return (
				0 === trimmedQuery.length ||
				entry.word.toLowerCase().includes(trimmedQuery) ||
				entry.means.toLowerCase().includes(trimmedQuery)
			);
		})
	);
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="help-scrim" />
		<Dialog.Content class={undefined === className ? 'help-panel' : `help-panel ${className}`} {...rest} id={rest.id ?? undefined}>
			<Stack gap="6">
				<Cluster justify="space-between" align="center" wrap gap="2">
					<Dialog.Title>
						<Text size="title-sm">{t('support.help.title')}</Text>
					</Dialog.Title>
					<Dialog.Close>
						{#snippet child({ props })}
							<Button variant="ghost" size="sm" {...props}>{t('common.close')}</Button>
						{/snippet}
					</Dialog.Close>
				</Cluster>
				<FormField label={t('support.help.search_label')} for="help-search">
					{#snippet control({ id, describedBy })}
						<Input {id} type="search" bind:value={query} autocomplete="off" aria-describedby={describedBy} />
					{/snippet}
				</FormField>
				{#if null !== selectedKey}
					<section aria-label={t('support.help.topic_label')} class="help-topic">
						<Stack gap="1-5">
							<Text weight={600}>{GLOSSARY[selectedKey].word}</Text>
							<Text tone="muted">{GLOSSARY[selectedKey].means}</Text>
						</Stack>
					</section>
				{/if}
				<Scroll axis="y" label={t('support.help.terms_label')}>
					{#if 0 === matches.length}
						<Text tone="muted">{t('support.help.no_match', { query: query.trim() })}</Text>
					{:else}
						<ul class="help-terms">
							{#each matches as key (key)}
								<li>
									<Stack gap="1">
										<Button
											variant="ghost"
											size="sm"
											aria-pressed={selectedKey === key}
											onclick={() => ontopic(key)}>{GLOSSARY[key].word}</Button
										>
										<Text size="sm" tone="muted">{GLOSSARY[key].means}</Text>
									</Stack>
								</li>
							{/each}
						</ul>
					{/if}
				</Scroll>
				{#if 0 < shortcuts.length}
					<section aria-label={t('support.help.shortcuts_label')}>
						<Stack gap="2">
							<Text size="title-sm">{t('support.help.shortcuts_label')}</Text>
							{#each shortcuts as shortcut (shortcut.label)}
								<Cluster justify="space-between" align="center" wrap gap="2">
									<Text>{shortcut.label}</Text>
									<Kbd keys={shortcut.keys} />
								</Cluster>
							{/each}
						</Stack>
					</section>
				{/if}
				<Stack gap="2">
					<Text size="sm" tone="muted">{t('support.help.report_help')}</Text>
					<CopyButton value={diagnostics} label={t('support.help.report')} variant="button" />
				</Stack>
			</Stack>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.help-scrim) {
		position: fixed;
		inset: 0;
		background: var(--scrim);
		z-index: var(--z-dialog);
	}

	:global(.help-panel) {
		position: fixed;
		inset-block: 0;
		inset-inline-end: 0;
		inline-size: min(var(--help-panel-width, var(--dialog-md)), 100%);
		padding: var(--help-panel-pad, var(--space-6));
		overflow-y: auto;
		z-index: var(--z-dialog);
		background: var(--help-panel-bg, var(--surface-overlay));
		color: var(--text);
		border-inline-start: var(--hairline) solid var(--help-panel-border, var(--border-subtle));
		box-shadow: var(--shadow-xl);
	}

	:global(.help-panel:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.help-topic {
		padding: var(--space-4);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
	}

	.help-terms {
		display: grid;
		gap: var(--space-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}
</style>
