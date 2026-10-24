<script lang="ts">
	import type { Me } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import type { HTMLAttributes } from 'svelte/elements';
	import AuthLayout from '../environments/AuthLayout.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';
	import LoginPlanet from '../planets/auth/LoginPlanet.svelte';
	import LiveRegions from './LiveRegions.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		expired?: boolean;
		onsignedin?: (me: Me) => void;
		class?: string;
	}

	let { expired = false, onsignedin, class: className, ...rest }: Props = $props();
</script>

<div class={['auth-galaxy', className]} {...rest}>
	<AuthLayout>
		{#if expired}
			<InlineAlert tone="warn" title={t('galaxy.auth.expired.title')} body={t('galaxy.auth.expired.body')} />
		{/if}
		<LoginPlanet {onsignedin} />
	</AuthLayout>
	<LiveRegions />
</div>
