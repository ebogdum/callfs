<script lang="ts" module>
	export type NameBadge = 'locked' | 'split' | 'remote' | 'backup' | 'link' | 'hidden';

	export const TOKENS = [
		'--name-cell-fg',
		'--name-cell-icon-fg',
		'--name-cell-badge-fg',
		'--name-cell-input-bg'
	] as const;
</script>

<script lang="ts">
	import { tick } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { iconForKind, kindFromName, type FileKind } from '$lib/format/mime';
	import type { IconName } from '../types';
	import Link from '../atoms/Link.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import Icon from '../quarks/Icon.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	const BADGE_ICONS: Record<NameBadge, IconName> = {
		locked: 'lock',
		split: 'split',
		remote: 'server',
		backup: 'backup',
		link: 'link',
		hidden: 'eye-off'
	};

	const BADGE_KEYS: Record<NameBadge, string> = {
		locked: 'status.file.locked',
		split: 'status.file.split',
		remote: 'status.file.remote',
		backup: 'status.file.backup_failed',
		link: 'cells.name.badge_link',
		hidden: 'status.file.hidden'
	};

	const KIND_BY_MIME = new Map<string, FileKind>([
		['application/pdf', 'pdf'],
		['image', 'image'],
		['video', 'video'],
		['audio', 'audio']
	]);

	const fileKind = (name: string, mime: string | undefined): FileKind => {
		if (undefined === mime) {
			return kindFromName(name);
		}
		return KIND_BY_MIME.get(mime) ?? KIND_BY_MIME.get(mime.split('/')[0]) ?? kindFromName(name);
	};

	const baseLength = (name: string): number => {
		const dot = name.lastIndexOf('.');
		return 0 < dot ? dot : name.length;
	};

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'children' | 'oncancel'> {
		name: string;
		kind: 'file' | 'folder';
		mime?: string;
		href?: string;
		badges?: NameBadge[];
		editing?: boolean;
		onrename?: (name: string) => void;
		oncancel?: () => void;
	}

	let {
		name,
		kind,
		mime,
		href,
		badges = [],
		editing = false,
		onrename,
		oncancel,
		class: className,
		...rest
	}: Props = $props();

	const icon = $derived<IconName>('folder' === kind ? 'folder' : iconForKind(fileKind(name, mime)));

	let input: HTMLInputElement | undefined = $state();
	let draft = $state('');

	$effect(() => {
		const element = input;
		if (!editing || undefined === element) {
			return;
		}
		draft = name;
		element.focus();
		void tick().then(() => element.setSelectionRange(0, baseLength(name)));
	});

	const onKeydown = (event: KeyboardEvent): void => {
		if ('Enter' === event.key) {
			event.preventDefault();
			const value = draft.trim();
			if ('' === value) {
				oncancel?.();
				return;
			}
			onrename?.(value);
		} else if ('Escape' === event.key) {
			event.preventDefault();
			draft = name;
			oncancel?.();
		}
	};
</script>

<span class={['name-cell', className]} data-kind={kind} {...rest}>
	<Icon name={icon} size="md" class="kind-icon" />
	{#if editing}
		<input
			bind:this={input}
			bind:value={draft}
			class="rename"
			type="text"
			aria-label={t('cells.name.rename_label', { name })}
			onkeydown={onKeydown}
		/>
	{:else}
		<span class="name-text">
			{#if href}
				<Link {href}>
					<MiddleTruncate text={name} />
				</Link>
			{:else}
				<MiddleTruncate text={name} />
			{/if}
		</span>
		{#if 0 < badges.length}
			<span class="badges">
				{#each badges as badge (badge)}
					<Tooltip content={t(BADGE_KEYS[badge])}>
						{#snippet children()}
							<span class="badge" data-badge={badge}>
								<Icon name={BADGE_ICONS[badge]} size="xs" />
								<VisuallyHidden>{t(BADGE_KEYS[badge])}</VisuallyHidden>
							</span>
						{/snippet}
					</Tooltip>
				{/each}
			</span>
		{/if}
	{/if}
</span>

<style>
	.name-cell {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
		max-inline-size: 100%;
		font-size: var(--table-text);
		color: var(--name-cell-fg, var(--text));
	}

	.name-cell :global(.kind-icon) {
		flex: none;
		color: var(--name-cell-icon-fg, var(--text-muted));
	}

	.name-text {
		display: block;
		min-inline-size: 0;
		max-inline-size: 100%;
	}

	.badges {
		display: inline-flex;
		align-items: center;
		flex: none;
		gap: var(--space-1);
	}

	.badge {
		display: inline-flex;
		align-items: center;
		padding: var(--space-0-5);
		color: var(--name-cell-badge-fg, var(--text-muted));
		border-radius: var(--radius-xs);
	}

	.rename {
		flex: 1 1 auto;
		min-inline-size: 0;
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		font: inherit;
		color: var(--text);
		background: var(--name-cell-input-bg, var(--surface-raised));
		border: var(--hairline) solid var(--border-control);
		border-radius: var(--radius-sm);
		outline: var(--focus-width) solid transparent;
	}

	.rename:focus-visible {
		box-shadow: var(--focus-ring);
	}
</style>
