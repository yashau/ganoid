<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Profile, type ProfileStore } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Table from '$lib/components/ui/table';
	import * as Dialog from '$lib/components/ui/dialog';

	let store = $state<ProfileStore | null>(null);
	let error = $state('');
	let successMsg = $state('');

	// Add/edit dialog
	let dialogOpen = $state(false);
	let dialogMode = $state<'add' | 'edit'>('add');
	let formId = $state('');
	let formName = $state('');
	let formServer = $state('');
	let formError = $state('');
	let formBusy = $state(false);

	// Delete dialog
	let deleteTarget = $state<Profile | null>(null);
	let deleteOpen = $state(false);
	let deleteBusy = $state(false);

	async function load() {
		try {
			store = await api.profiles();
			error = '';
		} catch (e) {
			error = String(e);
		}
	}

	function openAdd() {
		formId = ''; formName = ''; formServer = ''; formError = '';
		dialogMode = 'add';
		dialogOpen = true;
	}

	function openEdit(p: Profile) {
		formId = p.id; formName = p.name; formServer = p.login_server; formError = '';
		dialogMode = 'edit';
		dialogOpen = true;
	}

	async function submitForm() {
		formError = '';
		if (!formId || !formName) { formError = 'ID and name are required.'; return; }
		formBusy = true;
		try {
			if (dialogMode === 'add') {
				await api.createProfile(formId, formName, formServer);
				successMsg = `Profile "${formName}" created.`;
			} else {
				await api.updateProfile(formId, formName, formServer);
				successMsg = `Profile "${formName}" updated.`;
			}
			dialogOpen = false;
			await load();
		} catch (e) {
			formError = String(e);
		} finally {
			formBusy = false;
		}
	}

	async function doDelete() {
		if (!deleteTarget) return;
		deleteBusy = true;
		try {
			await api.deleteProfile(deleteTarget.id);
			successMsg = `Profile "${deleteTarget.name}" deleted.`;
			deleteOpen = false;
			await load();
		} catch (e) {
			error = String(e);
		} finally {
			deleteBusy = false;
		}
	}

	onMount(load);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-semibold tracking-tight">Profiles</h1>
		<Button onclick={openAdd}>Add Profile</Button>
	</div>

	{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
	{#if successMsg}<p class="text-sm text-green-600 dark:text-green-400">{successMsg}</p>{/if}

	{#if store}
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head>Name</Table.Head>
					<Table.Head>ID</Table.Head>
					<Table.Head>Login Server</Table.Head>
					<Table.Head>Last Used</Table.Head>
					<Table.Head class="w-[120px]"></Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each store.profiles as p}
					<Table.Row>
						<Table.Cell class="font-medium">
							<div class="flex items-center gap-2">
								{p.name}
								{#if p.id === store.active_profile_id}
									<Badge variant="outline">active</Badge>
								{/if}
							</div>
						</Table.Cell>
						<Table.Cell class="text-muted-foreground">{p.id}</Table.Cell>
						<Table.Cell class="max-w-[200px] truncate text-muted-foreground">
							{p.login_server || 'controlplane.tailscale.com'}
						</Table.Cell>
						<Table.Cell class="text-muted-foreground">
							{new Date(p.last_used).toLocaleDateString()}
						</Table.Cell>
						<Table.Cell>
							<div class="flex gap-2">
								<Button variant="ghost" size="sm" onclick={() => openEdit(p)}>Edit</Button>
								<Button
									variant="ghost"
									size="sm"
									class="text-destructive hover:text-destructive"
									disabled={p.id === store.active_profile_id}
									onclick={() => { deleteTarget = p; deleteOpen = true; }}
								>Delete</Button>
							</div>
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	{:else if !error}
		<p class="text-sm text-muted-foreground">Loading…</p>
	{/if}
</div>

<!-- Add / Edit dialog -->
<Dialog.Root bind:open={dialogOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{dialogMode === 'add' ? 'Add Profile' : 'Edit Profile'}</Dialog.Title>
			<Dialog.Description>
				{dialogMode === 'add'
					? 'Create a new Tailscale coordination server profile.'
					: 'Update the profile name or login server.'}
			</Dialog.Description>
		</Dialog.Header>

		{#if formError}
			<p class="text-sm text-destructive">{formError}</p>
		{/if}

		<div class="space-y-4">
			<div class="space-y-1.5">
				<Label for="fid">ID <span class="text-muted-foreground">(slug, no spaces)</span></Label>
				<Input id="fid" bind:value={formId} disabled={dialogMode === 'edit'} placeholder="headscale-home" />
			</div>
			<div class="space-y-1.5">
				<Label for="fname">Name</Label>
				<Input id="fname" bind:value={formName} placeholder="Home Headscale" />
			</div>
			<div class="space-y-1.5">
				<Label for="fserver">
					Login Server <span class="text-muted-foreground">(blank = official Tailscale)</span>
				</Label>
				<Input id="fserver" bind:value={formServer} placeholder="https://headscale.example.com" />
			</div>
		</div>

		<Dialog.Footer>
			<Button variant="outline" onclick={() => dialogOpen = false}>Cancel</Button>
			<Button disabled={formBusy} onclick={submitForm}>
				{formBusy ? 'Saving…' : 'Save'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<!-- Delete confirmation dialog -->
<Dialog.Root bind:open={deleteOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Delete Profile</Dialog.Title>
			<Dialog.Description>
				Are you sure you want to delete <strong>{deleteTarget?.name}</strong>?
				Saved Tailscale state for this profile will not be removed.
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => deleteOpen = false}>Cancel</Button>
			<Button variant="destructive" disabled={deleteBusy} onclick={doDelete}>
				{deleteBusy ? 'Deleting…' : 'Delete'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
