<script lang="ts">
	import DialogHost from '../lib/components/DialogHost.svelte';
	let { children } = $props();
</script>

<svelte:head><title>EraPay</title></svelte:head>
{@render children()}
<DialogHost />

<style>
	:global(body) { margin: 0; font-family: system-ui, sans-serif; color: #111827; background: #f9fafb; }
	:global(body:not(:has(.auth)):not(:has(.adm)) a) { color: #0f766e; }
</style>
