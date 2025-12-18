import { browser } from '$app/environment';
import { writable } from 'svelte/store';

export type TTSEvent = {
	voice: string;
	voice_label?: string;
	text: string;
	requested_by: string;
	platform: string;
	channel_id: string;
	timestamp: string;
	audio_base64?: string;
};

const createQueue = () => {
	const { subscribe, update } = writable<TTSEvent[]>([]);

	return {
		subscribe,
		push: (event: TTSEvent) => {
			update((list) => [event, ...list].slice(0, 10));
		}
	};
};

export const ttsQueue = createQueue();

const createVolumeStore = () => {
	const initial = browser ? Number(localStorage.getItem('tts_volume') ?? 1) : 1;
	const { subscribe, set } = writable(Math.min(Math.max(initial, 0), 1));

	return {
		subscribe,
		set: (value: number) => {
			const normalized = Math.min(Math.max(value, 0), 1);
			if (browser) {
				localStorage.setItem('tts_volume', normalized.toString());
			}
			set(normalized);
		}
	};
};

export const ttsVolume = createVolumeStore();
