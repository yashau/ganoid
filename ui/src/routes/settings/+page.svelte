<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Card from '$lib/components/ui/card';
	import { Separator } from '$lib/components/ui/separator';

	let port = $state(57400);
	let saved = $state(false);

	function save() {
		// TODO: POST /api/settings when the endpoint is added
		saved = true;
		setTimeout(() => { saved = false; }, 2000);
	}
</script>

<div class="space-y-6">
	<h1 class="text-2xl font-semibold tracking-tight">Settings</h1>

	<Card.Root class="max-w-lg">
		<Card.Header>
			<Card.Title>General</Card.Title>
			<Card.Description>Configure Ganoid behaviour.</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-6">
			<div class="space-y-1.5">
				<Label for="port">HTTP Port</Label>
				<Input id="port" type="number" min={1024} max={65535} bind:value={port} class="w-32" />
				<p class="text-xs text-muted-foreground">Restart required to take effect.</p>
			</div>

			<Separator />

			<div class="flex items-center justify-between opacity-50">
				<div class="space-y-0.5">
					<Label>Start with system</Label>
					<p class="text-xs text-muted-foreground">Autostart via registry / systemd / launchd — coming soon.</p>
				</div>
				<button
					role="switch"
					aria-label="Start with system"
					aria-checked={false}
					disabled
					class="relative inline-flex h-6 w-11 shrink-0 cursor-not-allowed items-center rounded-full border-2 border-transparent bg-input transition-colors"
				>
					<span class="pointer-events-none block h-5 w-5 translate-x-0 rounded-full bg-background shadow-lg ring-0 transition-transform"></span>
				</button>
			</div>
		</Card.Content>
		<Card.Footer class="flex items-center gap-3">
			<Button onclick={save}>Save</Button>
			{#if saved}
				<p class="text-sm text-green-600 dark:text-green-400">Saved.</p>
			{/if}
		</Card.Footer>
	</Card.Root>
</div>
