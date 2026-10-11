<script lang="ts">
	let { user }: { user: { name: string; role: string } } = $props();
</script>

<nav>
	<strong>EraPay</strong>
	<a href="/dashboard">Ringkasan</a>
	<a href="/dashboard/transactions">Transaksi</a>
	<small>{user.name} · {user.role}</small>
</nav>

<style>
	nav { display: flex; flex-direction: column; gap: 0.75rem; padding: 1rem; background: #fff; border-right: 1px solid #e5e7eb; }
</style>
