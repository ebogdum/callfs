<script lang="ts" module>
	export interface MoveOptions {
		storage?: string;
		server?: string;
		overwrite: boolean;
		createParents: boolean;
	}

	export const TOKENS = ['--destination-picker-gap', '--destination-group-gap', '--destination-preview-fg'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Capabilities, FileEntry, Result, ServerInfo } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { parentPath } from '$lib/format/path';
	import Button from '../atoms/Button.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Select from '../atoms/Select.svelte';
	import Switch from '../atoms/Switch.svelte';
	import FormField from '../molecules/FormField.svelte';
	import PathInput from '../molecules/PathInput.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Grid from '../quarks/Grid.svelte';
	import Stack from '../quarks/Stack.svelte';
	import Surface from '../quarks/Surface.svelte';
	import type { UiError } from '../types';
	import FileTree from './FileTree.svelte';

	const STORAGE_COPY: Record<string, string> = {
		localfs: 'fileforms.storage.localfs',
		s3: 'fileforms.storage.s3'
	};

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		start: string;
		value?: string;
		recents: string[];
		pinned: string[];
		load: (path: string) => Promise<Result<FileEntry[], UiError>>;
		storages: string[];
		servers: Pick<ServerInfo, 'instance_id'>[];
		options: MoveOptions;
		capabilities: Capabilities;
		count?: number;
		bytes?: number | null;
		folder?: boolean;
		class?: string;
	}

	let {
		start,
		value = $bindable(start),
		recents,
		pinned,
		load,
		storages,
		servers,
		options = $bindable(),
		capabilities,
		count = 1,
		bytes = null,
		folder = false,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const storageId = `${uid}-storage`;
	const serverId = `${uid}-server`;
	const lockedId = `${uid}-locked`;

	function storageLabel(storage: string): string {
		return Object.hasOwn(STORAGE_COPY, storage) ? t(STORAGE_COPY[storage]) : storage;
	}

	let storageChoice = $state(options.storage ?? '');
	let serverChoice = $state(options.server ?? '');

	$effect(() => {
		options.storage = '' === storageChoice ? undefined : storageChoice;
	});

	$effect(() => {
		options.server = '' === serverChoice ? undefined : serverChoice;
	});

	const storageOptions = $derived([
		{ value: '', label: t('fileforms.move.keep_storage') },
		...storages.map((storage) => ({ value: storage, label: storageLabel(storage) }))
	]);

	const serverOptions = $derived([
		{ value: '', label: t('fileforms.move.keep_server') },
		...servers.map((server) => ({ value: server.instance_id, label: server.instance_id }))
	]);

	const showServer = $derived(capabilities.features.cross_server);
	const crossesStorage = $derived(!folder && undefined !== options.storage);

	const moveSentence = $derived(
		folder
			? t('fileforms.move.preview_folder', { destination: value })
			: null === bytes
				? t('fileforms.move.preview', { count, destination: value })
				: t('fileforms.move.preview_sized', { count, size: formatBytes(bytes).text, destination: value })
	);
	const crossSentence = $derived(
		crossesStorage ? t('fileforms.move.preview_cross', { storage: storageLabel(options.storage ?? '') }) : ''
	);
	const preview = $derived('' === crossSentence ? moveSentence : `${moveSentence} ${crossSentence}`);

	const suggest = (prefix: string): Promise<string[]> => {
		const folderPath = '/' === prefix.charAt(prefix.length - 1) ? prefix : parentPath(prefix);
		return load(folderPath).then((result) =>
			true === result.ok
				? result.value
						.filter((entry) => 'directory' === entry.type && entry.path.startsWith(prefix))
						.map((entry) => `${entry.path}/`)
				: []
		);
	};

	const navigate = (path: string): void => {
		value = path;
	};
</script>

{#snippet pathGroup(label: string, paths: string[])}
	<Stack gap="2">
		<span class="group-label">{label}</span>
		<Cluster gap="2">
			{#each paths as path (path)}
				<Button size="sm" variant="ghost" onclick={() => (value = path)}>{path}</Button>
			{/each}
		</Cluster>
	</Stack>
{/snippet}

<section class={['destination-picker', className]} aria-label={t('fileforms.move.region')} {...rest}>
	<Stack gap="4">
		<PathInput bind:value label={t('fileforms.move.destination')} {suggest} />
		{#if 0 < recents.length}
			{@render pathGroup(t('fileforms.move.recent'), recents)}
		{/if}
		{#if 0 < pinned.length}
			{@render pathGroup(t('fileforms.move.pinned'), pinned)}
		{/if}
		<Stack gap="2">
			<span class="group-label">{t('fileforms.move.browse')}</span>
			<Surface level="sunken" pad="2" radius="md">
				<FileTree root="/" {load} current={value} onnavigate={navigate} />
			</Surface>
		</Stack>
		{#if folder}
			<HelpText id={lockedId}>{t('fileforms.move.folder_locked')}</HelpText>
		{/if}
		<Grid min="12" gap="4">
			<FormField label={t('fileforms.move.storage')} for={storageId}>
				{#snippet control()}
					<Select
						id={storageId}
						bind:value={storageChoice}
						options={storageOptions}
						disabled={folder}
						aria-describedby={folder ? lockedId : undefined}
					/>
				{/snippet}
			</FormField>
			{#if showServer}
				<FormField label={t('fileforms.move.server')} for={serverId}>
					{#snippet control()}
						<Select
							id={serverId}
							bind:value={serverChoice}
							options={serverOptions}
							disabled={folder}
							aria-describedby={folder ? lockedId : undefined}
						/>
					{/snippet}
				</FormField>
			{/if}
		</Grid>
		<Stack gap="3">
			<Switch
				bind:checked={options.overwrite}
				label={t('fileforms.move.overwrite')}
				description={t('fileforms.move.overwrite_help')}
			/>
			<Switch
				bind:checked={options.createParents}
				label={t('fileforms.move.create_parents')}
				description={t('fileforms.move.create_parents_help')}
			/>
		</Stack>
		<p class="preview">{preview}</p>
	</Stack>
</section>

<style>
	.destination-picker {
		display: grid;
		min-inline-size: 0;
	}

	.group-label {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.preview {
		margin: 0;
		color: var(--destination-preview-fg, var(--text));
		overflow-wrap: anywhere;
	}
</style>
