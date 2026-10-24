export class CrosshairState {
	time = $state<number | null>(null);
	ownerId = $state<string | null>(null);

	place(time: number, ownerId: string): void {
		this.time = time;
		this.ownerId = ownerId;
	}

	clear(ownerId: string): void {
		if (ownerId === this.ownerId) {
			this.time = null;
			this.ownerId = null;
		}
	}
}

const groups = new Map<string, CrosshairState>();

export const crosshairGroup = (syncKey: string): CrosshairState => {
	const existing = groups.get(syncKey);
	if (undefined !== existing) return existing;
	const created = new CrosshairState();
	groups.set(syncKey, created);
	return created;
};
