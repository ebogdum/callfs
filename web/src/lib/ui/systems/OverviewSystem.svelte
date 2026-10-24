<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getContext } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { CommandRegistry } from '$lib/commands/registry.svelte';
	import { t } from '$lib/copy/t';
	import OverviewPlanet from '../planets/overview/OverviewPlanet.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		class?: string;
	}

	let { class: className = '', ...rest }: Props = $props();

	const COMMANDS_CONTEXT = 'callfs.commands';
	const registry = getContext<CommandRegistry | undefined>(COMMANDS_CONTEXT);

	$effect(() => {
		if (undefined === registry) {
			return;
		}
		return registry.register([
			{
				id: 'overview.go',
				title: t('systems-a.overview.go'),
				group: 'Navigate',
				keywords: [t('systems-a.overview.go')],
				shortcut: ['g', 'o'],
				icon: 'home',
				run: () => goto(`${base}/`)
			}
		]);
	});
</script>

<div class="overview-system {className}" {...rest}>
	<OverviewPlanet />
</div>
