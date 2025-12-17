<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { locales, localizeHref } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';
	import { theme, toggleTheme, initTheme } from '$lib/stores/theme';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();

	const isStandaloneChat = $derived(page.route.id === '/chat');

	onMount(() => {
		initTheme();
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if isStandaloneChat}
	{@render children()}
{:else}
	<div class="min-h-screen bg-slate-50 text-slate-900 transition-colors dark:bg-slate-950 dark:text-slate-100">
		<header class="flex items-center justify-between border-b border-slate-200/70 px-6 py-4 dark:border-slate-800/80">
			<div>
				<p class="text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">{m.brand_tagline()}</p>
				<h1 class="text-xl font-semibold">{m.dashboard_title()}</h1>
			</div>
			<button
				class="inline-flex items-center gap-2 rounded-full border border-slate-200/70 px-4 py-2 text-sm font-medium shadow-sm transition-colors hover:bg-slate-100 dark:border-slate-700 dark:hover:bg-slate-800"
				onclick={toggleTheme}
				type="button"
				aria-live="polite"
				aria-label={m.theme_toggle_aria()}
			>
				<span class="h-4 w-4 rounded-full border border-slate-300 bg-gradient-to-tr from-amber-200 to-amber-400 shadow-inner dark:hidden"></span>
				<span class="hidden h-4 w-4 items-center justify-center rounded-full bg-slate-800 text-xs text-amber-200 dark:flex">☾</span>
				{$theme === 'light' ? m.theme_toggle_dark() : m.theme_toggle_light()}
			</button>
		</header>
		<main class="flex-1 p-6">{@render children()}</main>
	</div>
{/if}

<div class="hidden">
	{#each locales as locale}
		<a href={localizeHref(page.url.pathname, { locale })}>
			{locale}
		</a>
	{/each}
</div>
