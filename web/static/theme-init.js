(function () {
	var root = document.documentElement;
	var prefs = {};
	try {
		var stored = JSON.parse(window.localStorage.getItem('callfs.ui.prefs.v1'));
		if (stored && 'object' === typeof stored) {
			prefs = stored;
		}
	} catch (error) {
		prefs = {};
	}

	function pick(value, allowed, fallback) {
		return -1 !== allowed.indexOf(value) ? value : fallback;
	}

	function setOrRemove(name, value, neutral) {
		if (neutral === value) {
			root.removeAttribute(name);
		} else {
			root.setAttribute(name, value);
		}
	}

	function matches(query) {
		return 'function' === typeof window.matchMedia && true === window.matchMedia(query).matches;
	}

	var theme = pick(prefs.theme, ['system', 'light', 'dark'], 'system');
	var contrast = pick(prefs.contrast, ['standard', 'more'], 'standard');
	var density = pick(prefs.density, ['compact', 'default', 'comfortable'], 'default');
	var motion = pick(prefs.motion, ['system', 'reduced', 'none'], 'system');
	var radius = pick(prefs.radius, ['medium', 'sharp', 'round'], 'medium');
	var fontScale = pick(prefs.fontScale, [0.875, 1, 1.125, 1.25], 1);

	setOrRemove('data-theme', theme, 'system');
	setOrRemove('data-contrast', contrast, 'standard');
	setOrRemove('data-motion', motion, 'system');
	setOrRemove('data-radius', radius, 'medium');
	root.setAttribute('data-density', density);
	root.style.setProperty('--font-scale', String(fontScale));

	var dark = 'dark' === theme || ('system' === theme && matches('(prefers-color-scheme: dark)'));
	var highContrast = 'more' === contrast || matches('(prefers-contrast: more)');
	var background = '#f5f9f9';
	if (dark) {
		background = highContrast ? '#000000' : '#090d0d';
	} else if (highContrast) {
		background = '#ffffff';
	}

	var meta = document.querySelector('meta[name="theme-color"]');
	if (null !== meta) {
		meta.setAttribute('content', background);
	}
})();
