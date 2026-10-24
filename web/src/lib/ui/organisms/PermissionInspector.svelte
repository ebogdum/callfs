<script lang="ts" module>
	export type PermissionKind = 'read' | 'write' | 'delete';
	export type PermissionReason = 'owner' | 'admin' | 'open_directory' | 'not_owner' | 'not_found' | 'parent_open';

	export interface PermissionCheck {
		perm: PermissionKind;
		allowed: boolean;
		reason: PermissionReason;
		owner: string;
	}

	export const TOKENS = ['--permission-row-gap', '--permission-row-pad', '--permission-row-bg'] as const;

	const PERM_COPY: Record<PermissionKind, string> = {
		read: 'fileforms.permissions.read',
		write: 'fileforms.permissions.write',
		delete: 'fileforms.permissions.delete'
	};

	const ACTION_COPY: Record<PermissionKind, string> = {
		read: 'fileforms.permissions.action.read',
		write: 'fileforms.permissions.action.write',
		delete: 'fileforms.permissions.action.delete'
	};

	const RULE_COPY: Record<PermissionReason, string> = {
		owner: 'fileforms.permissions.rule.owner',
		admin: 'fileforms.permissions.rule.admin',
		open_directory: 'fileforms.permissions.rule.open_directory',
		not_owner: 'fileforms.permissions.rule.not_owner',
		not_found: 'fileforms.permissions.rule.not_found',
		parent_open: 'fileforms.permissions.rule.parent_open'
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Me } from '$lib/api/types';
	import { t } from '$lib/copy/t';
	import HelpText from '../atoms/HelpText.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		path: string;
		checks: PermissionCheck[];
		me: Me;
		class?: string;
	}

	let { path, checks, me, class: className, ...rest }: Props = $props();

	const signedInId = $props.id();
</script>

<section class={['permission-inspector', className]} aria-label={t('fileforms.permissions.region', { path })} {...rest}>
	<Stack gap="3">
		<HelpText id={signedInId}>{t('fileforms.permissions.signed_in', { user: me.user_id })}</HelpText>
		<ul class="rows">
			{#each checks as check (check.perm)}
				<li class="row" data-allowed={check.allowed}>
					<span class="name">{t(PERM_COPY[check.perm])}</span>
					<span class="verdict">
						<StatusGlyph tone={check.allowed ? 'ok' : 'danger'} size="sm" />
						<span>{check.allowed ? t('fileforms.permissions.allowed') : t('fileforms.permissions.denied')}</span>
					</span>
					<span class="rule">
						{t(RULE_COPY[check.reason], { action: t(ACTION_COPY[check.perm]), owner: check.owner })}
					</span>
				</li>
			{/each}
		</ul>
	</Stack>
</section>

<style>
	.permission-inspector {
		display: grid;
		min-inline-size: 0;
	}

	.rows {
		display: grid;
		gap: var(--permission-row-gap, var(--space-2));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.row {
		display: grid;
		gap: var(--space-1);
		padding: var(--permission-row-pad, var(--space-3));
		background: var(--permission-row-bg, var(--surface-raised));
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
	}

	.name {
		color: var(--text);
		font-weight: 600;
	}

	.verdict {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text);
	}

	.rule {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		overflow-wrap: anywhere;
	}
</style>
