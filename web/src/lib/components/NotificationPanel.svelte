<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { getLocale } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';
	import { notificationsStore } from '$lib/stores/notifications';
	import type { NotificationRecord } from '$lib/types/notification';

	const notifications = $derived($notificationsStore);

	let loading = $state(false);
	let error = $state<string | null>(null);

	const typeStyles: Record<
		NonNullable<NotificationRecord['type']>,
		{ label: string; classes: string }
	> = {
		subscription: {
			label: m.notifications_type_subscription(),
			classes: 'bg-purple-500/15 text-purple-800 dark:text-purple-200'
		},
		donation: {
			label: m.notifications_type_donation(),
			classes: 'bg-rose-500/15 text-rose-800 dark:text-rose-200'
		},
		bits: {
			label: m.notifications_type_bits(),
			classes: 'bg-indigo-500/15 text-indigo-800 dark:text-indigo-200'
		},
		giveaway_winner: {
			label: m.notifications_type_giveaway(),
			classes: 'bg-emerald-500/15 text-emerald-800 dark:text-emerald-200'
		},
		generic: {
			label: m.notifications_type_generic(),
			classes: 'bg-slate-500/15 text-slate-700 dark:text-slate-200'
		}
	};

	const refresh = async () => {
		loading = true;
		error = null;
		try {
			await notificationsStore.refresh(100);
		} catch (err) {
			console.error('notifications: refresh failed', err);
			error = m.notifications_error_loading();
		} finally {
			loading = false;
		}
	};

	onMount(() => {
		void refresh();
		if (!browser) return undefined;
		const interval = window.setInterval(() => {
			void refresh();
		}, 60_000);
		return () => {
			window.clearInterval(interval);
		};
	});

	const formatTimestamp = (timestamp: string) => {
		if (!timestamp) return '';
		const date = new Date(timestamp);
		if (Number.isNaN(date.getTime())) return timestamp;
		return date.toLocaleString(getLocale(), {
			hour: '2-digit',
			minute: '2-digit',
			day: '2-digit',
			month: 'short'
		});
	};

	const entryMessage = (entry: NotificationRecord) => {
		if (entry.message) return entry.message;
		if (entry.type === 'giveaway_winner') {
			const title = entry.metadata?.giveaway_title;
			if (title) {
				return m.notifications_default_giveaway_message({ title });
			}
			return m.notifications_default_giveaway_message_fallback();
		}
		return m.notifications_default_generic_message();
	};

	const typeLabel = (type: NotificationRecord['type']) =>
		(type && typeStyles[type]?.label) || m.notifications_type_generic();

	const typeClass = (type: NotificationRecord['type']) =>
		(type && typeStyles[type]?.classes) || typeStyles.generic.classes;
</script>

<section class="flex flex-col rounded-3xl border border-slate-200/70 bg-white/95 p-5 text-slate-900 shadow-sm dark:border-slate-800 dark:bg-slate-900/70 dark:text-slate-100">
	<header class="flex flex-wrap items-center gap-3 border-b border-slate-200/60 pb-4 dark:border-slate-800/70">
		<div>
			<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
				{m.notifications_title()}
			</p>
			<p class="text-sm text-slate-500 dark:text-slate-400">{m.notifications_description()}</p>
		</div>
		<div class="ml-auto flex items-center gap-2">
			<button
				type="button"
				class="rounded-full border border-slate-300 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
				onclick={() => refresh()}
				disabled={loading}
			>
				{loading ? m.notifications_refresh_loading() : m.notifications_refresh()}
			</button>
		</div>
	</header>

	{#if error}
		<p class="mt-3 rounded-xl bg-rose-500/10 px-3 py-2 text-xs text-rose-600 dark:text-rose-300" aria-live="polite">
			{error}
		</p>
	{/if}

	{#if notifications.length === 0 && loading}
		<p class="mt-6 text-sm text-slate-500 dark:text-slate-400">{m.notifications_loading()}</p>
	{:else if notifications.length === 0}
		<p class="mt-6 rounded-2xl border border-dashed border-slate-200/70 px-4 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
			{m.notifications_empty()}
		</p>
	{:else}
		<ul class="mt-4 space-y-3">
			{#each notifications as entry (entry.id)}
				<li class="rounded-2xl border border-slate-200/70 bg-white/90 p-4 shadow-sm dark:border-slate-800/70 dark:bg-slate-900/70">
					<div class="flex flex-wrap items-center gap-2">
						<span class={`rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ${typeClass(entry.type)}`}>
							{typeLabel(entry.type)}
						</span>
						{#if entry.username}
							<p class="text-sm font-semibold text-slate-800 dark:text-slate-100">{entry.username}</p>
						{/if}
						{#if entry.amount}
							<span class="text-xs text-slate-500 dark:text-slate-400">
								{m.notifications_amount({ amount: entry.amount.toFixed(2) })}
							</span>
						{/if}
						<span class="ml-auto text-xs text-slate-500 dark:text-slate-400">
							{formatTimestamp(entry.created_at)}
						</span>
					</div>
					<p class="mt-2 text-sm text-slate-700 dark:text-slate-200">{entryMessage(entry)}</p>
					{#if entry.type === 'giveaway_winner' && entry.metadata?.giveaway_title}
						<p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
							{m.notifications_label_giveaway_title({ title: entry.metadata?.giveaway_title })}
						</p>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>
