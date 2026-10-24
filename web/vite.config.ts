import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', 'CALLFS_');
	const apiTarget = env.CALLFS_API || 'http://127.0.0.1:8443';

	const apiProxy = {
		target: apiTarget,
		ws: true,
		changeOrigin: false,
		secure: false
	};

	return {
		plugins: [sveltekit()],
		server: {
			host: '127.0.0.1',
			port: 5173,
			proxy: {
				'/v1': apiProxy,
				'/health': apiProxy,
				'/metrics': apiProxy,
				'/download': apiProxy
			}
		}
	};
});
