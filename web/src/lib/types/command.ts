export type CommandAccessRole =
	| 'everyone'
	| 'followers'
	| 'subscribers'
	| 'moderators'
	| 'vips'
	| 'owner';

export type CommandRecord = {
	name: string;
	response: string;
	aliases: string[];
	platforms: string[];
	permissions: CommandAccessRole[];
	updated_at?: string;
	source?: 'builtin' | 'custom';
	editable?: boolean;
	description?: string;
	usage?: string;
};

export type CommandPayload = {
	name: string;
	response?: string;
	aliases?: string[];
	platforms?: string[];
	permissions?: CommandAccessRole[];
};
