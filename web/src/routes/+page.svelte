<script lang="ts">
	import ChatPanel from '$lib/components/ChatPanel.svelte';
	import AuthActions from '$lib/components/AuthActions.svelte';
	import StreamInfoPanel from '$lib/components/StreamInfoPanel.svelte';
</script>

<div class="flex flex-col gap-6 lg:grid lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
	<section class="flex flex-1">
		<ChatPanel />
	</section>
	<section class="flex flex-1">
		<div class="flex w-full flex-col gap-6">
			<AuthActions />
			<StreamInfoPanel />
		</div>
	</section>
</div>
