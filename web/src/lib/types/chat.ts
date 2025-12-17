export interface ChatMessage {
	platform: string;
	channel_id: string;
	user_id: string;
	username: string;
	text: string;
	is_private: boolean;
	is_platform_owner: boolean;
	is_platform_admin: boolean;
	is_platform_mod: boolean;
	is_platform_vip: boolean;
	received_at?: string;
}

export type ChatStreamStatus = 'connecting' | 'connected' | 'disconnected';

export interface ChatCommandPayload {
	text: string;
	platform?: string;
	channel_id?: string;
	username?: string;
	user_id?: string;
}
