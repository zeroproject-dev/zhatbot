<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { API_BASE_URL } from '$lib/config';

	type Platform = 'twitch' | 'kick';
	type Role = 'bot' | 'streamer';

	interface CredentialState {
		has_access_token?: boolean;
		has_refresh_token?: boolean;
		updated_at?: string;
		expires_at?: string;
	}

	type CredentialsMap = Record<Platform, Partial<Record<Role, CredentialState>>>;

	const baseUrl = API_BASE_URL ?? 'http://localhost:8080';

	const buttons = [
		{
			platform: 'twitch',
			role: 'bot',
			label: m.auth_button_twitch_bot(),
			accent: 'bg-[#9146FF]/20 text-[#d0b7ff]'
		},
		{
			platform: 'twitch',
			role: 'streamer',
			label: m.auth_button_twitch_streamer(),
			accent: 'bg-[#9146FF]/10 text-white/80'
		},
		{
			platform: 'kick',
			role: 'streamer',
			label: m.auth_button_kick_streamer(),
			accent: 'bg-[#53FC18]/10 text-white/80'
		}
	] satisfies Array<{ platform: Platform; role: Role; label: string; accent: string }>;

	let loadingKey = $state<string | null>(null);
	let feedback = $state<{ type: 'success' | 'error'; message: string } | null>(null);
	let credentials = $state<CredentialsMap>({ twitch: {}, kick: {} });
	let statusLoading = $state(false);
	let statusError = $state<string | null>(null);
	let lastSynced = $state<string | null>(null);

	const login = async (platform: Platform, role: Role) => {
		if (!browser) return;
		feedback = null;
		const key = `${platform}-${role}`;
		loadingKey = key;
		try {
			const response = await fetch(`${baseUrl}/api/oauth/${platform}/start`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ role })
			});

			if (!response.ok) {
				throw new Error(`Request failed with ${response.status}`);
			}

			const data = (await response.json()) as { url?: string };
			if (!data.url) {
				throw new Error('Missing redirect url');
			}

			window.open(data.url, '_blank', 'noopener');
			feedback = { type: 'success', message: m.auth_login_success() };
		} catch (error) {
			console.error('Login flow error', error);
			feedback = { type: 'error', message: m.auth_login_error() };
		} finally {
			loadingKey = null;
		}

		await loadStatus();
	};

	const loadStatus = async () => {
		if (!browser) return;
		statusLoading = true;
		statusError = null;
		try {
			const response = await fetch(`${baseUrl}/api/oauth/status`);
			if (!response.ok) {
				throw new Error(`Status request failed ${response.status}`);
			}

			const data = (await response.json()) as {
				credentials?: Record<string, Record<string, CredentialState>>;
			};

			credentials = {
				twitch: data.credentials?.twitch ?? {},
				kick: data.credentials?.kick ?? {}
			};
			lastSynced = new Date().toISOString();
		} catch (error) {
			console.error('Status fetch failed', error);
			statusError = m.auth_login_error();
		} finally {
			statusLoading = false;
		}
	};

	const getCredential = (platform: Platform, role: Role): CredentialState | undefined =>
		credentials?.[platform]?.[role];

	const isConnected = (platform: Platform, role: Role) =>
		Boolean(getCredential(platform, role)?.has_access_token);

	const formatDate = (iso?: string) => {
		if (!iso) return '';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleString(getLocale(), {
			dateStyle: 'short',
			timeStyle: 'short'
		});
	};

	const lastSyncLabel = () => {
		if (!lastSynced) return '';
		return m.auth_status_last_sync({ date: formatDate(lastSynced) });
	};

	onMount(() => {
		loadStatus();
		const handleFocus = () => loadStatus();
		window.addEventListener('focus', handleFocus);
		return () => window.removeEventListener('focus', handleFocus);
	});
</script>

<section
	class="flex h-full flex-col gap-5 rounded-3xl border border-slate-200/60 bg-white/90 p-6 text-slate-800 shadow-inner shadow-black/5 dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-100"
>
	<header class="space-y-1">
		<p class="text-xs font-semibold tracking-wide text-slate-500 uppercase dark:text-slate-400">
			{m.auth_login_title()}
		</p>
		<p class="text-sm text-slate-500 dark:text-slate-400">{m.auth_login_description()}</p>
		<p class="text-xs text-slate-400 dark:text-slate-500">{m.auth_login_hint()}</p>
		<div class="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
			{#if lastSynced}
				<span>{lastSyncLabel()}</span>
			{:else if statusLoading}
				<span>{m.chat_status_connecting()}</span>
			{/if}
			<button
				class="text-[11px] font-semibold tracking-wide text-indigo-500 uppercase hover:text-indigo-400 dark:text-indigo-300"
				type="button"
				onclick={loadStatus}
				disabled={statusLoading}
			>
				{statusLoading ? '…' : 'refresh'}
			</button>
		</div>
	</header>

	<div class="grid gap-3">
		{#each buttons as btn}
			{@const connected = isConnected(btn.platform, btn.role)}
			<button
				type="button"
				class={`rounded-2xl px-4 py-3 text-left text-sm font-semibold transition hover:bg-white/20 dark:hover:bg-white/10 ${btn.accent} ${connected ? 'opacity-70' : ''}`}
				onclick={() => login(btn.platform, btn.role)}
				disabled={connected || loadingKey !== null}
			>
				<div class="flex items-center justify-between gap-3">
					<div>
						<p>{btn.label}</p>
						<p class={`text-xs ${connected ? 'text-emerald-200' : 'text-white/60'}`}>
							{connected ? m.auth_status_connected() : m.auth_status_not_connected()}
						</p>
						{#if connected}
							<p class="text-[11px] text-white/50">
								{m.auth_status_last_sync({
									date: formatDate(getCredential(btn.platform, btn.role)?.updated_at)
								})}
							</p>
						{/if}
					</div>
					{#if loadingKey === `${btn.platform}-${btn.role}`}
						<span class="text-xs tracking-wide uppercase opacity-70">…</span>
					{/if}
				</div>
			</button>
		{/each}
	</div>

	{#if feedback}
		<p
			class={`text-xs ${feedback.type === 'success' ? 'text-emerald-400' : 'text-rose-300'}`}
			aria-live="assertive"
		>
			{feedback.message}
		</p>
	{/if}

	{#if statusError}
		<p class="text-xs text-rose-300" aria-live="assertive">{statusError}</p>
	{/if}
</section>
