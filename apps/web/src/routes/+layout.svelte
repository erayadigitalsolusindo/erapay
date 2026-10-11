<script lang="ts">
	let { children } = $props();
</script>

<svelte:head><title>EraPay</title></svelte:head>
{@render children()}

<style>
	:global(body) { margin: 0; font-family: system-ui, sans-serif; color: #111827; background: #f9fafb; }
	:global(body:not(:has(.auth)) a) { color: #0f766e; }
</style>
