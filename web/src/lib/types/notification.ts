export type NotificationType =
	| 'subscription'
	| 'donation'
	| 'bits'
	| 'giveaway_winner'
	| 'generic';

export type NotificationRecord = {
	id: number;
	type: NotificationType;
	platform?: string;
	username?: string;
	amount?: number;
	message?: string;
	metadata?: Record<string, string>;
	created_at: string;
};

export type CreateNotificationPayload = {
	type: NotificationType;
	platform?: string;
	username?: string;
	amount?: number;
	message?: string;
	metadata?: Record<string, string>;
};
