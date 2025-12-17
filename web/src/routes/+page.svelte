<script lang="ts">
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import AuthActions from '$lib/components/AuthActions.svelte';
</script>

<div class="flex flex-col gap-6 lg:h-[calc(100vh-180px)] lg:flex-row">
	<section class="flex flex-1">
		<ChatPanel />
	</section>
	<section class="flex flex-1 basis-1/3">
		<AuthActions />
	</section>
</div>
