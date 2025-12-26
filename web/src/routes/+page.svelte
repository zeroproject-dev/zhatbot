<script lang="ts">
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import AuthActions from '$lib/components/AuthActions.svelte';
	import StreamInfoPanel from '$lib/components/StreamInfoPanel.svelte';
	import GiveawayPanel from '$lib/components/GiveawayPanel.svelte';
	import NotificationPanel from '$lib/components/NotificationPanel.svelte';
	import { m } from '$lib/paraglide/messages.js';

	type TabId = 'stream' | 'giveaway';

	const tabs: Array<{ id: TabId; label: string }> = [
		{ id: 'stream', label: m.dashboard_tab_stream() },
		{ id: 'giveaway', label: m.dashboard_tab_giveaway() }
	];

	let activeTab: TabId = 'stream';
</script>

<div class="flex flex-col gap-6">
	<div
		role="tablist"
		aria-label={m.dashboard_tabs_aria_label()}
		class="inline-flex w-full flex-wrap gap-3 rounded-2xl border border-slate-200/70 bg-white/80 p-1 text-sm font-semibold text-slate-500 shadow-sm dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-300"
	>
		{#each tabs as tab}
			<button
				type="button"
				role="tab"
				class={`flex-1 rounded-xl px-4 py-2 transition ${
					activeTab === tab.id
						? 'bg-slate-900 text-white shadow dark:bg-white dark:text-slate-900'
						: 'hover:text-slate-900 dark:hover:text-white'
				}`}
				aria-selected={activeTab === tab.id}
				onclick={() => {
					activeTab = tab.id;
				}}
			>
				{tab.label}
			</button>
		{/each}
	</div>

	{#if activeTab === 'stream'}
		<div class="flex flex-col gap-6 lg:grid lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
			<section class="flex flex-1">
				<ChatPanel />
			</section>
			<section class="flex flex-1">
				<div class="flex w-full flex-col gap-6">
					<AuthActions />
					<StreamInfoPanel />
					<NotificationPanel />
				</div>
			</section>
		</div>
	{:else}
		<div class="flex flex-col gap-6 lg:grid lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
			<section class="flex flex-1">
				<ChatPanel />
			</section>
			<section class="flex flex-1">
				<div class="flex w-full flex-col gap-6">
					<NotificationPanel />
					<GiveawayPanel />
				</div>
			</section>
		</div>
	{/if}
</div>
