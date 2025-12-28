import { isWails, callWailsBinding } from '$lib/wails/adapter';
import type {
	CreateNotificationPayload,
	NotificationRecord
} from '$lib/types/notification';

const BASE_URL = '/api/notifications';

export const fetchNotifications = async (limit = 50): Promise<NotificationRecord[]> => {
	if (isWails()) {
		return await callWailsBinding<NotificationRecord[]>('Notifications_List', limit);
	}
	const params = new URLSearchParams();
	if (limit > 0) {
		params.set('limit', String(limit));
	}
	const response = await fetch(`${BASE_URL}?${params.toString()}`, {
		headers: {
			Accept: 'application/json'
		}
	});

	if (!response.ok) {
		throw new Error('Failed to load notifications');
	}

	return (await response.json()) as NotificationRecord[];
};

export const createNotification = async (
	payload: CreateNotificationPayload
): Promise<NotificationRecord> => {
	const response = await fetch(BASE_URL, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Accept: 'application/json'
		},
		body: JSON.stringify(payload)
	});

	if (!response.ok) {
		throw new Error('Failed to save notification');
	}

	return (await response.json()) as NotificationRecord;
};
