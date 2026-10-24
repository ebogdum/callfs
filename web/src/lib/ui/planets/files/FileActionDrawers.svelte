<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { cluster } from '$lib/api/admin/cluster';
	import { toUiError } from '$lib/api/errors';
	import { listDir } from '$lib/api/files';
	import { filePath } from '$lib/api/http';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { capabilities } from '$lib/stores/capabilities.svelte';
	import { notifications } from '$lib/stores/notifications.svelte';
	import { preferences } from '$lib/stores/preferences.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { FileEntry, Result } from '$lib/api/types';
	import type { UiError } from '$lib/ui/types';
	import Button from '../../atoms/Button.svelte';
	import HelpText from '../../atoms/HelpText.svelte';
	import Combobox from '../../molecules/Combobox.svelte';
	import ConfirmInline from '../../molecules/ConfirmInline.svelte';
	import OctalModeEditor from '../../molecules/OctalModeEditor.svelte';
	import RangeInput from '../../molecules/RangeInput.svelte';
	import ApiEquivalent from '../../organisms/ApiEquivalent.svelte';
	import ConflictResolver from '../../organisms/ConflictResolver.svelte';
	import Dialog from '../../organisms/Dialog.svelte';
	import Drawer from '../../organisms/Drawer.svelte';
	import DestinationPicker from '../../organisms/DestinationPicker.svelte';
	import PermissionInspector from '../../organisms/PermissionInspector.svelte';
	import ShareLinkForm from '../../organisms/ShareLinkForm.svelte';
	import Cluster from '../../quarks/Cluster.svelte';
	import Stack from '../../quarks/Stack.svelte';
	import { requiresTypedDelete, type DrawerKind, type FileActions } from './file-actions.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		actions: FileActions;
		class?: string;
	}

	let { actions, class: className, ...rest }: Props = $props();

	const kind = $derived(actions.drawer?.kind ?? null);
	const entries = $derived<FileEntry[]>(actions.drawer?.entries ?? []);
	const first = $derived<FileEntry | undefined>(entries[0]);
	const count = $derived(entries.length);
	const allFiles = $derived(entries.every((entry) => 'file' === entry.type));
	const isOpen = (target: DrawerKind): boolean => target === kind;
	const setOpen = (target: DrawerKind) => (next: boolean) => {
		if (!next && target === kind) {
			actions.closeDrawer();
		}
	};

	const loadFolders = async (path: string): Promise<Result<FileEntry[], UiError>> => {
		const outcome = await listDir(path);
		return outcome.ok
			? { ok: true, value: outcome.value.data.items }
			: { ok: false, error: toUiError(outcome.error, { action: 'browse.action.list_folders', object: path }) };
	};

	let serverChoices = $state<{ instance_id: string }[]>([]);

	const loadServers = async (): Promise<void> => {
		const outcome = await cluster();
		if (!outcome.ok) {
			notifications.add({
				tone: 'danger',
				category: 'error',
				title: outcome.error.title,
				body: outcome.error.body,
				requestId: outcome.error.requestId ?? null
			});
			return;
		}
		serverChoices = outcome.value.members.flatMap((member) =>
			null !== member.instance_id && !member.is_self && member.online ? [{ instance_id: member.instance_id }] : []
		);
	};

	$effect(() => {
		if ('move' === kind && session.isAdmin) {
			void loadServers();
		}
	});

	const planSentence = $derived(
		null === actions.deletePlan
			? t('fileactions.delete.plan_loading')
			: t('fileactions.delete.plan', {
					files: actions.deletePlan.files,
					directories: actions.deletePlan.directories,
					size: formatBytes(actions.deletePlan.bytes).text
				})
	);
	const typedWord = $derived(
		null !== actions.deletePlan && requiresTypedDelete(actions.deletePlan) ? t('fileactions.delete.word') : undefined
	);
	const deleteTitle = $derived(
		1 === count && undefined !== first
			? t('fileactions.delete.title', { name: first.name })
			: t('fileactions.delete.title_many', { count })
	);
	const moveTitle = $derived(
		1 === count && undefined !== first
			? t('fileactions.move.title', { name: first.name })
			: t('fileactions.move.title_many', { count })
	);
	const rangeTitle = $derived(t('fileactions.range.title', { name: first?.name ?? '' }));
	const totalBytes = $derived(entries.reduce((total, entry) => total + entry.size, 0));
</script>

<div class={['file-action-drawers', className]} {...rest}>
	<Drawer
		bind:open={() => isOpen('move'), setOpen('move')}
		title={moveTitle}
		width="form"
	>
		<Stack gap="4">
			{#if null !== capabilities.data}
				<DestinationPicker
					start="/"
					bind:value={actions.forms.move.destination}
					recents={preferences.value.files.recent}
					pinned={preferences.value.files.pinned}
					load={loadFolders}
					storages={['localfs', 's3']}
					servers={serverChoices}
					bind:options={actions.forms.move.options}
					capabilities={capabilities.data}
					{count}
					bytes={totalBytes}
					folder={!allFiles}
				/>
			{/if}
			<HelpText id="file-action-move-cross">{t('fileactions.move.cross_note')}</HelpText>
			<Cluster gap="2" justify="end">
				<Button variant="ghost" onclick={() => actions.closeDrawer()}>{t('common.cancel')}</Button>
				<Button variant="primary" loading={actions.busy} onclick={() => void actions.submitMove()}>
					{t('fileactions.move.confirm')}
				</Button>
			</Cluster>
		</Stack>
	</Drawer>

	<Drawer bind:open={() => isOpen('share'), setOpen('share')} title={t('fileactions.share.title')} width="form">
		<ShareLinkForm
			path={first?.path ?? ''}
			size={first?.size ?? null}
			bind:value={actions.forms.seconds}
			result={actions.shareLink}
			busy={actions.busy}
			ongenerate={() => void actions.submitShare()}
		/>
	</Drawer>

	<Drawer
		bind:open={() => isOpen('mode'), setOpen('mode')}
		title={t('fileactions.mode.title', { name: first?.name ?? '' })}
		width="form"
	>
		<Stack gap="4">
			<OctalModeEditor bind:value={actions.forms.mode} label={t('fileactions.mode.label')} />
			{#if undefined !== first}
				<ApiEquivalent request={{ method: 'PATCH', url: `/v1/files/${filePath(first.path)}`, body: { mode: actions.forms.mode } }} />
			{/if}
			<Cluster gap="2" justify="end">
				<Button variant="ghost" onclick={() => actions.closeDrawer()}>{t('common.cancel')}</Button>
				<Button variant="primary" loading={actions.busy} onclick={() => void actions.submitMode()}>
					{t('fileactions.mode.confirm')}
				</Button>
			</Cluster>
		</Stack>
	</Drawer>

	<Drawer
		bind:open={() => isOpen('owner'), setOpen('owner')}
		title={t('fileactions.owner.title', { name: first?.name ?? '' })}
		width="form"
	>
		<Stack gap="4">
			<Combobox bind:value={actions.forms.owner} items={actions.ownerItems} label={t('fileactions.owner.label')} />
			<HelpText id="file-action-owner-help">{t('fileactions.owner.help')}</HelpText>
			<Cluster gap="2" justify="end">
				<Button variant="ghost" onclick={() => actions.closeDrawer()}>{t('common.cancel')}</Button>
				<Button variant="primary" loading={actions.busy} onclick={() => void actions.submitOwner()}>
					{t('fileactions.owner.confirm')}
				</Button>
			</Cluster>
		</Stack>
	</Drawer>

	<Drawer
		bind:open={() => isOpen('permissions'), setOpen('permissions')}
		title={t('fileactions.permissions.title', { name: first?.name ?? '' })}
	>
		{#if null !== session.me && undefined !== first}
			<PermissionInspector path={first.path} checks={actions.permissionChecks} me={session.me} />
		{:else}
			<HelpText id="file-action-permissions-loading">{t('fileactions.permissions.loading')}</HelpText>
		{/if}
	</Drawer>

	<Drawer bind:open={() => isOpen('delete'), setOpen('delete')} title={deleteTitle} width="form">
		<ConfirmInline
			question={deleteTitle}
			consequence={allFiles ? t('fileactions.delete.body') : planSentence}
			confirmLabel={allFiles
				? t('fileactions.delete.confirm_files', { count })
				: t('fileactions.delete.confirm_items', { count })}
			typedWord={typedWord}
			tone="danger"
			busy={actions.busy}
			onconfirm={() => void actions.submitDelete()}
			oncancel={() => actions.closeDrawer()}
		/>
	</Drawer>

	<Dialog bind:open={() => isOpen('range'), setOpen('range')} title={rangeTitle} size="sm">
		{#if undefined !== first}
			<RangeInput
				size={first.size}
				bind:from={actions.forms.range.from}
				bind:to={actions.forms.range.to}
				label={t('fileactions.range.label')}
			/>
			<HelpText id="file-action-range-native">{t('fileactions.range.native')}</HelpText>
		{/if}
		{#snippet footer()}
			<Button variant="ghost" onclick={() => actions.closeDrawer()}>{t('common.cancel')}</Button>
			<Button variant="primary" loading={actions.busy} onclick={() => void actions.submitRange()}>
				{t('fileactions.range.confirm')}
			</Button>
		{/snippet}
	</Dialog>

	<Drawer
		bind:open={() => isOpen('conflicts'), setOpen('conflicts')}
		title={t('fileactions.conflicts.title', { count: actions.conflicts.length })}
		width="wide"
	>
		<Stack gap="4">
			<ConflictResolver
				conflicts={actions.conflicts}
				bind:policy={actions.forms.conflicts.policy}
				bind:overrides={actions.forms.conflicts.overrides}
			/>
			<Cluster gap="2" justify="end">
				<Button variant="ghost" onclick={() => actions.closeDrawer()}>{t('fileactions.conflicts.cancel')}</Button>
				<Button variant="primary" onclick={() => actions.confirmUpload()}>
					{t('fileactions.conflicts.confirm', { count: actions.pendingCount })}
				</Button>
			</Cluster>
		</Stack>
	</Drawer>
</div>
