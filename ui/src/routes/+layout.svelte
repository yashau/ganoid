<script lang="ts">
	import '../app.css';
	import { ModeWatcher } from 'mode-watcher';
	import { page } from '$app/stores';
	import { Separator } from '$lib/components/ui/separator';
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();

	const links = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/profiles', label: 'Profiles' },
		{ href: '/settings', label: 'Settings' }
	];
</script>

<!-- ModeWatcher reads system preference and applies .dark/.light on <html>.
     defaultMode="system" means it follows the OS unless the user overrides. -->
<ModeWatcher defaultMode="system" />

<div class="min-h-screen bg-background text-foreground">
	<nav class="border-b border-border bg-card">
		<div class="mx-auto flex max-w-5xl items-center gap-1 px-4 py-3">
			<span class="mr-auto text-base font-semibold tracking-tight">Ganoid</span>
			{#each links as link}
				<a
					href={link.href}
					class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors
						{$page.url.pathname === link.href
						? 'bg-accent text-accent-foreground'
						: 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
				>
					{link.label}
				</a>
			{/each}
		</div>
	</nav>

	<main class="mx-auto max-w-5xl px-4 py-6">
		{@render children()}
	</main>
</div>
