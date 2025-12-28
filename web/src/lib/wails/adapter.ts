import { onMount } from 'svelte';

type Unsubscribe = () => void;
const noop: Unsubscribe = () => undefined;

declare global {
	interface Window {
		runtime?: {
			EventsOn?: (topic: string, cb: (payload: unknown) => void) => Unsubscribe | void;
		};
		go?: Record<string, any>;
	}
}

const isBrowser = () => typeof window !== 'undefined';

export const isWails = () => isBrowser() && typeof window.runtime !== 'undefined';

const getBridge = () => window.go?.main?.App;
const runtimeMode = import.meta.env.VITE_ZHATBOT_MODE ?? 'production';

const subscribeToEvent = async (topic: string, callback: (payload: unknown) => void) => {
	if (!isWails() || !topic) {
		return noop;
	}
	try {
		const unsubscribe = window.runtime?.EventsOn?.(topic, callback);
		if (typeof unsubscribe === 'function') {
			return unsubscribe as Unsubscribe;
		}
	} catch (error) {
		console.warn(`[wails] subscribe ${topic} failed`, error);
	}
	return noop;
};

export const ping = async (): Promise<string | null> => {
	if (!isWails()) {
		return null;
	}
	try {
		const bridge = getBridge();
		if (bridge?.Ping) {
			return await bridge.Ping();
		}
	} catch (error) {
		console.warn('[wails] ping failed', error);
	}
	return null;
};

export const onHeartbeat = (callback: (payload: unknown) => void) =>
	subscribeToEvent('app:heartbeat', callback);

export const onChatMessage = (callback: (payload: unknown) => void) =>
	subscribeToEvent('chat:message', callback);

export const onCommandsChanged = (callback: (payload: unknown) => void) =>
	subscribeToEvent('commands:changed', callback);

export const onTTSStatus = (callback: (payload: unknown) => void) =>
	subscribeToEvent('tts:status', callback);

export const onTTSSpoken = (callback: (payload: unknown) => void) =>
	subscribeToEvent('tts:spoken', callback);

export const callWailsBinding = async <T>(method: string, ...args: unknown[]): Promise<T> => {
	if (!isWails()) {
		throw new Error('not running inside Wails');
	}
	const bridge = getBridge();
	if (typeof bridge?.[method] !== 'function') {
		throw new Error(`binding ${method} not available`);
	}
	return bridge[method](...args) as Promise<T>;
};

export const ttsGetRunnerStatus = () => callWailsBinding('TTS_GetStatus');
export const ttsEnqueue = (text: string, voice: string, lang: string, rate: number, volume: number) =>
	callWailsBinding('TTS_Enqueue', text, voice, lang, rate, volume);
export const ttsStopAll = () => callWailsBinding('TTS_StopAll');
export const ttsGetSettings = () => callWailsBinding('TTS_GetSettings');
export const ttsUpdateSettings = (payload: { voice?: string; enabled?: boolean }) =>
	callWailsBinding('TTS_UpdateSettings', payload);

export const oauthStart = (platform: string, role: string) =>
	callWailsBinding<void>('OAuth_Start', platform, role);
export const oauthStatus = () => callWailsBinding<Record<string, any>>('OAuth_Status');
export const oauthLogout = (platform: string, role: string) =>
	callWailsBinding<void>('OAuth_Logout', platform, role);
export const configSetTwitchSecret = (secret: string) =>
	callWailsBinding<void>('Config_SetTwitchSecret', secret);

export const onOAuthComplete = (callback: (payload: unknown) => void) =>
	subscribeToEvent('oauth:complete', callback);

export const onOAuthStatusEvent = (callback: (payload: unknown) => void) =>
	subscribeToEvent('oauth:status', callback);
export const onOAuthMissingSecret = (callback: (payload: unknown) => void) =>
	subscribeToEvent('oauth:missing-secret', callback);

const setupDesktopNetworkGuards = () => {
	if (!isBrowser()) return;
	const guardKey = '__ZHATBOT_NET_GUARD__';
	if ((window as any)[guardKey]) {
		return;
	}
	const originalFetch = window.fetch.bind(window);
	window.fetch = ((...args: Parameters<typeof window.fetch>) => {
		if (isWails()) {
			console.error('[wails] fetch() invocado en modo desktop. Usa bindings/events.', args[0]);
		}
		return originalFetch(...args);
	}) as typeof window.fetch;

	const NativeWebSocket = window.WebSocket;
	(window as any).WebSocket = class GuardedWebSocket extends NativeWebSocket {
		constructor(url: string | URL, protocols?: string | string[]) {
			if (isWails()) {
				console.error('[wails] WebSocket constructor invocado en modo desktop.', url);
			}
			super(url, protocols as any);
		}
	} as typeof WebSocket;

	(window as any)[guardKey] = true;
};

if (runtimeMode === 'development') {
	setupDesktopNetworkGuards();
}

export const useWailsAdapter = () => {
	onMount(() => {
		if (!isWails()) return undefined;

		const cleanups: Unsubscribe[] = [];
		const track = (promise: Promise<Unsubscribe>) => {
			promise
				.then((off) => {
					cleanups.push(off);
				})
				.catch((err) => {
					console.warn('[wails] subscribe error', err);
				});
		};

		ping().then((response) => {
			if (response) {
				console.info('[wails] ping =>', response);
			}
		});

		track(
			onHeartbeat((payload) => {
				console.info('[wails] heartbeat', payload);
			})
		);

		track(
			onChatMessage((payload) => {
				console.info('[wails] chat message', payload);
			})
		);

		track(
			onCommandsChanged((payload) => {
				console.info('[wails] commands changed', payload);
			})
		);

		track(
			onTTSStatus((payload) => {
				console.info('[wails] tts status', payload);
			})
		);

		track(
			onTTSSpoken((payload) => {
				console.info('[wails] tts spoken', payload);
			})
		);

		return () => {
			cleanups.forEach((off) => off());
		};
	});
};
