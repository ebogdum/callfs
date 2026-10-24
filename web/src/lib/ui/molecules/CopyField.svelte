<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		label: string;
		value: string;
		mono?: boolean;
		secretOnce?: boolean;
		class?: string;
	}

	let { label, value, mono = false, secretOnce = false, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const fieldId = $derived(`${uid}-field`);
	const warningId = $derived(`${uid}-warning`);
</script>

<Stack gap="1-5" class={['copy-field', className]} {...rest}>
	<Label for={fieldId}>{label}</Label>
	<Cluster gap="2">
		<div class="copy-field-input">
			<Input
				id={fieldId}
				{value}
				{mono}
				readonly
				aria-describedby={secretOnce ? warningId : undefined}
			/>
		</div>
		<CopyButton {value} variant="button" />
	</Cluster>
	{#if secretOnce}
		<Cluster gap="1" id={warningId}>
			<StatusGlyph tone="warn" size="sm" />
			<Text size="sm" tone="warn">{t('facts.secret_once')}</Text>
		</Cluster>
	{/if}
</Stack>

<style>
	.copy-field-input {
		flex: 1 1 auto;
		min-inline-size: 0;
	}
</style>
