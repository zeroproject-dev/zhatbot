<script lang="ts">
	import type { ChatToken } from '$lib/types/chat';

	export let tokens: ChatToken[] = [];
	export let fallbackText = '';

	const buildSrcSet = (token: Extract<ChatToken, { type: 'emote' }>) => {
		const entries = [];
		if (token.url2x) {
			entries.push(`${token.url2x} 2x`);
		}
		if (token.url3x) {
			entries.push(`${token.url3x} 3x`);
		}
		return entries.join(', ');
	};
</script>

{#if tokens?.length}
	{#each tokens as token, index (index)}
		{#if token.type === 'emote'}
			<img
				class="inline-emote"
				src={token.url}
				alt={token.code}
				loading="lazy"
				decoding="async"
				srcset={buildSrcSet(token)}
			/>
		{:else}
			<span class="inline-text">{token.text}</span>
		{/if}
	{/each}
{:else}
	<span class="inline-text">{fallbackText}</span>
{/if}

<style>
	.inline-emote {
		height: 1.2em;
		vertical-align: middle;
		margin: 0 0.1em;
		image-rendering: -webkit-optimize-contrast;
	}

	.inline-text {
		white-space: pre-wrap;
	}
</style>
