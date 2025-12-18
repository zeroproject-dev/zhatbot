<script lang="ts">
	import { browser } from '$app/environment';
	import { getLocale, locales, setLocale } from '$lib/paraglide/runtime';
	import { m } from '$lib/paraglide/messages.js';

	type Locale = (typeof locales)[number];

	const available: ReadonlyArray<Locale> = locales;
	let current = $state(getLocale() as Locale);
	let switching = $state(false);

	const handleSelect = async (locale: Locale) => {
		if (!browser || locale === current || switching) return;
		switching = true;
		try {
			current = locale;
			await setLocale(locale);
		} catch (error) {
			console.error('locale switch failed', error);
		} finally {
			switching = false;
		}
	};
</script>

<div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
	<span class="hidden sm:inline">{m.lang_switch_label()}</span>
	<div class="inline-flex rounded-full border border-slate-200/70 bg-white/80 p-1 text-[11px] shadow-sm dark:border-slate-700 dark:bg-slate-900/60">
		{#each available as locale}
			<button
				type="button"
				class={`min-w-[3rem] rounded-full px-2 py-1 transition ${current === locale ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white'}`}
				onclick={() => handleSelect(locale)}
				disabled={switching}
			>
				{locale.toUpperCase()}
			</button>
		{/each}
	</div>
</div>
