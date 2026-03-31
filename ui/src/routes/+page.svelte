<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type StatusResponse, type SwitchEvent } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as Card from '$lib/components/ui/card';
	import { Separator } from '$lib/components/ui/separator';

	let status = $state<StatusResponse | null>(null);
	let profiles = $state<{ id: string; name: string; login_server: string }[]>([]);
	let activeId = $state('');
	let error = $state('');
	let switching = $state('');
	let switchLog = $state<SwitchEvent[]>([]);
	let cancelSwitch: (() => void) | null = null;
	let interval: ReturnType<typeof setInterval>;
	let fastPollInterval: ReturnType<typeof setInterval> | null = null;

	function startFastPoll() {
		if (fastPollInterval) clearInterval(fastPollInterval);
		fastPollInterval = setInterval(async () => {
			await load();
			if (status && status.tailscale.backend_state !== 'NoState') {
				clearInterval(fastPollInterval!);
				fastPollInterval = null;
			}
		}, 2000);
		// Stop fast-polling after 30 seconds regardless.
		setTimeout(() => {
			if (fastPollInterval) { clearInterval(fastPollInterval); fastPollInterval = null; }
		}, 30000);
	}

	async function load() {
		try {
			const [s, p] = await Promise.all([api.status(), api.profiles()]);
			status = s;
			profiles = p.profiles;
			activeId = p.active_profile_id;
			error = '';
		} catch (e) {
			error = String(e);
		}
	}

	function doSwitch(id: string) {
		if (switching) return;
		switching = id;
		switchLog = [];
		cancelSwitch = api.switchProfile(
			id,
			(ev) => { switchLog = [...switchLog, ev]; },
			() => { switching = ''; cancelSwitch = null; load(); startFastPoll(); },
			(msg) => { switching = ''; cancelSwitch = null; error = msg; }
		);
	}

	function stateVariant(state: string): 'default' | 'secondary' | 'destructive' | 'outline' {
		if (state === 'Running') return 'default';
		if (state === 'NeedsLogin') return 'secondary';
		return 'destructive';
	}

	onMount(() => { load(); interval = setInterval(load, 15000); });
	onDestroy(() => { clearInterval(interval); if (fastPollInterval) clearInterval(fastPollInterval); cancelSwitch?.(); });
</script>

<div class="space-y-6">
	<h1 class="text-2xl font-semibold tracking-tight">Dashboard</h1>

	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if status}
		<!-- Status card -->
		<Card.Root>
			<Card.Header>
				<Card.Title>Status</Card.Title>
				<Card.Description>Current Tailscale connection</Card.Description>
			</Card.Header>
			<Card.Content>
				<div class="grid grid-cols-3 gap-6">
					<div class="space-y-1">
						<p class="text-xs text-muted-foreground uppercase tracking-wide">Active Profile</p>
						<p class="font-medium">{status.active_profile.name}</p>
						<p class="text-xs text-muted-foreground truncate">
							{status.active_profile.login_server || 'controlplane.tailscale.com'}
						</p>
					</div>
					<div class="space-y-1">
						<p class="text-xs text-muted-foreground uppercase tracking-wide">Tailscale</p>
						<Badge variant={stateVariant(status.tailscale.backend_state)}>
							{status.tailscale.backend_state}
						</Badge>
						{#if status.tailscale.backend_state === 'NeedsLogin'}
							<p class="text-xs text-muted-foreground">
								Open the Tailscale app or run <code class="font-mono">tailscale login</code> to authenticate with this profile's server.
							</p>
						{:else if status.tailscale.backend_state === 'Stopped'}
							<p class="text-xs text-muted-foreground">The Tailscale daemon is not running.</p>
						{/if}
					</div>
					<div class="space-y-1">
						<p class="text-xs text-muted-foreground uppercase tracking-wide">Peers</p>
						<p class="font-medium">{status.tailscale.peer_count}</p>
					</div>
				</div>
			</Card.Content>
		</Card.Root>

		<Separator />

		<!-- Profile switcher -->
		<div class="space-y-3">
			<h2 class="text-sm font-medium text-muted-foreground uppercase tracking-wide">Switch Profile</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each profiles as p}
					<Card.Root class={p.id === activeId ? 'border-primary' : ''}>
						<Card.Header class="pb-2">
							<div class="flex items-start justify-between gap-2">
								<Card.Title class="text-base">{p.name}</Card.Title>
								{#if p.id === activeId}
									<Badge variant="outline" class="shrink-0">Active</Badge>
								{/if}
							</div>
							<Card.Description class="truncate text-xs">
								{p.login_server || 'controlplane.tailscale.com'}
							</Card.Description>
						</Card.Header>
						{#if p.id !== activeId}
							<Card.Footer class="pt-0">
								<Button
									size="sm"
									class="w-full"
									disabled={!!switching}
									onclick={() => doSwitch(p.id)}
								>
									{switching === p.id ? 'Switching…' : 'Switch'}
								</Button>
							</Card.Footer>
						{/if}
					</Card.Root>
				{/each}
			</div>
		</div>

		<!-- Switch progress log -->
		{#if switchLog.length > 0}
			<Card.Root>
				<Card.Content class="pt-4">
					<div class="space-y-1 font-mono text-xs">
						{#each switchLog as ev}
							<p class={ev.error ? 'text-destructive' : 'text-muted-foreground'}>
								[{ev.step}/{ev.total}] {ev.error ? '✗ ' + ev.error : ev.message}
							</p>
						{/each}
					</div>
				</Card.Content>
			</Card.Root>
		{/if}
	{:else if !error}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{/if}
</div>
