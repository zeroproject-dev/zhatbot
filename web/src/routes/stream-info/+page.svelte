<script lang="ts">
	import StreamInfoPanel from '$lib/components/StreamInfoPanel.svelte';
	import AuthActions from '$lib/components/AuthActions.svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { theme, toggleTheme } from '$lib/stores/theme';
</script>

<div class="min-h-screen bg-slate-50 px-4 py-6 text-slate-900 transition-colors dark:bg-slate-950 dark:text-white">
	<div class="mx-auto flex min-h-[calc(100vh-48px)] max-w-5xl flex-col gap-4">
		<header class="flex items-center justify-between border-b border-white/10 pb-3">
			<h1 class="text-base font-semibold uppercase tracking-wide text-slate-600 dark:text-white/70">
				Stream info
			</h1>
			<button
				class="inline-flex items-center gap-2 rounded-full border border-white/30 px-4 py-2 text-sm font-medium text-white/80 transition hover:bg-white/10 dark:text-white"
				type="button"
				onclick={toggleTheme}
				aria-label={m.theme_toggle_aria()}
			>
				<span class="text-lg" aria-hidden="true">{$theme === 'light' ? '☾' : '☀︎'}</span>
				{$theme === 'light' ? m.theme_toggle_dark() : m.theme_toggle_light()}
			</button>
		</header>

		<div class="grid flex-1 gap-4 lg:grid-cols-2">
			<StreamInfoPanel />
			<AuthActions />
		</div>
	</div>
</div>
