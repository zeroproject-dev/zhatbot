import type { CommandPayload, CommandRecord } from '$lib/types/command';
import { isWails, callWailsBinding } from '$lib/wails/adapter';

const BASE_URL = '/api/commands';

export const fetchCommands = async (): Promise<CommandRecord[]> => {
	if (isWails()) {
		return await callWailsBinding<CommandRecord[]>('ListCommands');
	}
	const response = await fetch(BASE_URL, {
		headers: {
			Accept: 'application/json'
		}
	});

	if (!response.ok) {
		throw new Error('Failed to load commands');
	}

	return (await response.json()) as CommandRecord[];
};

export const saveCommand = async (payload: CommandPayload): Promise<CommandRecord> => {
	if (isWails()) {
		return await callWailsBinding<CommandRecord>('UpsertCommand', payload);
	}
	const response = await fetch(BASE_URL, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Accept: 'application/json'
		},
		body: JSON.stringify(payload)
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error?.error || 'Failed to save command');
	}

	return (await response.json()) as CommandRecord;
};

export const deleteCommand = async (name: string): Promise<void> => {
	if (isWails()) {
		await callWailsBinding<void>('DeleteCommand', name);
		return;
	}
	const params = new URLSearchParams();
	params.set('name', name);
	const response = await fetch(`${BASE_URL}?${params.toString()}`, {
		method: 'DELETE',
		headers: {
			Accept: 'application/json'
		}
	});
	if (!response.ok) {
		const error = await response.json().catch(() => ({}));
		throw new Error(error?.error || 'Failed to delete command');
	}
};
