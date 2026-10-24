<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import { COMMANDS_CONTEXT } from '../planets/cluster/MembersPlanet.svelte';
	import Button from '../atoms/Button.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';
	import PathInput from '../molecules/PathInput.svelte';
	import Dialog from '../organisms/Dialog.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	let { children, class: className, ...rest }: Props = $props();

	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	let newLinkOpen = $state(false);
	let filePath = $state('');
	let pathProblem = $state(false);

	function openNewLink(): void {
		filePath = '';
		pathProblem = false;
		newLinkOpen = true;
	}

	function continueToFile(): void {
		const path = filePath.trim();
		if ('' === path || '/' === path.at(-1)) {
			pathProblem = true;
			return;
		}
		pathProblem = false;
		newLinkOpen = false;
		const absolute = '/' === path.at(0) ? path : `/${path}`;
		void goto(`${base}/files${absolute.split('/').map(encodeURIComponent).join('/')}`);
	}

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'sharing.link.new',
				title: t('systems-b.sharing.command.title'),
				group: 'Sharing',
				keywords: t('systems-b.sharing.command.keywords').split(', '),
				icon: 'link',
				run: openNewLink
			}
		]);
	});
</script>

<div class={['sharing-system', className]} {...rest}>
	{@render children()}
</div>

<Dialog
	bind:open={newLinkOpen}
	title={t('systems-b.sharing.dialog.title')}
	description={t('systems-b.sharing.dialog.description')}
	size="sm"
>
	<Stack gap="4">
		{#if true === pathProblem}
			<InlineAlert
				tone="danger"
				title={t('systems-b.sharing.dialog.error.title')}
				body={t('systems-b.sharing.dialog.error.body')}
			/>
		{/if}
		<PathInput bind:value={filePath} label={t('systems-b.sharing.dialog.path')} onsubmit={continueToFile} />
	</Stack>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="secondary" onclick={() => (newLinkOpen = false)}>
				{t('systems-b.sharing.dialog.cancel')}
			</Button>
			<Button variant="primary" onclick={continueToFile}>
				{t('systems-b.sharing.dialog.continue')}
			</Button>
		</Cluster>
	{/snippet}
</Dialog>

<style>
	.sharing-system {
		display: grid;
		gap: var(--space-6);
		min-inline-size: 0;
	}
</style>
