import { browser } from '$app/environment';
import { readable, type Readable } from 'svelte/store';
import type { ChatCommandPayload, ChatMessage, ChatStreamStatus } from '$lib/types/chat';
import { WS_URL } from '$lib/config';

interface ChatStreamOptions {
	url?: string;
	maxMessages?: number;
	useMockFeed?: boolean;
}

export interface ChatStreamState {
	messages: ChatMessage[];
	status: ChatStreamStatus;
}

const initialState: ChatStreamState = {
	messages: [],
	status: 'disconnected'
};

let activeSocket: WebSocket | undefined;

export const createChatStream = (options: ChatStreamOptions = {}): Readable<ChatStreamState> => {
	return readable(initialState, (set) => {
		if (!browser) return;

		const maxMessages = options.maxMessages ?? 200;
		const url = normalizeWsUrl(options.url ?? WS_URL);
		let status: ChatStreamStatus = 'connecting';
		let messages: ChatMessage[] = [];
		let socket: WebSocket | undefined;
		let reconnectTimer: number | undefined;
		let attempts = 0;

		const update = () => set({ messages, status });
		const push = (payload: ChatMessage) => {
			const enriched: ChatMessage = {
				...payload,
				received_at: payload.received_at ?? new Date().toISOString()
			};
			messages = [enriched, ...messages].slice(-maxMessages);
			update();
		};

		if (options.useMockFeed) {
			const stopMock = startMockFeed(push);
			status = 'connected';
			update();
			return () => {
				stopMock();
			};
		}

		if (!url) {
			console.error('[chat-stream] Falta la URL de WebSocket (VITE_CHAT_WS_URL).');
			status = 'disconnected';
			update();
			return () => undefined;
		}

		const cleanupSocket = () => {
			if (activeSocket === socket) {
				activeSocket = undefined;
			}
			socket?.close();
			socket = undefined;
		};

		const scheduleReconnect = () => {
			if (!browser) return;
			if (reconnectTimer) {
				window.clearTimeout(reconnectTimer);
			}
			attempts += 1;
			const delay = Math.min(30000, 1000 * 2 ** attempts);
			reconnectTimer = window.setTimeout(connect, delay);
		};

		const connect = () => {
			cleanupSocket();
			status = 'connecting';
			update();
			console.info('[chat-stream] Intentando conectar WebSocket', { url });
			try {
				socket = new WebSocket(url);
				activeSocket = socket;
			} catch (error) {
				console.error('[chat-stream] No se pudo construir el WebSocket.', error);
				status = 'disconnected';
				update();
				scheduleReconnect();
				return;
			}

			socket.addEventListener('open', () => {
				console.info('[chat-stream] Conectado al WebSocket', { url });
				attempts = 0;
				status = 'connected';
				update();
			});

			socket.addEventListener('close', (event) => {
				console.warn('[chat-stream] WebSocket cerrado', { url, event });
				status = 'disconnected';
				update();
				scheduleReconnect();
			});

			socket.addEventListener('error', (event) => {
				console.error('[chat-stream] Error en WebSocket', { url, event });
			});

			socket.addEventListener('message', (event) => {
				try {
					const parsed = JSON.parse(event.data) as unknown;
					const normalized = normalizeMessagePayload(parsed);
					push(normalized);
				} catch (error) {
					console.error('[chat-stream] No se pudo procesar el mensaje entrante', error, event.data);
				}
			});
		};

		connect();

		return () => {
			if (reconnectTimer) {
				window.clearTimeout(reconnectTimer);
			}
			cleanupSocket();
		};
	});
};

const sampleUsers = [
	{ username: 'MrZeroProject', platform: 'twitch', is_platform_owner: true },
	{ username: 'ModNinja', platform: 'kick', is_platform_mod: true },
	{ username: 'VIPLegend', platform: 'twitch', is_platform_vip: true },
	{ username: 'Viewer123', platform: 'kick' },
	{ username: 'StreamerFriend', platform: 'twitch', is_platform_admin: true }
] satisfies Partial<ChatMessage>[];

const startMockFeed = (push: (message: ChatMessage) => void) => {
	const interval = window.setInterval(() => {
		const base = sampleUsers[Math.floor(Math.random() * sampleUsers.length)];

		const mock: ChatMessage = {
			platform: base.platform ?? 'twitch',
			channel_id: '#zeroproject',
			user_id: crypto.randomUUID(),
			username: base.username ?? 'anon',
			text: randomMessage(),
			is_private: false,
			is_platform_owner: Boolean(base.is_platform_owner),
			is_platform_admin: Boolean(base.is_platform_admin),
			is_platform_mod: Boolean(base.is_platform_mod),
			is_platform_vip: Boolean(base.is_platform_vip)
		};

		push(mock);
	}, 3500);

	return () => {
		window.clearInterval(interval);
	};
};

const randomMessage = () => {
	const texts = [
		'Hola chat!',
		'¿Listos para la próxima raid?',
		'Recuerden seguir la cuenta para más contenido.',
		'Oigan, esa jugada fue increíble.',
		'¿Quién viene desde Kick hoy?',
		'No olvides hidratarte.'
	];

	return texts[Math.floor(Math.random() * texts.length)];
};

export const normalizeMessagePayload = (payload: unknown): ChatMessage => {
	if (typeof payload !== 'object' || payload === null) {
		throw new Error('Payload inválido');
	}

	const source = payload as Record<string, unknown>;

	const getString = (...keys: string[]) => {
		for (const key of keys) {
			const value = source[key];
			if (typeof value === 'string') {
				return value;
			}
		}
		return '';
	};

	const getBoolean = (...keys: string[]) => {
		for (const key of keys) {
			const value = source[key];
			if (typeof value === 'boolean') {
				return value;
			}
			if (typeof value === 'number') {
				return Boolean(value);
			}
		}
		return false;
	};

	const platform = getString('platform', 'Platform') || 'unknown';
	const channel_id = getString('channel_id', 'ChannelID', 'channelId', 'channel');
	const user_id = getString('user_id', 'UserID', 'userId');
	const username = getString('username', 'Username', 'userName') || 'Guest';
	const text = getString('text', 'Text');
	const received_at =
		getString('received_at', 'ReceivedAt', 'receivedAt') || new Date().toISOString();

	return {
		platform,
		channel_id: channel_id || '#unknown',
		user_id: user_id || crypto.randomUUID(),
		username,
		text,
		is_private: getBoolean('is_private', 'IsPrivate'),
		is_platform_owner: getBoolean('is_platform_owner', 'IsPlatformOwner'),
		is_platform_admin: getBoolean('is_platform_admin', 'IsPlatformAdmin'),
		is_platform_mod: getBoolean('is_platform_mod', 'IsPlatformMod'),
		is_platform_vip: getBoolean('is_platform_vip', 'IsPlatformVip'),
		received_at
	};
};

export const sendChatCommand = async (payload: ChatCommandPayload) => {
	if (!browser) throw new Error('Solo disponible en el navegador');
	const socket = activeSocket;
	if (!socket || socket.readyState !== WebSocket.OPEN) {
		throw new Error('No hay conexión activa');
	}

	if (!payload.text || payload.text.trim().length === 0) {
		throw new Error('El texto es obligatorio');
	}

	const message = {
		platform: 'twitch',
		username: 'web-user',
		user_id: 'web',
		...payload,
		text: payload.text.trim()
	};

	socket.send(JSON.stringify(message));
};

const normalizeWsUrl = (input?: string) => {
	if (!input) return input;
	if (input.startsWith('http://')) return input.replace('http://', 'ws://');
	if (input.startsWith('https://')) return input.replace('https://', 'wss://');
	return input;
};
