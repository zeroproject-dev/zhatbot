declare global {
	interface Window {
		__CONFIG__?: {
			WS_URL?: string;
			API_BASE_URL?: string;
		};
	}
}

const cfg = typeof window !== 'undefined' ? (window.__CONFIG__ ?? {}) : {};

export const WS_URL = cfg.WS_URL ?? import.meta.env.VITE_CHAT_WS_URL;

export const API_BASE_URL = cfg.API_BASE_URL ?? import.meta.env.VITE_API_BASE_URL;
