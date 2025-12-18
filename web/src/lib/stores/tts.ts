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
			update((list) => [event, ...list].slice(0, 50));
		}
	};
};

export const ttsQueue = createQueue();
