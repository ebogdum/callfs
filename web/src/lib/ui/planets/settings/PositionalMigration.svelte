<script lang="ts">
	import type { PositionalMapping } from '$lib/api/admin/users';
	import type { UserInfo } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Input from '$lib/ui/atoms/Input.svelte';
	import FormField from '$lib/ui/molecules/FormField.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import ApiEquivalent from '$lib/ui/organisms/ApiEquivalent.svelte';
	import Dialog from '$lib/ui/organisms/Dialog.svelte';
	import Cluster from '$lib/ui/quarks/Cluster.svelte';
	import Stack from '$lib/ui/quarks/Stack.svelte';
	import type { UiError } from '$lib/ui/types';

	interface Props {
		open: boolean;
		rows: UserInfo[];
		reservedIds: string[];
		busy: boolean;
		error: UiError | null;
		onsubmit: (mapping: PositionalMapping[]) => void;
	}

	let { open = $bindable(false), rows, reservedIds, busy, error, onsubmit }: Props = $props();

	let targets = $state<Record<string, string>>({});

	const targetOf = (user: UserInfo): string => (targets[user.user_id] ?? '').trim();

	const keyNumber = (user: UserInfo): number => (user.index ?? 0) + 1;

	const reason = $derived.by((): string | undefined => {
		const ids = rows.map(targetOf);
		if (0 < ids.filter((id) => 0 === id.length).length) {
			return t('access.positional.reason.missing');
		}
		if (0 < ids.filter((id) => reservedIds.includes(id)).length) {
			return t('access.positional.reason.reserved');
		}
		if (0 < ids.length - new Set(ids).size) {
			return t('access.positional.reason.duplicate');
		}
		return undefined;
	});

	const mapping = $derived(
		rows.map((user): PositionalMapping => ({ index: user.index ?? 0, user_id: targetOf(user) }))
	);

	const request = $derived({
		method: 'POST' as const,
		url: '/v1/admin/users/migrate-positional',
		body: { mapping, reassign_owner: true }
	});

	const submit = () => {
		if (undefined !== reason) {
			return;
		}
		onsubmit(mapping);
	};
</script>

<Dialog bind:open title={t('access.positional.dialog.title')} description={t('access.positional.body', { count: rows.length })} size="md">
	<Stack gap="4">
		{#if undefined !== reason}
			<InlineAlert tone="warn" title={reason} />
		{/if}
		{#if null !== error}
			<InlineAlert tone="danger" title={error.title} body={error.body} />
		{/if}
		{#each rows as user (user.user_id)}
			<FormField
				label={t('access.positional.field.label', { index: keyNumber(user) })}
				for={`access-positional-${keyNumber(user)}`}
				help={t('access.positional.field.help', { index: keyNumber(user), user: user.user_id })}
			>
				{#snippet control({ id, describedBy, invalid })}
					<Input {id} mono aria-describedby={describedBy} {invalid} bind:value={() => targets[user.user_id] ?? '', (next) => { targets[user.user_id] = next; }} />
				{/snippet}
			</FormField>
		{/each}
		<ApiEquivalent {request} />
	</Stack>
	{#snippet footer()}
		<Cluster gap="2" justify="end">
			<Button variant="ghost" onclick={() => (open = false)}>{t('common.cancel')}</Button>
			<Button variant="primary" disabledReason={reason} loading={busy} onclick={submit}>
				{t('access.positional.submit')}
			</Button>
		</Cluster>
	{/snippet}
</Dialog>
