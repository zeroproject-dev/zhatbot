import { writable } from 'svelte/store';
import type { NotificationRecord } from '$lib/types/notification';
import { fetchNotifications } from '$lib/services/notifications';

const createNotificationsStore = () => {
	const { subscribe, set, update } = writable<NotificationRecord[]>([]);

	const refresh = async (limit = 50) => {
		const items = await fetchNotifications(limit);
		set(items);
	};

	const prepend = (entry: NotificationRecord) => {
		update((items) => {
			const next = [entry, ...items];
			return next.slice(0, 100);
		});
	};

	return {
		subscribe,
		refresh,
		prepend
	};
};

export const notificationsStore = createNotificationsStore();
