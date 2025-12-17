<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
</script>

<section class="flex h-full flex-col items-center justify-center rounded-3xl border border-dashed border-slate-200/80 bg-slate-50 text-center dark:border-slate-800 dark:bg-slate-900/40">
	<svg
		class="h-32 w-32 text-slate-400 dark:text-slate-600"
		viewBox="0 0 200 200"
		fill="none"
		role="presentation"
	>
		<rect x="20" y="120" width="160" height="40" rx="12" class="fill-current opacity-10" />
		<path
			d="M60 120L100 40L140 120"
			class="stroke-current"
			stroke-width="10"
			stroke-linecap="round"
			stroke-linejoin="round"
			opacity="0.4"
		/>
		<circle cx="100" cy="80" r="12" class="fill-current opacity-30" />
		<path
			d="M60 120H140"
			class="stroke-current"
			stroke-width="10"
			stroke-linecap="round"
			opacity="0.3"
		/>
	</svg>
	<h2 class="mt-6 text-2xl font-semibold text-slate-900 dark:text-white">{m.construction_title()}</h2>
	<p class="mt-2 max-w-sm text-sm text-slate-500 dark:text-slate-400">{m.construction_subtitle()}</p>
</section>
