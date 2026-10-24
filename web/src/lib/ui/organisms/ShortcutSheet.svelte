<script lang="ts" module>
	export const TOKENS = ['--shortcut-sheet-bg', '--shortcut-sheet-border', '--shortcut-sheet-radius'] as const;

	export interface KeymapEntry {
		id: string;
		label: string;
		keys: string[][];
	}

	export interface KeymapSection {
		id: string;
		title: string;
		entries: KeymapEntry[];
	}
</script>

<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import { fuzzyMatch } from '$lib/format/fuzzy';
	import HelpText from '../atoms/HelpText.svelte';
	import Input from '../atoms/Input.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import Label from '../atoms/Label.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props {
		open: boolean;
		keymap: KeymapSection[];
		singleKeyShortcuts?: boolean;
		class?: string;
	}

	let {
		open = $bindable(false),
		keymap,
		singleKeyShortcuts = true,
		class: className
	}: Props = $props();

	const baseId = $props.id();
	const searchId = `${baseId}-search`;
	const toggleStateId = `${baseId}-single-key`;

	let query = $state('');
	const trimmed = $derived(query.trim());

	const visibleSections = $derived(
		keymap
			.map((section) => ({
				...section,
				entries: section.entries.filter((entry) => null !== fuzzyMatch(trimmed, entry.label))
			}))
			.filter((section) => 0 < section.entries.length)
	);

	function handleOpenChange(next: boolean): void {
		if (!next) {
			query = '';
		}
	}
</script>

<Dialog.Root bind:open onOpenChange={handleOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay class="sheet-scrim" />
		<Dialog.Content class={['shortcut-sheet', className].filter(Boolean).join(' ')}>
			<Stack gap="4">
				<Dialog.Title class="sheet-title">{t('palette.shortcuts.title')}</Dialog.Title>
				<Stack gap="1-5">
					<Label for={searchId}>{t('palette.shortcuts.search_label')}</Label>
					<Input
						id={searchId}
						type="search"
						bind:value={query}
						placeholder={t('palette.shortcuts.search_placeholder')}
						autocomplete="off"
						aria-describedby={toggleStateId}
					/>
					<HelpText id={toggleStateId}>
						{singleKeyShortcuts ? t('palette.shortcuts.single_on') : t('palette.shortcuts.single_off')}
					</HelpText>
				</Stack>
				<Scroll axis="y" label={t('palette.shortcuts.title')}>
					{#if 0 < visibleSections.length}
						<Stack gap="6">
							{#each visibleSections as section (section.id)}
								<section aria-label={section.title} class="sheet-section">
									<Stack gap="2">
										<Text size="title-sm">{section.title}</Text>
										<ul class="sheet-list">
											{#each section.entries as entry (entry.id)}
												<li class="sheet-row">
													<Text>{entry.label}</Text>
													<Cluster as="span" gap="2" class="sheet-keys">
														{#each entry.keys as alternative, alternativeIndex (alternativeIndex)}
															{#if 0 < alternativeIndex}
																<Text size="sm" tone="muted">{t('palette.shortcuts.or')}</Text>
															{/if}
															<Kbd keys={alternative} />
														{/each}
													</Cluster>
												</li>
											{/each}
										</ul>
									</Stack>
								</section>
							{/each}
						</Stack>
					{:else}
						<div class="sheet-empty" role="status">
							<Text tone="muted">{t('palette.shortcuts.empty', { query: trimmed })}</Text>
						</div>
					{/if}
				</Scroll>
			</Stack>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.sheet-scrim) {
		position: fixed;
		inset: 0;
		background: var(--scrim);
		z-index: var(--z-dialog);
	}

	:global(.shortcut-sheet) {
		position: fixed;
		inset-block: var(--space-10) var(--space-6);
		inset-inline: var(--space-4);
		margin-inline: auto;
		inline-size: min(var(--dialog-md), 100%);
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--space-6);
		z-index: var(--z-dialog);
		background: var(--shortcut-sheet-bg, var(--surface-overlay));
		color: var(--text);
		border: var(--hairline) solid var(--shortcut-sheet-border, var(--border-subtle));
		border-radius: var(--shortcut-sheet-radius, var(--radius-xl));
		box-shadow: var(--shadow-xl);
		overflow: hidden;
	}

	:global(.sheet-title) {
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
	}

	.sheet-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sheet-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
		min-block-size: var(--control-h);
	}

	.sheet-empty {
		padding: var(--space-4);
	}
</style>
