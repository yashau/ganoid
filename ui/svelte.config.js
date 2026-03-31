import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			// Output goes to ../../cmd/ganoidd/ui/dist so ganoidd's embed picks it up
			pages: '../../cmd/ganoidd/ui/dist',
			assets: '../../cmd/ganoidd/ui/dist',
			fallback: 'index.html',
			precompress: false,
			strict: false
		})
	}
};

export default config;
