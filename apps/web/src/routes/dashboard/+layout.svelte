<script lang="ts">
	import Sidebar from './_components/Sidebar.svelte';
	let { data, children } = $props();
</script>

<div class="shell">
	<Sidebar user={data.user} />
	<main>{@render children()}</main>
</div>

<style>
	.shell { display: grid; grid-template-columns: 14rem 1fr; min-height: 100vh; }
	main { padding: 1.5rem; }
</style>
