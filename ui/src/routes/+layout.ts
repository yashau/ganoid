// Disable SSR — Ganoid is a pure client-side SPA served as static files.
// This ensures the fallback index.html is an empty shell, letting SvelteKit's
// client-side router handle all routes including direct navigation and reload.
export const ssr = false;
export const prerender = false;
