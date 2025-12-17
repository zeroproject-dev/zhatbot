<script lang="ts">
	import { tick } from 'svelte';
	import {
		createChatStream,
		sendChatCommand,
		type ChatStreamState
	} from '$lib/services/chat-stream';
	import type { ChatMessage } from '$lib/types/chat';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';

	const chatStream = createChatStream();
	let state: ChatStreamState = { messages: [], status: 'connecting' };
	let scrollContainer: HTMLElement | null = null;
	let draft = '';
	let sending = false;
	let sendError: string | null = null;

	$: state = $chatStream;
	$: if (state.messages.length > 0) {
		scrollToBottom();
	}

	const scrollToBottom = async () => {
		await tick();
		if (scrollContainer) {
			scrollContainer.scrollTop = scrollContainer.scrollHeight;
		}
	};

	const handleSubmit = async (event: SubmitEvent) => {
		event.preventDefault();
		sendError = null;
		const text = draft.trim();
		if (!text) {
			sendError = m.chat_send_error();
			return;
		}
		sending = true;
		try {
			await sendChatCommand({ text });
			draft = '';
		} catch (error) {
			console.error('No se pudo enviar el mensaje', error);
			sendError = m.chat_send_error();
		} finally {
			sending = false;
		}
	};

	const badgeClasses: Record<ChatStreamState['status'], string> = {
		connecting: 'bg-amber-500/20 text-amber-100',
		connected: 'bg-emerald-500/20 text-emerald-100',
		disconnected: 'bg-rose-500/20 text-rose-100'
	};

	type RoleKey = 'owner' | 'admin' | 'mod' | 'vip' | 'private';

	const getRoleKeys = (message: ChatMessage): RoleKey[] => {
		const roles: RoleKey[] = [];
		if (message.is_platform_owner) roles.push('owner');
		if (message.is_platform_admin) roles.push('admin');
		if (message.is_platform_mod) roles.push('mod');
		if (message.is_platform_vip) roles.push('vip');
		if (message.is_private) roles.push('private');
		return roles;
	};

	const formatPlatform = (platform: string) => platform?.toUpperCase();

	const platformColor = (platform: string) => {
		const normalized = platform?.toLowerCase();
		if (normalized === 'twitch') return '#9146FF';
		if (normalized === 'kick') return '#53FC18';
		return '#475569';
	};

	const formatTimestamp = (timestamp?: string) => {
		if (!timestamp) return m.chat_timestamp_now();
		const date = new Date(timestamp);
		if (Number.isNaN(date.getTime())) return m.chat_timestamp_now();
		const locale = getLocale();
		return date.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' });
	};

	const statusLabel = (status: ChatStreamState['status']) => {
		if (status === 'connected') return m.chat_status_connected();
		if (status === 'disconnected') return m.chat_status_disconnected();
		return m.chat_status_connecting();
	};

	const roleLabel = (role: RoleKey) => {
		switch (role) {
			case 'owner':
				return m.chat_role_owner();
			case 'admin':
				return m.chat_role_admin();
			case 'mod':
				return m.chat_role_mod();
			case 'vip':
				return m.chat_role_vip();
			case 'private':
				return m.chat_role_private();
			default:
				return role;
		}
	};
</script>

<section
	class="flex max-h-[80vh] w-full flex-col rounded-3xl bg-slate-900/80 p-5 text-slate-100 shadow-inner shadow-black/40 md:max-h-full!"
>
	<header class="flex items-center justify-between border-b border-white/10 pb-3">
		<!-- <h2 class="text-sm font-semibold tracking-wide text-white/70 uppercase">{m.chat_title()}</h2> -->
		<span
			class={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-semibold ${badgeClasses[state.status]}`}
		>
			<span class="h-1.5 w-1.5 rounded-full bg-current opacity-80"></span>
			{statusLabel(state.status)}
		</span>
	</header>

	<form class="mt-4 flex gap-3" onsubmit={handleSubmit}>
		<label class="sr-only" for="chat-input">{m.chat_input_placeholder()}</label>
		<input
			id="chat-input"
			class="flex-1 rounded-2xl border border-white/20 bg-white/5 px-4 py-2 text-sm text-white placeholder:text-white/40 focus:border-white/60 focus:outline-none"
			placeholder={m.chat_input_placeholder()}
			bind:value={draft}
			autocomplete="off"
		/>
		<button
			type="submit"
			class="rounded-2xl bg-white/10 px-5 py-2 text-sm font-semibold text-white transition hover:bg-white/20 disabled:opacity-50"
			disabled={sending || state.status !== 'connected'}
		>
			{state.status !== 'connected' ? m.chat_send_disabled() : m.chat_send_button()}
		</button>
	</form>
	{#if sendError}
		<p class="mt-2 text-xs text-rose-200">{sendError}</p>
	{/if}

	<div class="min-h-0 flex-1 pt-4">
		{#if state.messages.length === 0}
			<div
				class="flex h-full flex-col items-center justify-center gap-2 text-center text-sm text-white/60"
			>
				<span class="text-4xl">💬</span>
				<p>{m.chat_waiting_title()}</p>
				<p class="text-xs text-white/40">{m.chat_waiting_hint()}</p>
			</div>
		{:else}
			<div class="h-full pr-2" bind:this={scrollContainer}>
				<ul
					class="chat-scroll flex h-full flex-col-reverse gap-4 overflow-y-auto pr-2"
					aria-live="polite"
				>
					{#each state.messages as message (message.received_at ?? message.user_id + message.text)}
						<li class="flex gap-3 text-sm">
							<span
								class="w-1 self-stretch rounded-full"
								style={`background:${platformColor(message.platform)}`}
								aria-hidden="true"
							></span>
							<div class="flex-1 space-y-1">
								<div class="flex flex-wrap items-center gap-2">
									<span class="font-semibold text-white">{message.username}</span>
									<!-- <span class="text-xs tracking-wide text-white/50 uppercase"> -->
									<!-- 	{formatPlatform(message.platform)} · {m.chat_channel_label({ -->
									<!-- 		channel: message.channel_id -->
									<!-- 	})} -->
									<!-- </span> -->
									<time class="ml-auto text-[11px] text-white/40"
										>{formatTimestamp(message.received_at)}</time
									>
								</div>
								<p class="text-base leading-relaxed text-white/90">{message.text}</p>
								{#if getRoleKeys(message).length}
									<div class="flex flex-wrap gap-2">
										{#each getRoleKeys(message) as role}
											<span
												class="rounded-full bg-white/10 px-2 py-0.5 text-[10px] font-semibold tracking-wide text-white/80 uppercase"
											>
												{roleLabel(role)}
											</span>
										{/each}
									</div>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
</section>

<style>
	:global(.chat-scroll) {
		scrollbar-width: thin;
		scrollbar-color: rgba(148, 163, 184, 0.9) transparent;
	}

	:global(.chat-scroll::-webkit-scrollbar) {
		width: 6px;
	}

	:global(.chat-scroll::-webkit-scrollbar-track) {
		background: transparent;
	}

	:global(.chat-scroll::-webkit-scrollbar-thumb) {
		background: rgba(148, 163, 184, 0.8);
		border-radius: 999px;
	}

	:global(.chat-scroll::-webkit-scrollbar-thumb:hover) {
		background: rgba(241, 245, 249, 0.9);
	}
</style>
