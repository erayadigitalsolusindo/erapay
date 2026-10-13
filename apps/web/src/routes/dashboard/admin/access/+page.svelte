<script lang="ts">
	let { data } = $props();
	let roles = $derived(data.roles as any[]);
	let permissions = $derived(data.permissions as any[]);
	let users = $derived(data.users as any[]);
	let menu = $derived(data.menu as any);
</script>

<section class="space-y-6">
	<div>
		<h1 class="text-xl font-bold">Hak akses (DEWA)</h1>
		<p class="muted text-xs">Kelola role, izin, menu, dan penugasan user. Hanya DEWA (rbac:manage).</p>
	</div>

	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">Role ({roles.length})</h2>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>Kode</th><th>Lingkup</th></tr></thead>
				<tbody>
					{#each roles as r (r.id)}
						<tr><td class="font-mono text-xs">{r.code}</td><td>{r.scope}</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">Izin ({permissions.length})</h2>
		<div class="flex flex-wrap gap-1.5">
			{#each permissions as p (p.code)}
				<span class="rounded-full border border-ink/10 px-2.5 py-1 text-[11px] font-mono">{p.code}</span>
			{/each}
		</div>
	</div>

	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">Menu ({menu.data?.length ?? 0})</h2>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>Menu</th><th>Href</th><th>Izin</th><th>Platform only</th></tr></thead>
				<tbody>
					{#each menu.data ?? [] as m (m.id)}
						<tr>
							<td>{m.label}</td>
							<td class="font-mono text-xs">{m.href}</td>
							<td class="font-mono text-xs">{m.permission_code ?? '—'}</td>
							<td>{m.platform_only ? 'Ya' : 'Tidak'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">User ({users.length})</h2>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>Email</th><th>Status</th><th>Role platform</th><th>Role merchant</th></tr></thead>
				<tbody>
					{#each users as u (u.id)}
						<tr>
							<td class="text-xs">{u.email}</td>
							<td>{u.status}</td>
							<td class="text-xs">{(u.platform_roles ?? []).join(', ') || '—'}</td>
							<td class="text-xs">{u.merchant_role || '—'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</section>