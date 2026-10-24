<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	type OverlayLayer = 'dropdown' | 'scrim' | 'drawer' | 'dialog' | 'palette' | 'popover-top' | 'toast' | 'tooltip';

	const layers: OverlayLayer[] = ['dropdown', 'scrim', 'drawer', 'dialog', 'palette', 'popover-top', 'toast', 'tooltip'];

	let { class: className, ...rest }: HTMLAttributes<HTMLDivElement> = $props();

	function createClaimSlot() {
		let owner: string | null = null;
		return {
			claim(next: string): boolean {
				if (null === owner || next === owner) {
					owner = next;
					return true;
				}
				return false;
			},
			release(next: string): void {
				if (next === owner) {
					owner = null;
				}
			}
		};
	}

	const dialogSlot = createClaimSlot();
	const bannerSlot = createClaimSlot();

	export const claimDialog = dialogSlot.claim;
	export const releaseDialog = dialogSlot.release;
	export const claimBanner = bannerSlot.claim;
	export const releaseBanner = bannerSlot.release;
</script>

<div class={['overlay-host', className]} {...rest}>
	{#each layers as layer (layer)}
		<div id={`overlay-${layer}`} class="layer" data-layer={layer} style:z-index={`var(--z-${layer})`}></div>
	{/each}
</div>

<style>
	.overlay-host {
		display: contents;
	}

	.layer {
		position: fixed;
		inset: 0;
		pointer-events: none;
	}
</style>
