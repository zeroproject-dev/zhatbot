import { API_BASE_URL } from '$lib/config';
import {
	isWails,
	ttsGetSettings,
	ttsUpdateSettings as ttsUpdateSettingsBinding
} from '$lib/wails/adapter';

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

const normalizeStatus = (payload: any): TTSStatus => {
	if (!payload) {
		return { enabled: false, voice: '', voices: [] };
	}
	const voiceObj = typeof payload.voice === 'object' ? payload.voice : undefined;
	const voicesList: TTSVoice[] = Array.isArray(payload.voices)
		? payload.voices.map((item: any) => ({
				code: item?.code ?? item?.Code ?? '',
				label: item?.label ?? item?.Label ?? ''
			}))
		: [];

	return {
		enabled: Boolean(payload.enabled),
		voice: voiceObj?.code ?? payload.voice ?? '',
		voice_label: voiceObj?.label ?? payload.voice_label ?? '',
		voices: voicesList
	};
};

export const fetchTTSStatus = async (): Promise<TTSStatus> => {
	if (isWails()) {
		const snapshot = await ttsGetSettings();
		return normalizeStatus(snapshot);
	}
	const response = await fetch(`${baseUrl}/api/tts/status`);
	if (!response.ok) {
		throw new Error(`Request failed ${response.status}`);
	}
	const payload = await response.json();
	return normalizeStatus(payload);
};

export const updateTTSSettings = async (payload: { voice?: string; enabled?: boolean }) => {
	if (isWails()) {
		const snapshot = await ttsUpdateSettingsBinding(payload);
		return normalizeStatus(snapshot);
	}
	const response = await fetch(`${baseUrl}/api/tts/settings`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!response.ok) {
		const detail = await response.text();
		throw new Error(detail || `Update failed ${response.status}`);
	}
	const data = await response.json();
	return normalizeStatus(data);
};
