<script lang="ts" module>
	export const TOKENS = ['--member-card-facts-gap'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ClusterMember } from '$lib/api/types';
	import type { StatusKey } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';
	import CopyButton from '../atoms/CopyButton.svelte';
	import Duration from '../atoms/Duration.svelte';
	import Heading from '../atoms/Heading.svelte';
	import Num from '../atoms/Num.svelte';
	import StatusPill from '../atoms/StatusPill.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import Surface from '../quarks/Surface.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		member: ClusterMember;
		class?: string;
	}

	let { member, class: className, ...rest }: Props = $props();

	const MEMBER_STATUS: Record<ClusterMember['state'], StatusKey> = {
		leader: 'member.leader',
		follower: 'member.follower',
		offline: 'member.unreachable'
	};

	const status = $derived(MEMBER_STATUS[member.state]);
	const isLeader = $derived('leader' === member.state);
	const roleWord = $derived(isLeader ? t('status.member.leader') : t('topology.member.role_member'));
	const onlineWord = $derived(member.online ? t('members.online.yes') : t('members.online.no'));
</script>

<Surface
	level="raised"
	pad="card"
	radius="lg"
	class={['member-card', className]}
	{...rest}
>
	<Stack gap="4">
		<Cluster gap="2">
			<Heading level={3}>{member.node_id}</Heading>
			<StatusPill {status} />
		</Cluster>
		<dl class="facts">
			<div>
				<dt>{t('topology.member.role')}</dt>
				<dd>{roleWord}</dd>
			</div>
			<div>
				<dt>{t('topology.member.online')}</dt>
				<dd>{onlineWord}</dd>
			</div>
			<div>
				<dt>{t('topology.member.last_seen')}</dt>
				<dd>
					{#if null === member.last_seen_ms}
						{t('common.unknown')}
					{:else}
						<Duration value={member.last_seen_ms} />
					{/if}
				</dd>
			</div>
			<div>
				<dt>{t('topology.member.behind')}</dt>
				<dd>
					{#if null === member.behind_by}
						{t('common.unknown')}
					{:else}
						<Num value={member.behind_by} />
					{/if}
				</dd>
			</div>
			<div>
				<dt>{t('topology.member.server_address')}</dt>
				<dd>
					{#if null === member.api_endpoint}
						{t('common.unknown')}
					{:else}
						<Cluster gap="2">
							<Text mono>{member.api_endpoint}</Text>
							<CopyButton value={member.api_endpoint} variant="button" />
						</Cluster>
					{/if}
				</dd>
			</div>
		</dl>
	</Stack>
</Surface>

<style>
	.facts {
		display: grid;
		gap: var(--member-card-facts-gap, var(--space-3));
		margin: 0;
	}

	.facts > div {
		display: grid;
		gap: var(--space-1-5);
	}

	dt {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
	}

	dd {
		margin: 0;
	}
</style>
