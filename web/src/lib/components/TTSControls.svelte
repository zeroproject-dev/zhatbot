<script lang="ts">
	import { onMount } from 'svelte';
	import { fetchTTSStatus, updateTTSSettings, type TTSStatus } from '$lib/services/tts';
	import { ttsVolume } from '$lib/stores/tts';

	const volume = $derived($ttsVolume as number);

	let status = $state<TTSStatus | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let saving = $state(false);

	const loadStatus = async () => {
		loading = true;
		error = null;
		try {
			status = await fetchTTSStatus();
		} catch (err) {
			console.error('tts status failed', err);
			error = 'No se pudo cargar el estado de TTS.';
		} finally {
			loading = false;
		}
	};

	const toggleEnabled = async () => {
		if (!status) return;
		saving = true;
		try {
			status = await updateTTSSettings({ enabled: !status.enabled });
		} catch (err) {
			console.error('tts toggle failed', err);
			error = 'No se pudo actualizar el estado.';
		} finally {
			saving = false;
		}
	};

	const changeVoice = async (voice: string) => {
		if (!voice) return;
		saving = true;
		try {
			status = await updateTTSSettings({ voice });
		} catch (err) {
			console.error('tts voice failed', err);
			error = 'No se pudo cambiar la voz.';
		} finally {
			saving = false;
		}
	};

	const handleVolume = (value: number) => {
		ttsVolume.set(value);
	};

	onMount(() => {
		void loadStatus();
	});
</script>

<article class="rounded-2xl border border-slate-200/60 bg-white/80 p-4 text-slate-900 shadow-inner dark:border-slate-800 dark:bg-slate-900/70 dark:text-white">
	<header class="flex items-center justify-between text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
		<span>TTS Control</span>
		<button
			type="button"
			class="text-[11px] uppercase text-slate-400 hover:text-slate-600 dark:text-slate-500 dark:hover:text-slate-300"
			onclick={() => loadStatus()}
			disabled={loading}
		>
			{loading ? '…' : 'refresh'}
		</button>
	</header>

	<div class="mt-3 space-y-4 text-sm">
		<div class="flex items-center justify-between">
			<span>Estado TTS</span>
			<label class="relative inline-flex cursor-pointer items-center">
				<input
					type="checkbox"
					class="peer sr-only"
					checked={status?.enabled}
					onchange={toggleEnabled}
					disabled={saving || loading}
				/>
				<div class="h-5 w-9 rounded-full bg-slate-300 transition peer-checked:bg-emerald-400 peer-focus:outline peer-focus:outline-2 peer-focus:outline-emerald-500"></div>
				<div class="peer absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white transition peer-checked:translate-x-4"></div>
			</label>
		</div>

		<div>
			<label for="tts-voice" class="text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">Voz</label>
			<select
				id="tts-voice"
				class="mt-1 w-full rounded-xl border border-slate-200 bg-white/60 px-3 py-2 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900/50 dark:text-white"
				value={status?.voice ?? ''}
				onchange={(event) => changeVoice(event.currentTarget.value)}
				disabled={saving || loading || !status}
			>
				{#if status?.voices}
					{#each status.voices as voice}
						<option value={voice.code}>{voice.label}</option>
					{/each}
				{:else}
					<option>Cargando...</option>
				{/if}
			</select>
		</div>

		<div>
			<label for="tts-volume" class="flex items-center justify-between text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">
				<span>Volumen</span>
				<span>{Math.round((volume ?? 1) * 100)}%</span>
			</label>
			<input
				id="tts-volume"
				type="range"
				min="0"
				max="100"
				value={(volume ?? 1) * 100}
				class="mt-2 w-full accent-emerald-400"
				oninput={(event) => handleVolume(Number(event.currentTarget.value) / 100)}
			/>
		</div>
	</div>

	{#if error}
		<p class="mt-3 text-xs text-rose-400" aria-live="polite">{error}</p>
	{/if}
</article>
