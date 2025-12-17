<script lang="ts">
	import { browser } from '$app/environment';
	import {
		getSharedChatStream,
		sendChatCommand,
		type ChatStreamState
	} from '$lib/services/chat-stream';
	import {
		searchCategories,
		updateCategory,
		type CategoryOption,
		type Platform
	} from '$lib/services/categories';

	type Feedback = { type: 'success' | 'error'; message: string };

	const chatStream = getSharedChatStream();
	const streamState = $derived($chatStream as ChatStreamState);

	let titleDraft = $state('');
	let titleLoading = $state(false);
	let titleFeedback = $state<Feedback | null>(null);

	const createState = () => ({
		query: '',
		options: [] as CategoryOption[],
		loading: false,
		applyingId: null as string | null,
		feedback: null as Feedback | null
	});

	let platformState = $state<Record<Platform, ReturnType<typeof createState>>>({
		twitch: createState(),
		kick: createState()
	});

	const cards = [
		{
			id: 'twitch',
			label: 'Twitch',
			gradient: 'from-[#9146FF]/30 via-[#9146FF]/10 to-transparent',
			border: 'border-[#9146FF]/30',
			placeholder: 'Busca categorías de Twitch…',
			actionLabel: 'Aplicar en Twitch'
		},
		{
			id: 'kick',
			label: 'Kick',
			gradient: 'from-[#53FC18]/20 via-[#53FC18]/10 to-transparent',
			border: 'border-[#53FC18]/20',
			placeholder: 'Busca categorías de Kick…',
			actionLabel: 'Aplicar en Kick'
		}
	] satisfies Array<{
		id: Platform;
		label: string;
		gradient: string;
		border: string;
		placeholder: string;
		actionLabel: string;
	}>;

	const debounceTimers: Partial<Record<Platform, number>> = {};
	const debounceMs = 400;
	const minChars = 2;

	const statusBadge: Record<ChatStreamState['status'], { label: string; classes: string }> = {
		connecting: { label: 'Conectando…', classes: 'bg-amber-500/20 text-amber-100' },
		connected: { label: 'WS conectado', classes: 'bg-emerald-500/20 text-emerald-100' },
		disconnected: { label: 'Sin conexión', classes: 'bg-rose-500/20 text-rose-100' }
	};

	const canSendTitle = () => streamState.status === 'connected' && titleDraft.trim().length > 0;

	const submitTitle = async (event: SubmitEvent) => {
		event.preventDefault();
		titleFeedback = null;
		const payload = titleDraft.trim();
		if (!payload) {
			titleFeedback = { type: 'error', message: 'Escribe un nuevo título.' };
			return;
		}
		if (streamState.status !== 'connected') {
			titleFeedback = { type: 'error', message: 'Conéctate al WebSocket antes de enviar.' };
			return;
		}
		titleLoading = true;
		try {
			await sendChatCommand({ text: `!title ${payload}` });
			titleFeedback = { type: 'success', message: 'Título actualizado en todas las plataformas.' };
			titleDraft = '';
		} catch (error) {
			console.error('title update failed', error);
			titleFeedback = { type: 'error', message: 'No se pudo actualizar el título.' };
		} finally {
			titleLoading = false;
		}
	};

	const onQueryInput = (platform: Platform, value: string) => {
		platformState[platform].query = value;
		platformState[platform].feedback = null;
		if (debounceTimers[platform]) {
			clearTimeout(debounceTimers[platform]);
		}
		if (!browser) return;
		if (value.trim().length < minChars) {
			platformState[platform].options = [];
			return;
		}
		debounceTimers[platform] = window.setTimeout(() => {
			void performSearch(platform);
		}, debounceMs);
	};

	const performSearch = async (platform: Platform) => {
		const query = platformState[platform].query.trim();
		if (query.length < minChars) return;
		platformState[platform].loading = true;
		try {
			const options = await searchCategories(platform, query);
			platformState[platform].options = options;
			if (options.length === 0) {
				platformState[platform].feedback = { type: 'error', message: 'Sin resultados.' };
			} else {
				platformState[platform].feedback = null;
			}
		} catch (error) {
			console.error('category search failed', error);
			platformState[platform].feedback = {
				type: 'error',
				message: 'No se pudo buscar la categoría.'
			};
		} finally {
			platformState[platform].loading = false;
		}
	};

	const applyCategory = async (platform: Platform, option: CategoryOption) => {
		if (!option?.Name) return;
		platformState[platform].applyingId = option.ID || option.Name;
		platformState[platform].feedback = null;
		try {
			await updateCategory(platform, option.Name);
			platformState[platform].feedback = {
				type: 'success',
				message: `Categoría cambiada a ${option.Name}.`
			};
		} catch (error) {
			console.error('category update failed', error);
			platformState[platform].feedback = {
				type: 'error',
				message: 'No se pudo actualizar la categoría.'
			};
		} finally {
			platformState[platform].applyingId = null;
		}
	};
</script>

<section
	class="rounded-3xl border border-slate-200/60 bg-white/90 p-6 text-slate-800 shadow-inner shadow-black/5 dark:border-slate-800 dark:bg-slate-900/60 dark:text-slate-100"
>
	<header class="space-y-1">
		<p class="text-xs font-semibold tracking-wide text-slate-500 uppercase dark:text-slate-400">
			stream info
		</p>
		<p class="text-sm text-slate-500 dark:text-slate-400">
			Cambia título y categoría para Twitch y Kick desde un solo panel.
		</p>
	</header>

	<div class="mt-5 space-y-6">
		<article
			class="rounded-2xl border border-slate-200/50 bg-slate-900/80 p-4 text-white shadow-inner"
		>
			<div class="flex items-center justify-between gap-3">
				<div>
					<p class="text-xs font-semibold tracking-wide text-white/80 uppercase">Título</p>
					<p class="text-xs text-white/60">Envía un !title para ambas plataformas.</p>
				</div>
				<span
					class={`inline-flex items-center rounded-full px-3 py-1 text-[11px] font-semibold tracking-wide uppercase ${statusBadge[streamState.status].classes}`}
				>
					{statusBadge[streamState.status].label}
				</span>
			</div>
			<form class="mt-4 flex flex-col gap-3 sm:flex-row" onsubmit={submitTitle}>
				<input
					type="text"
					class="w-full rounded-xl border border-white/40 bg-white/10 px-3 py-2 text-sm text-white placeholder:text-white/60 focus:border-white/70 focus:outline-none"
					placeholder="Nuevo título para el stream"
					bind:value={titleDraft}
				/>
				<button
					type="submit"
					class="rounded-xl border border-white/40 bg-white/10 px-4 py-2 text-sm font-semibold tracking-wide text-white uppercase hover:bg-white/20 disabled:opacity-50"
					disabled={!canSendTitle() || titleLoading}
				>
					{titleLoading ? 'Actualizando…' : 'Actualizar'}
				</button>
			</form>
			{#if titleFeedback}
				<p
					class={`mt-2 text-xs ${titleFeedback.type === 'success' ? 'text-emerald-200' : 'text-rose-200'}`}
					aria-live="polite"
				>
					{titleFeedback.message}
				</p>
			{/if}
		</article>

		<div class="space-y-2">
			<div class="flex items-center justify-between">
				<p class="text-xs font-semibold tracking-wide text-slate-500 uppercase dark:text-slate-400">
					Categoría
				</p>
			</div>
			<div class="grid gap-4">
				{#each cards as card}
					{@const stateId = card.id}
					<article
						class={`rounded-2xl border ${card.border} bg-gradient-to-br ${card.gradient} p-4 text-white`}
					>
						<div
							class="flex items-center justify-between text-sm font-semibold tracking-wide uppercase"
						>
							<span>{card.label}</span>
							{#if platformState[stateId].loading}
								<span class="text-xs opacity-70">buscando…</span>
							{:else if platformState[stateId].applyingId}
								<span class="text-xs opacity-70">aplicando…</span>
							{/if}
						</div>

						<form
							class="mt-3 flex gap-2"
							onsubmit={(event) => {
								event.preventDefault();
								void performSearch(card.id);
							}}
						>
							<input
								type="text"
								class="w-full rounded-xl border border-white/40 bg-white/15 px-3 py-2 text-sm text-white placeholder-white/60 focus:border-white/70 focus:outline-none"
								placeholder={card.placeholder}
								value={platformState[stateId].query}
								oninput={(event) => onQueryInput(card.id, event.currentTarget.value)}
							/>
							<button
								type="submit"
								class="rounded-xl border border-white/50 px-4 text-sm font-semibold tracking-wide text-white/80 uppercase hover:bg-white/10"
								disabled={platformState[stateId].loading}
							>
								Buscar
							</button>
						</form>

						{#if platformState[stateId].options.length > 0}
							<ul class="mt-3 space-y-2">
								{#each platformState[stateId].options as option}
									<li>
										<button
											type="button"
											class="flex w-full items-center justify-between rounded-xl border border-white/20 bg-white/10 px-3 py-2 text-left text-sm hover:bg-white/20"
											onclick={() => applyCategory(card.id, option)}
											disabled={platformState[stateId].applyingId !== null}
										>
											<span>{option.Name}</span>
											{#if platformState[stateId].applyingId === (option.ID || option.Name)}
												<span class="text-xs tracking-wide uppercase">…</span>
											{/if}
										</button>
									</li>
								{/each}
							</ul>
						{:else if platformState[stateId].feedback?.type === 'error'}
							<p class="mt-3 text-xs text-rose-200" aria-live="polite">
								{platformState[stateId].feedback?.message}
							</p>
						{/if}

						{#if platformState[stateId].feedback?.type === 'success'}
							<p class="mt-3 text-xs text-emerald-200" aria-live="polite">
								{platformState[stateId].feedback?.message}
							</p>
						{/if}
					</article>
				{/each}
			</div>
		</div>
	</div>
</section>
