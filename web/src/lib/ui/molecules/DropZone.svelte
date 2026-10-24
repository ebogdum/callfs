<script lang="ts" module>
	export type DropZoneState = 'idle' | 'dragging' | 'accept' | 'reject';

	export const TOKENS = [
		'--dropzone-bg',
		'--dropzone-bg-active',
		'--dropzone-border',
		'--dropzone-border-active',
		'--dropzone-radius',
		'--dropzone-pad',
	] as const;

	interface Feedback {
		tone: 'accept' | 'reject';
		message: string;
	}

	const matchesPattern = (file: File, pattern: string): boolean => {
		const lowered = pattern.toLowerCase();
		if ('.' === lowered.charAt(0)) {
			return file.name.toLowerCase().endsWith(lowered);
		}
		if (lowered.endsWith('/*')) {
			return file.type.startsWith(lowered.slice(0, -1));
		}
		return lowered === file.type.toLowerCase();
	};

	const hasFiles = (event: DragEvent): boolean =>
		null !== event.dataTransfer && Array.from(event.dataTransfer.types).includes('Files');
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import Button from '../atoms/Button.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'children'> {
		accept?: string[];
		maxBytes?: number;
		label: string;
		hint: string;
		onfiles: (files: File[], folders: boolean) => void;
		children?: Snippet;
	}

	let { accept = [], maxBytes, label, hint, onfiles, children, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const hintId = `${uid}-hint`;
	const acceptAttribute = $derived(accept.join(','));

	let zone = $state<HTMLElement | undefined>(undefined);
	let fileInput = $state<HTMLInputElement | undefined>(undefined);
	let folderInput = $state<HTMLInputElement | undefined>(undefined);
	let dragging = $state(false);
	let feedback = $state<Feedback | null>(null);

	function reasonFor(file: File): string | null {
		if (0 < accept.length && !accept.some((pattern) => matchesPattern(file, pattern))) {
			return t('u082molecules.drop.reason_type');
		}
		if (undefined !== maxBytes && maxBytes < file.size) {
			return t('u082molecules.drop.reason_size', { max: formatBytes(maxBytes).text });
		}
		return null;
	}

	function ingest(files: File[], folders: boolean): void {
		const accepted: File[] = [];
		let rejectedCount = 0;
		let reason: string | null = null;
		for (const file of files) {
			const failure = reasonFor(file);
			if (null === failure) {
				accepted.push(file);
			} else {
				rejectedCount += 1;
				reason = null === reason ? failure : reason;
			}
		}
		if (0 < accepted.length) {
			onfiles(accepted, folders);
		}
		if (null !== reason) {
			feedback = { tone: 'reject', message: t('u082molecules.drop.rejected', { count: rejectedCount, reason }) };
		} else if (0 < accepted.length) {
			feedback = { tone: 'accept', message: t('u082molecules.drop.ready', { count: accepted.length }) };
		}
	}

	function collect(items: DataTransferItemList): { files: File[]; folders: boolean } {
		const files: File[] = [];
		let folders = false;
		for (const item of Array.from(items)) {
			const entry = item.webkitGetAsEntry();
			if (null !== entry && entry.isDirectory) {
				folders = true;
			} else {
				const file = item.getAsFile();
				if (null !== file) {
					files.push(file);
				}
			}
		}
		return { files, folders };
	}

	function handleDragEnter(event: DragEvent): void {
		if (!hasFiles(event)) {
			return;
		}
		event.preventDefault();
		dragging = true;
	}

	function handleDragLeave(event: DragEvent): void {
		const next = event.relatedTarget;
		if (!(next instanceof Node) || undefined === zone || !zone.contains(next)) {
			dragging = false;
		}
	}

	function handleDrop(event: DragEvent): void {
		event.preventDefault();
		dragging = false;
		if (null === event.dataTransfer) {
			return;
		}
		const { files, folders } = collect(event.dataTransfer.items);
		ingest(files, folders);
	}

	function handlePaste(event: ClipboardEvent): void {
		if (null === event.clipboardData) {
			return;
		}
		const { files, folders } = collect(event.clipboardData.items);
		if (0 === files.length) {
			return;
		}
		event.preventDefault();
		ingest(files, folders);
	}

	function handlePicked(event: Event, folders: boolean): void {
		const input = event.currentTarget;
		if (!(input instanceof HTMLInputElement)) {
			return;
		}
		const picked = null === input.files ? [] : Array.from(input.files);
		input.value = '';
		ingest(picked, folders);
	}
</script>

<section
	bind:this={zone}
	class={['drop-zone', className]}
	data-state={dragging ? 'dragging' : (feedback?.tone ?? 'idle')}
	aria-label={label}
	aria-describedby={hintId}
	ondragenter={handleDragEnter}
	ondragover={handleDragEnter}
	ondragleave={handleDragLeave}
	ondrop={handleDrop}
	onpaste={handlePaste}
	{...rest}
>
	<Stack gap="3" align="center">
		<p class="title">{dragging ? t('u082molecules.drop.dragging') : label}</p>
		<HelpText id={hintId}>{hint}</HelpText>
		<Cluster gap="2" justify="center" wrap>
			<Button variant="secondary" iconStart="upload" onclick={() => fileInput?.click()}>
				{t('u082molecules.drop.choose_files')}
			</Button>
			<Button variant="secondary" iconStart="folder" onclick={() => folderInput?.click()}>
				{t('u082molecules.drop.choose_folder')}
			</Button>
		</Cluster>
		{#if children}{@render children()}{/if}
		<p class="feedback" role="status">{feedback?.message ?? ''}</p>
	</Stack>
	<input
		bind:this={fileInput}
		type="file"
		multiple
		accept={acceptAttribute}
		hidden
		onchange={(event) => handlePicked(event, false)}
	/>
	<input bind:this={folderInput} type="file" webkitdirectory hidden onchange={(event) => handlePicked(event, true)} />
</section>

<style>
	.drop-zone {
		display: block;
		min-inline-size: 0;
		padding: var(--dropzone-pad, var(--space-6));
		border: var(--hairline) dashed var(--dropzone-border, var(--border-control));
		border-radius: var(--dropzone-radius, var(--radius-lg));
		background: var(--dropzone-bg, var(--surface-sunken));
		color: var(--text);
	}

	.drop-zone[data-state='dragging'] {
		border-color: var(--dropzone-border-active, var(--accent));
		background: var(--dropzone-bg-active, var(--accent-soft));
	}

	.drop-zone[data-state='accept'] {
		border-color: var(--ok-border);
	}

	.drop-zone[data-state='reject'] {
		border-color: var(--danger-border);
	}

	.title {
		margin: 0;
	}

	.feedback {
		margin: 0;
		color: var(--text-muted);
	}

	.drop-zone[data-state='accept'] .feedback {
		color: var(--ok-text);
	}

	.drop-zone[data-state='reject'] .feedback {
		color: var(--danger-text);
	}
</style>
