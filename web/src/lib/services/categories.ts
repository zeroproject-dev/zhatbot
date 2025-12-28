import { API_BASE_URL } from '$lib/config';
import { isWails, callWailsBinding } from '$lib/wails/adapter';

export type CategoryOption = {
	ID: string;
	Name: string;
};

export type Platform = 'twitch' | 'kick';

const baseUrl = API_BASE_URL ?? 'http://localhost:8080';

export const searchCategories = async (
	platform: Platform,
	query: string
): Promise<CategoryOption[]> => {
	if (isWails()) {
		return await callWailsBinding<CategoryOption[]>('Category_Search', platform, query);
	}
	const url = new URL('/api/categories/search', baseUrl);
	url.searchParams.set('platform', platform);
	url.searchParams.set('query', query);

	const response = await fetch(url);
	if (!response.ok) {
		throw new Error(`Search failed ${response.status}`);
	}

	const data = (await response.json()) as { options?: CategoryOption[] };
	return data.options ?? [];
};

export const updateCategory = async (platform: Platform, name: string): Promise<void> => {
	if (isWails()) {
		await callWailsBinding('Category_Update', platform, name);
		return;
	}
	const response = await fetch(`${baseUrl}/api/categories/update`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ platform, name })
	});

	if (!response.ok) {
		throw new Error(`Update failed ${response.status}`);
	}
};
