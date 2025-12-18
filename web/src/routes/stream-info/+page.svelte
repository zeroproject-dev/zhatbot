<script lang="ts">
	import AuthActions from '$lib/components/AuthActions.svelte';
	import StreamInfoPanel from '$lib/components/StreamInfoPanel.svelte';
	import LanguageSwitcher from '$lib/components/LanguageSwitcher.svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { theme, toggleTheme } from '$lib/stores/theme';
</script>

<div class="min-h-screen bg-slate-50/90 text-slate-900 transition-colors dark:bg-slate-950 dark:text-white">
	<div class="mx-auto flex min-h-screen w-full max-w-5xl flex-col gap-6 px-4 py-6">
		<header class="flex flex-wrap items-center justify-between gap-4 border-b border-slate-200/80 pb-3 dark:border-slate-800/80">
			<div>
				<p class="text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">
					{m.stream_info_tag()}
				</p>
				<h1 class="text-2xl font-semibold text-slate-900 dark:text-white">
					{m.stream_info_heading_full()}
				</h1>
			</div>
			<div class="flex items-center gap-3">
				<LanguageSwitcher />
				<button
					class="inline-flex items-center gap-2 rounded-full border border-slate-200/70 px-4 py-2 text-sm font-medium shadow-sm transition-colors hover:bg-slate-100 dark:border-slate-700 dark:hover:bg-slate-800"
					type="button"
					onclick={toggleTheme}
					aria-label={m.theme_toggle_aria()}
				>
					<span class="text-lg" aria-hidden="true">{$theme === 'light' ? '☾' : '☀︎'}</span>
					{$theme === 'light' ? m.theme_toggle_dark() : m.theme_toggle_light()}
				</button>
			</div>
		</header>

		<div class="grid flex-1 gap-4 pb-8 lg:grid-cols-2">
			<StreamInfoPanel />
			<AuthActions />
		</div>
	</div>
</div>
