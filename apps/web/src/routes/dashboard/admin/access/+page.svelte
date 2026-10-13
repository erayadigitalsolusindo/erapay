<script lang="ts">
	import { enhance } from '$app/forms';
	let { data, form } = $props();
	let tab = $state<'roles' | 'perms' | 'menus' | 'users'>('roles');

	let roles = $derived(data.roles as any[]);
	let permissions = $derived(data.permissions as any[]);
	let rolePermissions = $derived(data.rolePermissions as any[]);
	let users = $derived(data.users as any[]);
	let menu = $derived(data.menu as any);
	let menus = $derived((menu.data ?? []) as any[]);

	const hasPerm = (roleId: string, code: string) =>
		rolePermissions.some((p) => p.role_id === roleId && p.permission_code === code);
	const menuAllowed = (menuId: string, roleId: string) =>
		(menu.access ?? []).some((a: any) => a.menu_id === menuId && a.role_id === roleId && a.allowed === true);
</script>

<section class="space-y-6">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div>
			<h1 class="text-xl font-bold">Hak akses (DEWA)</h1>
			<p class="muted text-xs">Kelola role, izin, menu, dan user secara dinamis.</p>
		</div>
		<nav class="flex flex-wrap gap-1.5" aria-label="Tab hak akses">
			{#each ([['roles', 'Role'], ['perms', 'Matriks izin'], ['menus', 'Akses menu'], ['users', 'User']] as const) as [key, label] (key)}
				<button
					type="button"
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					aria-current={tab === key ? 'true' : undefined}
					onclick={() => (tab = key)}
				>
					{label}
				</button>
			{/each}
		</nav>
	</div>

	{#if form?.message}<p class="mb-2 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
	{#if form?.done}<p class="mb-2 text-xs font-semibold text-emerald-700" role="status">Perubahan tersimpan.</p>{/if}

	{#if tab === 'roles'}
		<div class="card p-5">
			<h2 class="mb-3 text-sm font-bold">Buat role</h2>
			<form method="POST" action="?/createRole" use:enhance class="flex flex-wrap items-center gap-2">
				<input class="inp w-48 py-2" name="code" placeholder="kode_role" required minlength="2" />
				<select class="inp w-36 py-2" name="scope" aria-label="Lingkup">
					<option value="platform">platform</option>
					<option value="merchant">merchant</option>
				</select>
				<button class="btn" type="submit">Simpan</button>
			</form>
		</div>

		<div class="card mt-5 overflow-hidden">
			<div class="p-5 pb-3"><h2 class="text-sm font-bold">Daftar role ({roles.length})</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>Kode</th><th>Lingkup</th><th>Aksi</th></tr></thead>
					<tbody>
						{#each roles as r (r.id)}
							<tr>
								<td><input class="inp w-48 py-2" name="code" value={r.code} form={`role-${r.id}`} required /></td>
								<td>
									<select class="inp w-32 py-2" name="scope" form={`role-${r.id}`} aria-label="Lingkup">
										<option value="platform" selected={r.scope === 'platform'}>platform</option>
										<option value="merchant" selected={r.scope === 'merchant'}>merchant</option>
									</select>
								</td>
								<td class="flex gap-2">
									<form id={`role-${r.id}`} method="POST" action="?/updateRole" use:enhance>
										<input type="hidden" name="id" value={r.id} />
										<button class="btn" type="submit">Ubah</button>
									</form>
									<form method="POST" action="?/deleteRole" use:enhance>
										<input type="hidden" name="id" value={r.id} />
										<button class="btn btn-danger" type="submit">Hapus</button>
									</form>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}

	{#if tab === 'perms'}
		<div class="card overflow-hidden">
			<div class="p-5"><h2 class="text-sm font-bold">Matriks izin</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr><th>Role / Izin</th>{#each permissions as p (p.code)}<th class="text-[10px]">{p.code}</th>{/each}</tr>
					</thead>
					<tbody>
						{#each roles as r (r.id)}
							<tr>
								<td class="font-mono text-xs">{r.code}</td>
								{#each permissions as p (p.code)}
									<td class="text-center">
										<form method="POST" action="?/togglePerm" use:enhance>
											<input type="hidden" name="role_id" value={r.id} />
											<input type="hidden" name="permission_code" value={p.code} />
											<input type="hidden" name="allowed" value={hasPerm(r.id, p.code) ? 'false' : 'true'} />
											<button
												class="size-7 rounded-md border text-xs font-bold"
												type="submit"
												aria-label={`${r.code} ${p.code}`}
												class:bg-emas={hasPerm(r.id, p.code)}
												class:text-black={hasPerm(r.id, p.code)}
												class:border-transparent={hasPerm(r.id, p.code)}
												class:hover:bg-ink={!hasPerm(r.id, p.code)}
											>
												{hasPerm(r.id, p.code) ? '×' : '+'}
											</button>
										</form>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}

	{#if tab === 'menus'}
		<div class="card overflow-hidden">
			<div class="p-5"><h2 class="text-sm font-bold">Akses menu</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr><th>Menu / Role</th>{#each roles as r (r.id)}<th class="text-[10px]">{r.code}</th>{/each}</tr>
					</thead>
					<tbody>
						{#each menus as m (m.id)}
							<tr>
								<td class="text-xs">{m.label}<div class="muted font-mono text-[10px]">{m.href}</div></td>
								{#each roles as r (r.id)}
									<td class="text-center">
										<form method="POST" action="?/toggleMenu" use:enhance>
											<input type="hidden" name="menu_id" value={m.id} />
											<input type="hidden" name="role_id" value={r.id} />
											<input type="hidden" name="allowed" value={menuAllowed(m.id, r.id) ? 'false' : 'true'} />
											<button
												class="size-7 rounded-md border text-xs font-bold"
												type="submit"
												aria-label={`${m.href} ${r.code}`}
												class:bg-emas={menuAllowed(m.id, r.id)}
												class:text-black={menuAllowed(m.id, r.id)}
												class:border-transparent={menuAllowed(m.id, r.id)}
												class:hover:bg-ink={!menuAllowed(m.id, r.id)}
											>
												{menuAllowed(m.id, r.id) ? '×' : '+'}
											</button>
										</form>
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}

	{#if tab === 'users'}
		<div class="card p-5">
			<h2 class="mb-3 text-sm font-bold">Assign user</h2>
			<form method="POST" action="?/assignUser" use:enhance class="grid gap-3 sm:grid-cols-4">
				<select class="inp w-full py-2" name="user_id" required aria-label="User">
					<option value="">Pilih user</option>
					{#each users as u (u.id)}
						<option value={u.id}>{u.email}</option>
					{/each}
				</select>
				<select class="inp w-full py-2" name="role_id" required aria-label="Role">
					<option value="">Pilih role</option>
					{#each roles as r (r.id)}
						<option value={r.id}>{r.code} ({r.scope})</option>
					{/each}
				</select>
				<input class="inp w-full py-2" name="merchant_id" placeholder="merchant_id (opsional)" />
				<button class="btn" type="submit">Assign</button>
			</form>
		</div>

		<div class="card mt-5 overflow-hidden">
			<div class="p-5 pb-3"><h2 class="text-sm font-bold">User ({users.length})</h2></div>
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
	{/if}
</section>