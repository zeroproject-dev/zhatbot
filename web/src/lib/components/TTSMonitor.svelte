<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { m } from '$lib/paraglide/messages.js';
	import { ttsQueue, type TTSEvent, ttsVolume } from '$lib/stores/tts';
	import { isWails, onTTSSpoken } from '$lib/wails/adapter';

	const events = $derived($ttsQueue as TTSEvent[]);
	const latest = $derived(events.at(0));
	const volume = $derived($ttsVolume as number);

	const formatTime = (iso?: string) => {
		if (!iso) return '';
		const date = new Date(iso);
		if (Number.isNaN(date.getTime())) return '';
		return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	};

	let lastAutoPlayed = $state<string | null>(null);
	const desktopMode = browser && isWails();

	onMount(() => {
		if (!desktopMode) {
			return;
		}
		let unsubscribe: (() => void) | undefined;
		onTTSSpoken((payload) => {
			const event = normalizeSpokenEvent(payload as Record<string, unknown>);
			if (event) {
				ttsQueue.push(event);
			}
		}).then((off) => {
			unsubscribe = off;
		});
		return () => {
			unsubscribe?.();
		};
	});

	const playEvent = async (event: TTSEvent | undefined, reason: 'auto' | 'manual' | 'history') => {
		if (!event?.audio_base64 || desktopMode) {
			console.warn(`[tts] playback requested without audio (${reason})`, event);
			return;
		}
		const src = `data:audio/mpeg;base64,${event.audio_base64}`;
		try {
			console.debug(`[tts] initializing playback (${reason})`, event);
			const audio = new Audio(src);
			const vol = typeof volume === 'number' ? volume : 1;
			audio.volume = Math.min(Math.max(vol, 0), 1);
			audio.onplay = () => console.debug(`[tts] playing audio (${reason})`);
			audio.onended = () => console.debug(`[tts] audio finished (${reason})`);
			await audio.play();
		} catch (error) {
			console.error(`tts playback failed (${reason})`, error, event);
		}
	};

	$effect(() => {
		if (desktopMode) return;
		if (!latest?.timestamp || !latest.audio_base64) return;
		if (latest.timestamp === lastAutoPlayed) return;
		lastAutoPlayed = latest.timestamp;
		void playEvent(latest, 'auto');
	});

	const normalizeSpokenEvent = (payload: Record<string, unknown> | null): TTSEvent | null => {
		if (!payload) return null;
		return {
			voice: (payload.voice as string) ?? '',
			voice_label: (payload.voice_label as string) ?? '',
			text: (payload.text as string) ?? '',
			requested_by: (payload.requested_by as string) ?? '',
			platform: 'desktop',
			channel_id: '',
			timestamp: (payload.finished_at as string) ?? new Date().toISOString(),
			audio_base64: (payload.audio_base64 as string) ?? ''
		};
	};

	const canPlay = (event?: TTSEvent) => Boolean(event?.audio_base64) && !desktopMode;
</script>

<article class="rounded-2xl border border-slate-200/70 bg-white/90 p-4 text-slate-900 shadow-sm dark:border-slate-800 dark:bg-slate-900/70 dark:text-white">
	<header class="flex items-center justify-between text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
		<span>{m.tts_monitor_title()}</span>
		{#if latest && !desktopMode}
			<button
				type="button"
				class="rounded-full border border-slate-300 px-3 py-1 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-white/10"
				onclick={() => playEvent(latest, 'manual')}
				disabled={!canPlay(latest)}
			>
				{m.tts_monitor_play_latest()}
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
		<p class="mt-3 text-sm text-slate-500 dark:text-slate-400">{m.tts_monitor_empty()}</p>
	{/if}

	{#if events.length > 1}
		<ul class="mt-4 space-y-1 text-xs text-slate-500 dark:text-slate-400">
			{#each events.slice(1, 10) as event}
				<li class="flex items-center justify-between gap-2 rounded-xl bg-slate-100/40 px-3 py-1 dark:bg-white/5">
					<div class="flex-1 truncate">
						<span class="font-semibold">{event.requested_by}</span>: {event.text}
					</div>
					{#if !desktopMode}
						<button
							type="button"
							class="rounded-full border border-slate-300 px-2 py-1 text-xs dark:border-slate-700"
							onclick={() => playEvent(event, 'history')}
							disabled={!canPlay(event)}
						>
							{m.tts_monitor_history_play()}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</article>
