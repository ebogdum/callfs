<script lang="ts" module>
	export type ServerRole = 'leader' | 'follower' | 'candidate' | 'shutdown' | 'none';

	export interface ServerChoice {
		id: string;
		endpoint: string;
		self: boolean;
		up: boolean;
		role: ServerRole;
		latencyMs: number | null;
	}

	export const TOKENS = [
		'--server-picker-bg',
		'--server-picker-fg',
		'--server-picker-border',
		'--server-picker-remote-bg',
		'--server-picker-remote-fg',
		'--server-picker-remote-border'
	] as const;
</script>

<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import Dot from '../quarks/Dot.svelte';

	const MENU_SIDE_OFFSET_PX = 4;
	const MENU_COLLISION_PADDING_PX = 8;

	interface Props {
		servers: ServerChoice[];
		active: string;
		remote: boolean;
		compact?: boolean;
		onselect: (id: string) => void;
		class?: string;
	}

	let { servers, active, remote, compact = false, onselect, class: className }: Props = $props();

	const current = $derived(servers.find((server) => active === server.id) ?? null);

	function detailOf(server: ServerChoice): string {
		const role = t(`nodes.role.${server.role}`);
		if (false === server.up) {
			return `${role} · ${t('nodes.picker.unreachable')}`;
		}
		return null === server.latencyMs ? role : `${role} · ${t('nodes.picker.latency', { ms: server.latencyMs })}`;
	}

	function choose(id: string): void {
		if (active !== id) {
			onselect(id);
		}
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<button
				type="button"
				class={['server-trigger', className]}
				data-remote={remote ? 'true' : undefined}
				data-compact={compact ? 'true' : undefined}
				aria-label={t('nodes.picker.trigger', { node: active })}
				{...props}
			>
				<Dot tone={null === current || current.up ? 'ok' : 'danger'} />
				<span class="trigger-icon" aria-hidden="true"><Icon name="server" size="sm" /></span>
				<span class="trigger-name">{active}</span>
				{#if null !== current && 'leader' === current.role && false === compact}
					<span class="trigger-role" aria-hidden="true"><Icon name="leader" size="xs" /></span>
				{/if}
				<span class="trigger-chevron" aria-hidden="true"><Icon name="chevrons-up-down" size="sm" /></span>
			</button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			class="server-menu"
			aria-label={t('nodes.picker.label')}
			align="end"
			sideOffset={MENU_SIDE_OFFSET_PX}
			collisionPadding={MENU_COLLISION_PADDING_PX}
		>
			<DropdownMenu.RadioGroup value={active} onValueChange={choose}>
				<DropdownMenu.GroupHeading class="server-menu-heading">{t('nodes.picker.heading')}</DropdownMenu.GroupHeading>
				{#each servers as server (server.id)}
					<DropdownMenu.RadioItem
						class="server-menu-item"
						value={server.id}
						textValue={server.id}
						disabled={false === server.up && active !== server.id}
					>
						{#snippet children({ checked })}
							<Dot tone={server.up ? 'ok' : 'danger'} />
							<span class="item-text">
								<span class="item-title">
									<span class="item-name">{server.id}</span>
									{#if server.self}
										<span class="item-self">{t('nodes.picker.this_server')}</span>
									{/if}
								</span>
								<span class="item-detail">{detailOf(server)}</span>
							</span>
							<span class="item-check" aria-hidden="true">
								{#if checked}
									<Icon name="check" size="sm" />
								{/if}
							</span>
						{/snippet}
					</DropdownMenu.RadioItem>
				{/each}
			</DropdownMenu.RadioGroup>
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>

<style>
	.server-trigger {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1-5);
		flex: none;
		min-inline-size: 0;
		max-inline-size: calc(var(--space-16) * 3);
		block-size: var(--control-h);
		padding-inline: var(--space-2);
		border: var(--hairline) solid var(--server-picker-border, var(--border-subtle));
		border-radius: var(--radius-md);
		background: var(--server-picker-bg, var(--surface));
		color: var(--server-picker-fg, var(--text));
		font: inherit;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		cursor: pointer;
		box-shadow: var(--shadow-xs);
		transition:
			border-color var(--dur-micro) var(--ease-out),
			background-color var(--dur-micro) var(--ease-out);
	}

	.server-trigger[data-remote='true'] {
		border-color: var(--server-picker-remote-border, var(--warn-border));
		background: var(--server-picker-remote-bg, var(--warn-soft));
		color: var(--server-picker-remote-fg, var(--warn-text));
	}

	@media (hover: hover) {
		.server-trigger:hover {
			border-color: var(--border);
			background: var(--surface-hover);
		}

		.server-trigger[data-remote='true']:hover {
			border-color: var(--warn);
			background: var(--server-picker-remote-bg, var(--warn-soft));
		}
	}

	.server-trigger:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.trigger-icon,
	.trigger-role,
	.trigger-chevron {
		display: inline-flex;
		flex: none;
	}

	.trigger-icon,
	.trigger-chevron {
		color: inherit;
		opacity: 0.7;
	}

	.trigger-role {
		color: var(--accent-text);
	}

	.server-trigger[data-remote='true'] .trigger-role {
		color: inherit;
	}

	.trigger-name {
		min-inline-size: 0;
		overflow: hidden;
		font-family: var(--font-mono);
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.server-trigger[data-compact='true'] .trigger-icon {
		display: none;
	}

	:global(.server-menu) {
		z-index: var(--z-dropdown);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: calc(var(--space-16) * 3.5);
		max-inline-size: min(calc(var(--space-16) * 5), calc(var(--viewport-w) - var(--space-4)));
		max-block-size: min(calc(var(--space-16) * 6), calc(var(--viewport-h) - var(--space-8)));
		overflow-y: auto;
		padding: var(--space-1);
		color: var(--text);
		background: var(--surface-overlay);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
	}

	:global(.server-menu-heading) {
		padding: var(--space-1-5) var(--space-2) var(--space-1);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: 600;
		color: var(--text-muted);
	}

	:global(.server-menu-item) {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		box-sizing: border-box;
		min-block-size: var(--row-h-2line);
		padding-block: var(--space-1-5);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		color: inherit;
		cursor: default;
		user-select: none;
		outline: var(--focus-width) solid transparent;
	}

	:global(.server-menu-item[data-highlighted]) {
		background: var(--surface-hover);
	}

	:global(.server-menu-item[data-state='checked']) {
		background: var(--surface-selected);
	}

	:global(.server-menu-item:focus-visible) {
		box-shadow: var(--focus-ring);
	}

	:global(.server-menu-item[data-disabled]) {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.item-text {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.item-title {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.item-name {
		min-inline-size: 0;
		overflow: hidden;
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.item-self {
		flex: none;
		padding-inline: var(--space-1-5);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		color: var(--accent-text);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: 500;
	}

	.item-detail {
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		color: var(--text-muted);
	}

	.item-check {
		display: inline-flex;
		flex: none;
		inline-size: var(--space-4);
		color: var(--accent-text);
	}
</style>
