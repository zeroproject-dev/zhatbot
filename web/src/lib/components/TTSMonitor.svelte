<script lang="ts">
	import { ttsQueue, type TTSEvent } from '$lib/stores/tts';

	const events = $derived($ttsQueue as TTSEvent[]);
	const latest = $derived(events.at(0));

	const formatTime = (iso?: string) => {
		if (!iso) return '';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	};

	const audioSrc = $derived(
		latest?.audio_base64 ? `data:audio/mpeg;base64,${latest.audio_base64}` : ''
	);

	let lastAutoPlayed = $state<string | null>(null);

	const playLatest = async (reason: 'auto' | 'manual' = 'manual') => {
		if (!audioSrc) {
			console.warn(`[tts] intento de reproducción sin audio disponible (${reason})`, latest);
			return;
		}
		try {
			console.debug(`[tts] inicializando reproducción (${reason})`, latest);
			const audio = new Audio(audioSrc);
			audio.onplay = () => console.debug(`[tts] reproduciendo audio (${reason})`);
			audio.onended = () => console.debug(`[tts] audio finalizado (${reason})`);
			await audio.play();
		} catch (error) {
			console.error(`No se pudo reproducir el TTS (${reason})`, error, latest);
		}
	};

	$effect(() => {
		if (!latest?.timestamp || !audioSrc) return;
		if (latest.timestamp === lastAutoPlayed) return;
		lastAutoPlayed = latest.timestamp;
		void playLatest('auto');
	});
</script>

<article class="rounded-2xl border border-slate-200/60 bg-white/80 p-4 text-slate-900 shadow-inner dark:border-slate-800 dark:bg-slate-900/70 dark:text-white">
	<header class="flex items-center justify-between text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
		<span>TTS</span>
		{#if latest}
			<button
				type="button"
				class="rounded-full border border-slate-300 px-3 py-1 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-white/10"
				onclick={() => playLatest()}
			>
				Reproducir
			</button>
		{/if}
	</header>

	{#if latest}
		<div class="mt-3 space-y-1 text-sm">
			<p class="font-semibold text-slate-800 dark:text-white">{latest.requested_by}</p>
			<p class="rounded-2xl bg-slate-100/80 p-3 text-slate-700 shadow-inner dark:bg-white/10 dark:text-white">{latest.text}</p>
			<p class="text-xs text-slate-400 dark:text-slate-500">
				{latest.voice_label ?? latest.voice} · {formatTime(latest.timestamp)}
			</p>
		</div>
	{:else}
		<p class="mt-3 text-sm text-slate-500 dark:text-slate-400">Aún no llegan peticiones de TTS.</p>
	{/if}

	{#if events.length > 1}
		<ul class="mt-4 space-y-1 text-xs text-slate-500 dark:text-slate-400">
			{#each events.slice(1, 4) as event}
				<li class="flex items-center justify-between rounded-xl bg-slate-100/40 px-3 py-1 dark:bg-white/5">
					<span class="truncate">{event.requested_by}: {event.text}</span>
					<span>{event.voice}</span>
				</li>
			{/each}
		</ul>
	{/if}
</article>
