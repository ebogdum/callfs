<script lang="ts" module>
	import type { Command, CommandGroup, SearchHit } from '$lib/commands/registry.svelte';

	export const TOKENS = ['--palette-bg', '--palette-border', '--palette-radius'] as const;

	type PaletteRunVariant = 'primary' | 'secondary';

	interface PaletteSegment {
		text: string;
		marked: boolean;
	}

	const groupTitleKeys: Record<CommandGroup, string> = {
		Navigate: 'palette.group.navigate',
		Files: 'palette.group.files',
		Sharing: 'palette.group.sharing',
		Cluster: 'palette.group.cluster',
		Insights: 'palette.group.insights',
		Activity: 'palette.group.activity',
		Settings: 'palette.group.settings',
		Appearance: 'palette.group.appearance',
		Help: 'palette.group.help',
		Account: 'palette.group.account'
	};

	function segmentTitle(title: string, indices: readonly number[]): PaletteSegment[] {
		const marked = new Set(indices);
		const segments: PaletteSegment[] = [];
		for (const [index, char] of [...title].entries()) {
			const isMarked = marked.has(index);
			const last = segments.at(-1);
			if (undefined !== last && last.marked === isMarked) {
				last.text += char;
			} else {
				segments.push({ text: char, marked: isMarked });
			}
		}
		return segments;
	}
</script>

<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import type { CommandContext } from '$lib/commands/registry.svelte';
	import Input from '../atoms/Input.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Label from '../atoms/Label.svelte';
	import Text from '../atoms/Text.svelte';
	import Icon from '../quarks/Icon.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props {
		open: boolean;
		commands: Command[];
		recent: string[];
		context?: CommandContext;
		search: (raw: string) => SearchHit[];
		completePath?: (partial: string) => string | null;
		onrun: (command: Command, variant: PaletteRunVariant) => void;
		class?: string;
	}

	let {
		open = $bindable(false),
		commands,
		recent,
		context,
		search,
		completePath,
		onrun,
		class: className
	}: Props = $props();

	interface PaletteRow {
		hit: SearchHit;
		index: number;
	}

	interface PaletteGroup {
		key: string;
		title: string;
		rows: PaletteRow[];
	}

	const baseId = $props.id();
	const inputId = `${baseId}-input`;
	const listId = `${baseId}-list`;
	const hintId = `${baseId}-hint`;

	let raw = $state('');
	let activeIndex = $state(0);

	const trimmed = $derived(raw.trim());

	function contextReason(command: Command): string | null {
		return undefined === context ? null : (command.disabledReason?.(context) ?? null);
	}

	const recentHits = $derived.by((): SearchHit[] => {
		if ('' !== trimmed) {
			return [];
		}
		return recent.flatMap((id): SearchHit[] => {
			const command = commands.find((entry) => id === entry.id);
			return undefined === command
				? []
				: [{ command, disabledReason: contextReason(command), score: 0, indices: [] }];
		});
	});

	const searchHits = $derived.by((): SearchHit[] => {
		const hits = search(raw);
		if (0 === recentHits.length) {
			return hits;
		}
		return hits.filter((hit) => !recent.includes(hit.command.id));
	});

	const groups = $derived.by((): PaletteGroup[] => {
		let index = 0;
		const result: PaletteGroup[] = [];
		const addRows = (key: string, title: string, hits: SearchHit[]): void => {
			if (0 === hits.length) {
				return;
			}
			result.push({
				key,
				title,
				rows: hits.map((hit) => {
					index += 1;
					return { hit, index: index - 1 };
				})
			});
		};
		addRows('recent', t('palette.group.recent'), recentHits);
		const byGroup = new Map<CommandGroup, SearchHit[]>();
		for (const hit of searchHits) {
			const bucket = byGroup.get(hit.command.group);
			if (undefined === bucket) {
				byGroup.set(hit.command.group, [hit]);
			} else {
				bucket.push(hit);
			}
		}
		for (const [group, hits] of byGroup) {
			addRows(group, t(groupTitleKeys[group]), hits);
		}
		return result;
	});

	const options = $derived(groups.flatMap((group) => group.rows.map((row) => row.hit)));
	const cursor = $derived(0 < options.length ? Math.min(activeIndex, options.length - 1) : -1);
	const active = $derived(0 <= cursor ? options.at(cursor) : undefined);

	const optionId = (index: number): string => `${baseId}-option-${index}`;
	const groupId = (key: string): string => `${baseId}-group-${key}`;

	function resetPalette(): void {
		raw = '';
		activeIndex = 0;
	}

	function handleOpenChange(next: boolean): void {
		if (!next) {
			resetPalette();
		}
	}

	function run(hit: SearchHit, variant: PaletteRunVariant): void {
		if (null !== hit.disabledReason) {
			return;
		}
		if ('secondary' === variant && undefined === hit.command.secondary) {
			return;
		}
		open = false;
		resetPalette();
		onrun(hit.command, variant);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if ('ArrowDown' === event.key) {
			event.preventDefault();
			if (0 < options.length) {
				activeIndex = (cursor + 1) % options.length;
			}
		} else if ('ArrowUp' === event.key) {
			event.preventDefault();
			if (0 < options.length) {
				activeIndex = (cursor - 1 + options.length) % options.length;
			}
		} else if ('Enter' === event.key) {
			if (undefined === active) {
				return;
			}
			event.preventDefault();
			run(active, event.metaKey || event.ctrlKey ? 'secondary' : 'primary');
		} else if ('Tab' === event.key && '/' === raw.charAt(0) && undefined !== completePath) {
			const completed = completePath(raw);
			if (null !== completed) {
				event.preventDefault();
				raw = completed;
				activeIndex = 0;
			}
		}
	}

	function segmentsOf(hit: SearchHit): PaletteSegment[] {
		return segmentTitle(hit.command.title, hit.indices);
	}
</script>

<Dialog.Root bind:open onOpenChange={handleOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay class="palette-scrim" />
		<Dialog.Content class={undefined === className ? 'palette' : `palette ${className}`}>
			<Dialog.Title>
				<VisuallyHidden>{t('palette.title')}</VisuallyHidden>
			</Dialog.Title>
			<div class="palette-head">
				<Label for={inputId}>{t('palette.label')}</Label>
				<Input
					id={inputId}
					role="combobox"
					aria-expanded={0 < options.length}
					aria-controls={listId}
					aria-autocomplete="list"
					aria-describedby={hintId}
					aria-activedescendant={undefined === active ? undefined : optionId(cursor)}
					placeholder={t('palette.placeholder')}
					autocomplete="off"
					spellcheck={false}
					enterkeyhint="go"
					bind:value={raw}
					oninput={() => {
						activeIndex = 0;
					}}
					onkeydown={handleKeydown}
				/>
				<HelpText id={hintId}>{t('palette.hint')}</HelpText>
			</div>
			{#if 0 < options.length}
				<Scroll axis="y" label={t('palette.title')}>
					<div id={listId} role="listbox" aria-label={t('palette.title')} class="palette-list">
						{#each groups as group (group.key)}
							<div role="group" aria-labelledby={groupId(group.key)} class="palette-group">
								<div id={groupId(group.key)} class="palette-group-title">
									<Text size="caption" tone="muted">{group.title}</Text>
								</div>
								{#each group.rows as row (row.hit.command.id)}
									{@const hit = row.hit}
									{@const icon = hit.command.icon}
									{@const shortcut = hit.command.shortcut}
									<div
										id={optionId(row.index)}
										role="option"
										aria-selected={row.index === cursor}
										aria-disabled={null !== hit.disabledReason}
										data-state={row.index === cursor ? 'selected' : undefined}
										class="palette-option"
										onclick={() => run(hit, 'primary')}
									>
										{#if undefined !== icon}
											<Icon name={icon} />
										{/if}
										<span class="palette-title">
											{#each segmentsOf(hit) as segment, segmentIndex (segmentIndex)}{#if segment.marked}<mark>{segment.text}</mark>{:else}{segment.text}{/if}{/each}
										</span>
										{#if null !== hit.disabledReason}
											<Text size="sm" tone="muted">{hit.disabledReason}</Text>
										{/if}
										{#if undefined !== shortcut}
											<Kbd keys={shortcut} />
										{/if}
									</div>
								{/each}
							</div>
						{/each}
					</div>
				</Scroll>
			{:else}
				<div class="palette-empty" role="status">
					<Text tone="muted">{'' === trimmed ? t('palette.none') : t('palette.empty', { query: trimmed })}</Text>
				</div>
			{/if}
			<Cluster as="div" gap="2" class="palette-foot">
				<Kbd keys={['enter']} />
				<Text size="sm" tone="muted">{t('palette.footer.run')}</Text>
				{#if undefined !== active && undefined !== active.command.secondary}
					<Kbd keys={['mod', 'enter']} />
					<Text size="sm" tone="muted">{active.command.secondary.title}</Text>
				{/if}
				{#if '/' === raw.charAt(0) && undefined !== completePath}
					<Kbd keys={['Tab']} />
					<Text size="sm" tone="muted">{t('palette.footer.complete')}</Text>
				{/if}
			</Cluster>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	:global(.palette-scrim) {
		position: fixed;
		inset: 0;
		background: var(--scrim);
		z-index: var(--z-palette);
	}

	:global(.palette) {
		position: fixed;
		inset-block-start: calc(100% / 3);
		inset-inline: var(--space-4);
		margin-inline: auto;
		inline-size: min(var(--palette-w), 100%);
		max-block-size: calc(100% * 2 / 3 - var(--space-4));
		display: flex;
		flex-direction: column;
		z-index: var(--z-palette);
		background: var(--palette-bg, var(--surface-overlay));
		color: var(--text);
		border: var(--hairline) solid var(--palette-border, var(--border-subtle));
		border-radius: var(--palette-radius, var(--radius-xl));
		box-shadow: var(--shadow-xl);
		overflow: hidden;
	}

	.palette-head {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
		padding: var(--space-3);
	}

	.palette-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-1);
	}

	.palette-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.palette-group-title {
		padding-inline: var(--space-2);
	}

	.palette-option {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		cursor: pointer;
	}

	.palette-option[data-state='selected'] {
		background: var(--surface-selected);
	}

	@media (hover: hover) {
		.palette-option:hover {
			background: var(--surface-hover);
		}
	}

	@media (pointer: coarse) {
		.palette-option {
			min-block-size: var(--touch-target);
		}
	}

	.palette-option[aria-disabled='true'] {
		color: var(--text-faint);
		cursor: default;
	}

	.palette-title {
		flex: 1 1 auto;
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	mark {
		background: var(--accent-soft);
		color: var(--accent-text);
		border-radius: var(--radius-xs);
	}

	.palette-empty {
		padding: var(--space-4);
	}

	:global(.palette-foot) {
		padding: var(--space-2) var(--space-3);
		background: var(--bg-subtle);
	}
</style>
