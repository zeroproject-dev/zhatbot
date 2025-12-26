<script lang="ts">
	import { getSharedChatStream } from '$lib/services/chat-stream';
	import type { ChatMessage } from '$lib/types/chat';
	import { getLocale } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';

	type Phase = 'idle' | 'collecting' | 'closed';

	type Participant = {
		user_id: string;
		username: string;
		platform: string;
		channel_id: string;
		joined_at: string;
	};

	const chatStream = getSharedChatStream();

	let keywordInput = '';
	let activeKeyword = '';
	let phase: Phase = 'idle';
	let participants: Participant[] = [];
	let searchQuery = '';
	let keywordError: string | null = null;
	let winner: Participant | null = null;
	let winnerModalOpen = false;
	let winnerMessages: ChatMessage[] = [];
	let modalError: string | null = null;

	let filteredParticipants: Participant[] = [];
	let canReroll = false;

	const participantIds = new Set<string>();
	const disqualifiedIds = new Set<string>();
	const processedMessageKeys = new Set<string>();
	const processedOrder: string[] = [];
	const winnerProcessedKeys = new Set<string>();
	const winnerProcessedOrder: string[] = [];

	const maxProcessedMessages = 600;
	const maxWinnerMessages = 20;

	const statusBadge: Record<Phase, { label: string; classes: string }> = {
		idle: { label: m.giveaway_status_idle(), classes: 'bg-amber-500/15 text-amber-700 dark:text-amber-200' },
		collecting: {
			label: m.giveaway_status_collecting(),
			classes: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-200'
		},
		closed: { label: m.giveaway_status_closed(), classes: 'bg-slate-500/15 text-slate-700 dark:text-slate-200' }
	};

	const rememberProcessed = (key: string) => {
		processedOrder.push(key);
		processedMessageKeys.add(key);
		const overflow = processedOrder.length - maxProcessedMessages;
		if (overflow > 0) {
			const removed = processedOrder.splice(0, overflow);
			for (const value of removed) {
				processedMessageKeys.delete(value);
			}
		}
	};

	const rememberWinnerMessage = (key: string) => {
		winnerProcessedOrder.push(key);
		winnerProcessedKeys.add(key);
		const overflow = winnerProcessedOrder.length - maxWinnerMessages * 2;
		if (overflow > 0) {
			const removed = winnerProcessedOrder.splice(0, overflow);
			for (const value of removed) {
				winnerProcessedKeys.delete(value);
			}
		}
	};

	const messageKey = (message: ChatMessage) =>
		`${message.user_id}|${message.received_at ?? ''}|${message.text}`;

	const maybeAddParticipant = (message: ChatMessage) => {
		if (phase !== 'collecting') return;
		const keyword = activeKeyword.trim().toLowerCase();
		if (!keyword) return;
		const firstToken = (message.text || '').trim().toLowerCase().split(/\s+/)[0] ?? '';
		if (firstToken !== `!${keyword}`) return;
		if (participantIds.has(message.user_id)) return;
		const joined_at = message.received_at ?? new Date().toISOString();
		const participant: Participant = {
			user_id: message.user_id,
			username: message.username,
			platform: message.platform,
			channel_id: message.channel_id,
			joined_at
		};
		participantIds.add(participant.user_id);
		participants = [...participants, participant];
	};

	const resetWinnerTracking = () => {
		winnerMessages = [];
		winnerProcessedKeys.clear();
		winnerProcessedOrder.length = 0;
		modalError = null;
	};

	const resetGiveaway = () => {
		keywordInput = '';
		activeKeyword = '';
		phase = 'idle';
		participants = [];
		filteredParticipants = [];
		searchQuery = '';
		keywordError = null;
		winner = null;
		winnerModalOpen = false;
		modalError = null;
		participantIds.clear();
		disqualifiedIds.clear();
		resetWinnerTracking();
	};

	const handleKeywordSubmit = (event: SubmitEvent) => {
		event.preventDefault();
		const nextKeyword = keywordInput.trim();
		if (nextKeyword.length === 0) {
			keywordError = m.giveaway_keyword_error_required();
			return;
		}
		activeKeyword = nextKeyword;
		phase = 'collecting';
		participants = [];
		filteredParticipants = [];
		searchQuery = '';
		keywordError = null;
		winner = null;
		winnerModalOpen = false;
		modalError = null;
		participantIds.clear();
		disqualifiedIds.clear();
		resetWinnerTracking();
	};

	const finishGiveaway = () => {
		if (phase !== 'collecting') return;
		const eligible = getEligibleParticipants();
		if (eligible.length === 0) {
			keywordError = m.giveaway_finish_error_no_participants();
			return;
		}
		phase = 'closed';
		selectRandomWinner(eligible);
	};

	const getEligibleParticipants = () =>
		participants.filter((participant) => !disqualifiedIds.has(participant.user_id));

	const selectRandomWinner = (pool = getEligibleParticipants()) => {
		if (pool.length === 0) {
			modalError = m.giveaway_modal_reroll_error();
			winnerModalOpen = false;
			winner = null;
			return;
		}
		const index = Math.floor(Math.random() * pool.length);
		winner = pool[index];
		winnerModalOpen = true;
		resetWinnerTracking();
	};

	const rerollWinner = () => {
		if (!winner) return;
		const pool = participants.filter(
			(participant) => participant.user_id !== winner?.user_id && !disqualifiedIds.has(participant.user_id)
		);
		if (pool.length === 0) {
			modalError = m.giveaway_modal_reroll_error();
			return;
		}
		disqualifiedIds.add(winner.user_id);
		selectRandomWinner(pool);
	};

	const closeModal = () => {
		winnerModalOpen = false;
		modalError = null;
	};

	const formatTimestamp = (timestamp?: string) => {
		if (!timestamp) return '';
		const date = new Date(timestamp);
		if (Number.isNaN(date.getTime())) return '';
		try {
			return date.toLocaleTimeString(getLocale(), { hour: '2-digit', minute: '2-digit' });
		} catch {
			return date.toISOString();
		}
	};

	const phaseInstructions = () => {
		if (phase === 'collecting' && activeKeyword) {
			return m.giveaway_keyword_hint({ keyword: activeKeyword });
		}
		if (phase === 'closed') {
			return m.giveaway_status_closed_hint();
		}
		return m.giveaway_status_idle_hint();
	};

	$: {
		const state = $chatStream;
		for (const message of state.messages) {
			const key = messageKey(message);
			if (!processedMessageKeys.has(key)) {
				rememberProcessed(key);
				maybeAddParticipant(message);
			}
			if (winner && message.user_id === winner.user_id && !winnerProcessedKeys.has(key)) {
				rememberWinnerMessage(key);
				winnerMessages = [message, ...winnerMessages].slice(0, maxWinnerMessages);
			}
		}
	}

	$: {
		const query = searchQuery.trim().toLowerCase();
		if (!query) {
			filteredParticipants = participants;
		} else {
			filteredParticipants = participants.filter((participant) => {
				const haystack = `${participant.username} ${participant.platform} ${participant.channel_id}`.toLowerCase();
				return haystack.includes(query);
			});
		}
	}

	$: canReroll =
		Boolean(winner) &&
		participants.some(
			(participant) => participant.user_id !== winner?.user_id && !disqualifiedIds.has(participant.user_id)
		);

	const isWinner = (participant: Participant) => winner?.user_id === participant.user_id;
	const isDisqualified = (participant: Participant) => disqualifiedIds.has(participant.user_id);
</script>

<section class="flex h-full flex-col rounded-3xl border border-slate-200/70 bg-white/95 p-6 text-slate-900 shadow-sm dark:border-slate-800 dark:bg-slate-900/70 dark:text-slate-100">
	<header class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200/60 pb-4 dark:border-slate-800/70">
		<div>
			<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
				{m.giveaway_title()}
			</p>
			<p class="text-sm text-slate-500 dark:text-slate-400">{m.giveaway_description()}</p>
		</div>
		<span
			class={`inline-flex items-center rounded-full px-3 py-1 text-[11px] font-semibold tracking-wide uppercase ${statusBadge[phase].classes}`}
		>
			{statusBadge[phase].label}
		</span>
	</header>

	<div class="mt-5 space-y-6">
		<form class="rounded-2xl border border-slate-200/70 bg-slate-900 text-white dark:border-slate-700" onsubmit={handleKeywordSubmit}>
			<div class="flex flex-col gap-3 px-4 py-5">
				<div class="flex items-center justify-between">
					<label class="text-xs font-semibold uppercase tracking-wide text-white/70" for="keyword-input">
						{m.giveaway_keyword_label()}
					</label>
					<button
						type="button"
						class="text-xs font-semibold uppercase tracking-wide text-white/60 transition hover:text-white"
						onclick={resetGiveaway}
					>
						{m.giveaway_reset_button()}
					</button>
				</div>
				<div class="flex flex-col gap-2 sm:flex-row">
					<input
						id="keyword-input"
						type="text"
						class="w-full rounded-xl border border-white/30 bg-white/10 px-3 py-2 text-sm text-white placeholder:text-white/50 focus:border-white/70 focus:outline-none disabled:opacity-50"
						placeholder={m.giveaway_keyword_placeholder()}
						bind:value={keywordInput}
						disabled={phase === 'collecting'}
					/>
					<button
						type="submit"
						class="rounded-xl border border-white/30 bg-white/10 px-4 py-2 text-sm font-semibold tracking-wide text-white uppercase transition hover:bg-white/20 disabled:opacity-60"
						disabled={phase === 'collecting'}
					>
						{m.giveaway_keyword_action()}
					</button>
				</div>
				<p class="text-xs text-white/70">{phaseInstructions()}</p>
				{#if keywordError}
					<p class="text-xs text-rose-200" aria-live="polite">{keywordError}</p>
				{/if}
				{#if phase === 'collecting' && activeKeyword}
					<p class="rounded-xl bg-white/10 px-3 py-2 text-sm font-semibold text-emerald-100">
						{m.giveaway_keyword_hint_command({ keyword: activeKeyword })}
					</p>
				{/if}
			</div>
		</form>

		<section class="rounded-2xl border border-slate-200/70 bg-white/70 p-4 shadow-sm dark:border-slate-800/80 dark:bg-slate-900/80">
			<div class="flex flex-wrap items-center gap-3">
				<div>
					<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
						{m.giveaway_participants_label()}
					</p>
					<p class="text-xs text-slate-500 dark:text-slate-400">
						{m.giveaway_participants_count({ count: participants.length })}
					</p>
				</div>
				<button
					type="button"
					class="ml-auto rounded-xl bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-emerald-500 disabled:opacity-60"
					onclick={finishGiveaway}
					disabled={phase !== 'collecting' || participants.length === 0}
				>
					{m.giveaway_finish_button()}
				</button>
			</div>
			<div class="mt-4 flex flex-col gap-2 sm:flex-row">
				<input
					type="search"
					class="w-full rounded-xl border border-slate-200/80 bg-white/90 px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:border-slate-400 focus:outline-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
					placeholder={m.giveaway_participants_search_placeholder()}
					bind:value={searchQuery}
				/>
			</div>

			{#if filteredParticipants.length === 0}
				<p class="mt-6 rounded-xl border border-dashed border-slate-300/70 px-4 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
					{m.giveaway_participants_empty()}
				</p>
			{:else}
				<ul class="mt-4 max-h-72 space-y-3 overflow-y-auto pr-1">
					{#each filteredParticipants as participant}
						<li class="rounded-2xl border border-slate-200/70 bg-white/90 p-3 text-sm dark:border-slate-700/70 dark:bg-slate-900/70">
							<div class="flex flex-wrap items-center gap-2">
								<div>
									<p class="font-semibold">{participant.username}</p>
									<p class="text-xs text-slate-500 dark:text-slate-400">
										{participant.platform.toUpperCase()} · {participant.channel_id}
									</p>
								</div>
								<span class="ml-auto text-xs text-slate-500 dark:text-slate-400">
									{formatTimestamp(participant.joined_at)}
								</span>
							</div>
							{#if isWinner(participant)}
								<p class="mt-2 rounded-full bg-emerald-100/80 px-2 py-1 text-xs font-semibold text-emerald-900 dark:bg-emerald-500/20 dark:text-emerald-200">
									{m.giveaway_participant_winner_badge()}
								</p>
							{:else if isDisqualified(participant)}
								<p class="mt-2 rounded-full bg-slate-200/80 px-2 py-1 text-xs font-semibold text-slate-700 dark:bg-slate-700/40 dark:text-slate-200">
									{m.giveaway_participant_disqualified_badge()}
								</p>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	</div>
</section>

{#if winnerModalOpen && winner}
	<div class="fixed inset-0 z-40 flex items-center justify-center bg-slate-950/70 px-4">
		<div class="w-full max-w-2xl rounded-3xl border border-slate-200/70 bg-white p-6 text-slate-900 shadow-2xl dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100">
			<header class="space-y-1">
				<p class="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.giveaway_modal_title()}
				</p>
				<h3 class="text-2xl font-bold">{winner.username}</h3>
				<p class="text-sm text-slate-500 dark:text-slate-400">
					{m.giveaway_modal_subtitle({ username: winner.username })}
				</p>
			</header>

			<div class="mt-4 flex flex-wrap gap-2 text-xs text-slate-600 dark:text-slate-400">
				<span class="rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-800">
					{winner.platform.toUpperCase()}
				</span>
				<span class="rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-800">
					{winner.channel_id}
				</span>
				<span class="rounded-full bg-slate-100 px-3 py-1 dark:bg-slate-800">
					{m.giveaway_modal_listening_label()}
				</span>
			</div>

			<div class="mt-5 rounded-2xl border border-slate-200/70 bg-white/80 p-4 dark:border-slate-700 dark:bg-slate-900/70">
				{#if winnerMessages.length === 0}
					<p class="text-sm text-slate-500 dark:text-slate-400">
						{m.giveaway_modal_waiting()}
					</p>
				{:else}
					<ul class="space-y-3 max-h-60 overflow-y-auto pr-1">
						{#each winnerMessages as message (messageKey(message))}
							<li class="rounded-xl bg-slate-900/5 p-3 dark:bg-white/5">
								<div class="flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400">
									<span class="font-semibold text-slate-700 dark:text-slate-200">{message.platform.toUpperCase()}</span>
									<time>{formatTimestamp(message.received_at)}</time>
								</div>
								<p class="mt-1 text-sm text-slate-900 dark:text-slate-100">{message.text}</p>
							</li>
						{/each}
					</ul>
				{/if}
				{#if modalError}
					<p class="mt-3 text-xs text-rose-500 dark:text-rose-300" aria-live="polite">{modalError}</p>
				{/if}
			</div>

			<div class="mt-6 flex flex-wrap justify-end gap-3">
				<button
					type="button"
					class="rounded-full border border-slate-300 px-4 py-2 text-sm font-semibold text-slate-700 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800"
					onclick={closeModal}
				>
					{m.giveaway_modal_close()}
				</button>
				<button
					type="button"
					class="rounded-full bg-amber-500 px-4 py-2 text-sm font-semibold text-white transition hover:bg-amber-400 disabled:opacity-60"
					onclick={rerollWinner}
					disabled={!canReroll}
				>
					{m.giveaway_modal_reroll()}
				</button>
			</div>
		</div>
	</div>
{/if}
