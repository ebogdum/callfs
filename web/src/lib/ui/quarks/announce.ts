export type AnnounceLevel = 'polite' | 'assertive';

const REGION_IDS: Record<AnnounceLevel, string> = {
	polite: 'live-polite',
	assertive: 'live-assertive'
};

export const announce = (text: string, level: AnnounceLevel): void => {
	const region = document.getElementById(REGION_IDS[level]);
	if (null === region) {
		return;
	}
	region.textContent = '';
	window.requestAnimationFrame(() => {
		region.textContent = text;
	});
};
