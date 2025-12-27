<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { getLocale } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';
	import { fetchCommands, saveCommand, deleteCommand } from '$lib/services/commands';
	import type { CommandAccessRole, CommandRecord } from '$lib/types/command';

	let commands = $state<CommandRecord[]>([]);
	let loadingList = $state(false);
	let listError = $state<string | null>(null);

	let name = $state('');
	let response = $state('');
	let aliasesText = $state('');
	let selectedPlatforms = $state<string[]>([]);
	let selectedPermissions = $state<CommandAccessRole[]>([]);
	let editingName = $state<string | null>(null);
	let editingSource = $state<'builtin' | 'custom' | null>(null);
	let editingIsEditable = $state(true);
	let metaDescription = $state('');
	let metaUsage = $state('');
const isBuiltinSelection = $derived(editingSource === 'builtin');
const canEditSelection = $derived(!isBuiltinSelection && editingIsEditable);
	let formError = $state<string | null>(null);
	let formStatus = $state<string | null>(null);
	let saving = $state(false);
	let deleting = $state(false);

	const permissionOrder: CommandAccessRole[] = [
		'everyone',
		'followers',
		'subscribers',
		'moderators',
		'vips',
		'owner'
	];

	const platformOptions = ['twitch', 'kick'];

	onMount(() => {
		void loadCommands();
	});

	const sortCommands = (items: CommandRecord[]) =>
		[...items].sort((a, b) => {
			const aBuiltin = (a.source ?? 'custom') === 'builtin';
			const bBuiltin = (b.source ?? 'custom') === 'builtin';
			if (aBuiltin !== bBuiltin) {
				return aBuiltin ? -1 : 1;
			}
			return a.name.localeCompare(b.name);
		});

	const loadCommands = async () => {
		loadingList = true;
		listError = null;
		try {
			const items = await fetchCommands();
			const sorted = sortCommands(items);
			commands = sorted;
			if (editingName) {
				const existing = sorted.find((item) => item.name === editingName);
				if (existing) {
					startEditCommand(existing);
				}
			}
		} catch (error) {
			console.error('commands: list failed', error);
			listError = m.commands_list_error();
		} finally {
			loadingList = false;
		}
	};

	const startNewCommand = () => {
		editingName = null;
		editingSource = null;
		editingIsEditable = true;
		name = '';
		response = '';
		aliasesText = '';
		selectedPlatforms = [];
		selectedPermissions = [];
		metaDescription = '';
		metaUsage = '';
		formError = null;
		formStatus = null;
	};

	const startEditCommand = (command: CommandRecord) => {
		editingName = command.name;
		editingSource = (command.source ?? 'custom') as 'builtin' | 'custom';
		editingIsEditable = command.editable ?? true;
		name = command.name;
		response = command.response ?? '';
		aliasesText = command.aliases?.join(', ') ?? '';
		selectedPlatforms = [...(command.platforms ?? [])];
		selectedPermissions = [...(command.permissions ?? [])];
		metaDescription = command.description ?? '';
		metaUsage = command.usage ?? '';
		formError = null;
		formStatus = null;
	};

	const togglePlatform = (platform: string) => {
		if (selectedPlatforms.includes(platform)) {
			selectedPlatforms = selectedPlatforms.filter((entry) => entry !== platform);
		} else {
			selectedPlatforms = [...selectedPlatforms, platform];
		}
	};

	const togglePermission = (role: CommandAccessRole) => {
		if (selectedPermissions.includes(role)) {
			selectedPermissions = selectedPermissions.filter((entry) => entry !== role);
		} else {
			selectedPermissions = [...selectedPermissions, role];
		}
	};

	const parseAliases = (value: string) =>
		value
			.split(',')
			.map((entry) => entry.trim())
			.filter(Boolean);

	const upsertLocalCommand = (entry: CommandRecord) => {
		const index = commands.findIndex((item) => item.name === entry.name);
		const updated = [...commands];
		if (index === -1) {
			updated.push(entry);
		} else {
			updated[index] = entry;
		}
		commands = sortCommands(updated);
	};

	const formatUpdatedAt = (timestamp?: string) => {
		if (!timestamp) return '';
		const date = new Date(timestamp);
		if (Number.isNaN(date.getTime())) return '';
		try {
			return new Intl.DateTimeFormat(getLocale(), {
				dateStyle: 'short',
				timeStyle: 'short'
			}).format(date);
		} catch {
			return date.toISOString();
		}
	};

	const permissionLabel = (role: CommandAccessRole) => {
		switch (role) {
			case 'followers':
				return m.commands_permission_followers();
			case 'subscribers':
				return m.commands_permission_subscribers();
			case 'moderators':
				return m.commands_permission_moderators();
			case 'vips':
				return m.commands_permission_vips();
			case 'owner':
				return m.commands_permission_owner();
			case 'everyone':
			default:
				return m.commands_permission_everyone();
		}
	};

	const platformLabel = (platform: string) => {
		const normalized = platform?.toLowerCase();
		if (normalized === 'twitch') return m.commands_platform_twitch();
		if (normalized === 'kick') return m.commands_platform_kick();
		return normalized;
	};

const listPermissions = (command: CommandRecord) => {
	const roles = command.permissions ?? [];
	if (roles.length === 0) {
		return [{ id: 'everyone', label: permissionLabel('everyone') }];
	}
	return roles.map((role) => ({
		id: role,
		label: permissionLabel(role)
	}));
};

	const handleSubmit = async (event: SubmitEvent) => {
		event.preventDefault();
		if (editingSource === 'builtin' || !editingIsEditable) {
			return;
		}
		formError = null;
		formStatus = null;
		const trimmedName = name.trim();
		const trimmedResponse = response.trim();
		if (!trimmedName) {
			formError = m.commands_form_error_required_name();
			return;
		}
		if (!trimmedResponse) {
			formError = m.commands_form_error_required_response();
			return;
		}
		const payloadName = editingName ?? trimmedName;
		const payload = {
			name: payloadName,
			response: trimmedResponse,
			aliases: parseAliases(aliasesText),
			platforms: [...selectedPlatforms],
			permissions: [...selectedPermissions]
		};
		saving = true;
		try {
			const saved = await saveCommand(payload);
			upsertLocalCommand(saved);
			startEditCommand(saved);
			formStatus = editingName ? m.commands_form_status_updated() : m.commands_form_status_created();
		} catch (error) {
			console.error('commands: save failed', error);
			formError = m.commands_form_error_save();
		} finally {
			saving = false;
		}
	};

	const handleDelete = async () => {
		if (!editingName) return;
		if (editingSource === 'builtin') return;
		formError = null;
		formStatus = null;
		if (browser) {
			const confirmed = window.confirm(
				m.commands_form_delete_confirm({
					name: editingName
				})
			);
			if (!confirmed) return;
		}
		deleting = true;
		try {
			await deleteCommand(editingName);
			commands = sortCommands(commands.filter((command) => command.name !== editingName));
			startNewCommand();
			formStatus = m.commands_form_status_deleted();
		} catch (error) {
			console.error('commands: delete failed', error);
			formError = m.commands_form_error_delete();
		} finally {
			deleting = false;
		}
	};
</script>

<section class="flex flex-col rounded-3xl border border-slate-200/70 bg-white/95 p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900/70">
	<header class="flex flex-wrap items-center gap-3 border-b border-slate-200/60 pb-4 text-slate-900 dark:border-slate-800/70 dark:text-slate-100">
		<div>
			<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">{m.commands_title()}</p>
			<p class="text-sm text-slate-500 dark:text-slate-400">{m.commands_description()}</p>
		</div>
		<div class="ml-auto flex items-center gap-2">
			<button
				type="button"
				class="rounded-full border border-slate-300 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
				onclick={() => loadCommands()}
				disabled={loadingList}
			>
				{loadingList ? m.commands_refresh_loading() : m.general_refresh()}
			</button>
			<button
				type="button"
				class="rounded-full border border-slate-300 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
				onclick={startNewCommand}
			>
				{m.commands_form_reset()}
			</button>
		</div>
	</header>

	{#if listError}
		<p class="mt-3 rounded-xl bg-rose-500/10 px-3 py-2 text-xs text-rose-600 dark:text-rose-300" aria-live="polite">
			{listError}
		</p>
	{/if}

	<div class="mt-5 grid gap-6 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)]">
		<section class="rounded-2xl border border-slate-200/70 bg-white/90 p-4 dark:border-slate-800 dark:bg-slate-900/70">
			<div class="flex flex-wrap items-center gap-2">
				<div>
					<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
						{m.commands_list_title()}
					</p>
					<p class="text-xs text-slate-500 dark:text-slate-400">
						{m.commands_list_count({ count: commands.length })}
					</p>
				</div>
			</div>

			{#if loadingList}
				<p class="mt-6 text-sm text-slate-500 dark:text-slate-400">{m.commands_list_loading()}</p>
			{:else if commands.length === 0}
				<p class="mt-6 rounded-2xl border border-dashed border-slate-300/70 px-4 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
					{m.commands_list_empty()}
				</p>
			{:else}
				<ul class="mt-4 space-y-3">
					{#each commands as command (command.name)}
						<li>
							<button
								type="button"
								class={`w-full rounded-2xl border px-4 py-3 text-left transition ${
									editingName === command.name
										? 'border-slate-900 bg-slate-900/5 dark:border-white/80 dark:bg-white/5'
										: 'border-slate-200/70 bg-white dark:border-slate-800 dark:bg-slate-900'
								}`}
								onclick={() => startEditCommand(command)}
							>
								<div class="flex flex-wrap items-center gap-2">
									<p class="text-sm font-semibold text-slate-900 dark:text-slate-100">{command.name}</p>
									{#if command.source === 'builtin'}
										<span class="rounded-full bg-slate-900/5 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-slate-600 dark:bg-white/10 dark:text-slate-200">
											{m.commands_tag_builtin()}
										</span>
									{/if}
									{#if command.updated_at}
										<span class="text-xs text-slate-500 dark:text-slate-400">
											{m.commands_list_updated_at({ time: formatUpdatedAt(command.updated_at) })}
										</span>
									{/if}
								</div>
								{#if command.response || command.description}
									<p class="mt-1 line-clamp-2 text-sm text-slate-600 dark:text-slate-300">
										{command.response || command.description}
									</p>
								{/if}
								{#if command.aliases?.length}
									<p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
										{m.commands_aliases_preview({ aliases: command.aliases.join(', ') })}
									</p>
								{/if}
								<div class="mt-3 flex flex-wrap gap-2">
									{#if command.platforms?.length}
										{#each command.platforms as platform}
											<span class="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700 dark:bg-slate-800 dark:text-slate-200">
												{platformLabel(platform)}
											</span>
										{/each}
									{:else}
										<span class="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700 dark:bg-slate-800 dark:text-slate-200">
											{m.commands_list_platform_any()}
										</span>
									{/if}
									{#each listPermissions(command) as entry (entry.id)}
										<span class="rounded-full bg-slate-900/5 px-3 py-1 text-xs font-semibold text-slate-700 dark:bg-white/10 dark:text-slate-200">
											{entry.label}
										</span>
									{/each}
								</div>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<form
			class="flex flex-col gap-4 rounded-2xl border border-slate-200/70 bg-white/90 p-4 text-slate-900 shadow-sm dark:border-slate-800 dark:bg-slate-900/70 dark:text-slate-100"
			onsubmit={handleSubmit}
		>
			<div>
				<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{editingName ? m.commands_form_heading_edit({ name: editingName }) : m.commands_form_heading_new()}
				</p>
				{#if editingName}
					<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_name_edit_hint()}</p>
				{/if}
			</div>
			{#if metaDescription || metaUsage}
				<div class="rounded-xl border border-slate-200/70 bg-slate-50 px-3 py-2 text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-800/70 dark:text-slate-300">
					{#if metaDescription}
						<p>
							<span class="font-semibold">{m.commands_details_description_label()}:</span>
							{metaDescription}
						</p>
					{/if}
					{#if metaUsage}
						<p class="mt-1">
							<span class="font-semibold">{m.commands_details_usage_label()}:</span>
							{metaUsage}
						</p>
					{/if}
				</div>
			{/if}
			{#if !canEditSelection}
				<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_readonly_notice()}</p>
			{/if}
			<label class="flex flex-col gap-1 text-sm">
				<span class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.commands_form_name_label()}
				</span>
				<input
					type="text"
					class="rounded-xl border border-slate-200/80 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-slate-400 focus:outline-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
					placeholder={m.commands_form_name_placeholder()}
					bind:value={name}
					disabled={Boolean(editingName)}
				/>
			</label>
			<label class="flex flex-col gap-1 text-sm">
				<span class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.commands_form_response_label()}
				</span>
				<textarea
					rows="4"
					class="rounded-xl border border-slate-200/80 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-slate-400 focus:outline-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
					placeholder={m.commands_form_response_placeholder()}
					bind:value={response}
					disabled={!canEditSelection}
				></textarea>
			</label>
			<label class="flex flex-col gap-1 text-sm">
				<span class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.commands_form_aliases_label()}
				</span>
				<input
					type="text"
					class="rounded-xl border border-slate-200/80 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-slate-400 focus:outline-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
					placeholder={m.commands_form_aliases_placeholder()}
					bind:value={aliasesText}
					disabled={!canEditSelection}
				/>
				<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_aliases_hint()}</p>
			</label>
			<div class="flex flex-col gap-2">
				<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.commands_form_platforms_label()}
				</p>
				<div class="flex flex-wrap gap-2">
					{#each platformOptions as platform}
						<label class="inline-flex items-center gap-2 rounded-full border border-slate-200/70 px-3 py-1 text-xs font-semibold text-slate-600 dark:border-slate-700 dark:text-slate-200">
							<input
								type="checkbox"
								class="rounded border-slate-300 text-slate-900 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900"
								checked={selectedPlatforms.includes(platform)}
								onchange={() => togglePlatform(platform)}
								disabled={!canEditSelection}
							/>
							{platformLabel(platform)}
						</label>
					{/each}
				</div>
				<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_platforms_hint()}</p>
			</div>
			<div class="flex flex-col gap-2">
				<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.commands_form_permissions_label()}
				</p>
				<div class="flex flex-wrap gap-2">
					{#each permissionOrder as role}
						<label class="inline-flex items-center gap-2 rounded-full border border-slate-200/70 px-3 py-1 text-xs font-semibold text-slate-600 dark:border-slate-700 dark:text-slate-200">
							<input
								type="checkbox"
								class="rounded border-slate-300 text-slate-900 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900"
								checked={selectedPermissions.includes(role)}
								onchange={() => togglePermission(role)}
								disabled={!canEditSelection}
							/>
							{permissionLabel(role)}
						</label>
					{/each}
				</div>
				<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_permissions_hint()}</p>
				<p class="text-xs text-slate-500 dark:text-slate-400">{m.commands_form_permissions_followers_hint()}</p>
			</div>

			{#if formError}
				<p class="text-xs text-rose-500 dark:text-rose-300" aria-live="polite">{formError}</p>
			{/if}
			{#if formStatus}
				<p class="text-xs text-emerald-600 dark:text-emerald-300" aria-live="polite">{formStatus}</p>
			{/if}

			<div class="mt-2 flex flex-wrap gap-3">
				<button
					type="submit"
					class="rounded-full bg-slate-900 px-5 py-2 text-sm font-semibold text-white transition hover:bg-slate-800 disabled:opacity-60 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-100"
					disabled={saving || !canEditSelection}
				>
					{saving ? m.commands_form_saving() : m.commands_form_save()}
				</button>
				{#if editingName && canEditSelection}
					<button
						type="button"
						class="rounded-full border border-rose-200 px-4 py-2 text-sm font-semibold text-rose-600 transition hover:bg-rose-50 disabled:opacity-60 dark:border-rose-500/60 dark:text-rose-300 dark:hover:bg-rose-900/20"
						onclick={handleDelete}
						disabled={deleting}
					>
						{deleting ? m.commands_form_deleting() : m.commands_form_delete()}
					</button>
				{/if}
			</div>
		</form>
	</div>
</section>
