import { API_BASE_URL } from '$lib/config';

export type TTSVoice = {
	code: string;
	label: string;
};

export type TTSStatus = {
	enabled: boolean;
	voice: string;
	voice_label?: string;
	voices: TTSVoice[];
};

const baseUrl = API_BASE_URL ?? 'http://localhost:8080';

export const fetchTTSStatus = async (): Promise<TTSStatus> => {
	const response = await fetch(`${baseUrl}/api/tts/status`);
	if (!response.ok) {
		throw new Error(`Request failed ${response.status}`);
	}
	return (await response.json()) as TTSStatus;
};

export const updateTTSSettings = async (payload: { voice?: string; enabled?: boolean }) => {
	const response = await fetch(`${baseUrl}/api/tts/settings`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const detail = await response.text();
		throw new Error(detail || `Update failed ${response.status}`);
	}
	return (await response.json()) as TTSStatus;
};
