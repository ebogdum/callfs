import adapter from '@sveltejs/adapter-static';

const config = {
	compilerOptions: {
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		adapter: adapter({
			fallback: 'index.html',
			pages: 'build',
			assets: 'build',
			precompress: true,
			strict: true
		}),
		paths: {
			base: '/ui',
			relative: false
		},
		appDir: '_app',
		output: {
			bundleStrategy: 'split'
		}
	}
};

export default config;
