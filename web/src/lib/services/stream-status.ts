export type StreamStatusRecord = {
	platform: string;
	is_live: boolean;
	title?: string;
	game_title?: string;
	viewer_count?: number;
	url?: string;
	started_at?: string;
};

export const fetchStreamStatuses = async (): Promise<StreamStatusRecord[]> => {
	const response = await fetch('/api/streams/status', {
		headers: {
			Accept: 'application/json'
		}
	});

	if (!response.ok) {
		throw new Error('Failed to load stream statuses');
	}

	return (await response.json()) as StreamStatusRecord[];
};
