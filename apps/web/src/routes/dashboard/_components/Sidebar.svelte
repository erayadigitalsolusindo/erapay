<script lang="ts">
	let { user }: { user: { email: string; merchantId: string; permissions?: string[] } } = $props();
</script>

<nav>
	<strong>EraPay</strong>
	<a href="/dashboard">Ringkasan</a>
	<a href="/dashboard/transactions">Transaksi</a>
	{#if user.merchantId && user.permissions?.includes('api_key:manage')}<a href="/dashboard/api-keys">API Keys</a>{/if}
	<div class="me">
		<small>{user.email}</small>
		<small>{user.merchantId ? 'Merchant' : 'Platform'}</small>
		<form method="POST" action="/logout"><button type="submit">Keluar</button></form>
	</div>
</nav>

<style>
	nav { display: flex; flex-direction: column; gap: 0.75rem; padding: 1rem; background: #fff; border-right: 1px solid #e5e7eb; }
	.me { margin-top: auto; display: flex; flex-direction: column; gap: 0.25rem; }
	small { color: #6b7280; overflow-wrap: anywhere; }
	button { margin-top: 0.5rem; padding: 0.4rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.5rem; background: #fff; cursor: pointer; }
	button:hover { border-color: #c8102e; color: #c8102e; }
</style>
