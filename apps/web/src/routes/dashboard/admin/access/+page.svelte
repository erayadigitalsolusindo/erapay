<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';

	const i18n = getI18n();
	const { t } = i18n;
	let { data, form } = $props();

	let roles = $derived(data.roles as any[]);
	let permissions = $derived(data.permissions as any[]);
	let rolePerms = $derived(data.rolePerms as any[]);
	let users = $derived(data.users as any[]);
	let menu = $derived(data.menu as any);

	let roleById = $derived(new Map(roles.map((r) => [r.id, r])));
	let permSet = $derived(new Set(rolePerms.map((rp) => `${rp.role_id}:${rp.permission_code}`)));
	let accessMap = $derived(new Map((menu.access ?? []).map((a: any) => [`${a.menu_id}:${a.role_id}`, a.allowed])));

	let editingRole = $state<string | null>(null);
	let editCode = $state('');
	let editScope = $state('');

	function startEdit(r: any) {
		editingRole = r.id;
		editCode = r.code;
		editScope = r.scope;
	}
	function cancelEdit() {
		editingRole = null;
	}

	const hasPerm = (roleId: string, code: string) => permSet.has(`${roleId}:${code}`);
	const isAllowed = (menuId: string, roleId: string) => accessMap.get(`${menuId}:${roleId}`) ?? false;
	const platformRoles = $derived(roles.filter((r) => r.scope === 'platform'));
	const merchantRoles = $derived(roles.filter((r) => r.scope === 'merchant'));
</script>

<section class="space-y-6">
	<div>
		<h1 class="text-xl font-bold">{t('nav.access')}</h1>
		<p class="muted text-xs">{t('dewa.sub')}</p>
	</div>

	{#if form?.message}<p class="text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}

	<!-- ==================== ROLE ==================== -->
	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">{t('dewa.role')} ({roles.length})</h2>

		<form method="POST" action="?/createRole" class="mb-4 flex flex-wrap items-end gap-3" use:enhance>
			<div>
				<label class="lbl mb-1.5" for="role-code">{t('dewa.role.code')}</label>
				<input id="role-code" class="inp" name="code" maxlength="60" required placeholder={t('dewa.role.codePh')} />
			</div>
			<div>
				<label class="lbl mb-1.5" for="role-scope">{t('dewa.role.scope')}</label>
				<select id="role-scope" class="inp" name="scope" required>
					<option value="platform">{t('dewa.role.scope.platform')}</option>
					<option value="merchant">{t('dewa.role.scope.merchant')}</option>
				</select>
			</div>
			<button type="submit" class="btn btn-red">{t('dewa.role.create')}</button>
		</form>

		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('dewa.role.code')}</th><th>{t('dewa.role.scope')}</th><th>{t('c.actions')}</th></tr></thead>
				<tbody>
					{#each roles as r (r.id)}
						{#if editingRole === r.id}
							<tr>
								<td><input class="inp" name="code" bind:value={editCode} maxlength="60" required form="edit-role-{r.id}" /></td>
								<td>
									<select class="inp" name="scope" bind:value={editScope} required form="edit-role-{r.id}">
										<option value="platform">{t('dewa.role.scope.platform')}</option>
										<option value="merchant">{t('dewa.role.scope.merchant')}</option>
									</select>
								</td>
								<td>
									<div class="flex items-center gap-2">
										<form method="POST" action="?/updateRole" id="edit-role-{r.id}" use:enhance>
											<input type="hidden" name="id" value={r.id} />
										</form>
										<button type="submit" form="edit-role-{r.id}" class="btn btn-red">{t('dewa.role.save')}</button>
										<button type="button" class="btn btn-light" onclick={cancelEdit}>{t('dewa.role.cancel')}</button>
									</div>
								</td>
							</tr>
						{:else}
							<tr>
								<td class="font-mono text-xs">{r.code}</td>
								<td>{t(r.scope === 'platform' ? 'dewa.role.scope.platform' : 'dewa.role.scope.merchant')}</td>
								<td>
									<div class="flex items-center gap-2">
										<button type="button" class="btn btn-light" onclick={() => startEdit(r)}>{t('dewa.role.edit')}</button>
										<form
											method="POST"
											action="?/deleteRole"
											use:enhance={({ cancel }) => {
												if (!confirm(t('dewa.role.deleteConfirm', { code: r.code }))) cancel();
											}}
										>
											<input type="hidden" name="id" value={r.id} />
											<button type="submit" class="btn btn-danger">{t('dewa.role.delete')}</button>
										</form>
									</div>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<!-- ==================== PERMISSION ==================== -->
	<div class="card p-5">
		<h2 class="mb-1 text-sm font-bold">{t('dewa.perm')}</h2>
		<p class="muted mb-3 text-xs">{t('dewa.perm.hint')}</p>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('dewa.role.code')}</th>{#each permissions as p (p.code)}<th class="text-center font-mono text-[11px] font-normal">{p.code}</th>{/each}</tr></thead>
				<tbody>
					{#each roles as r (r.id)}
						<tr>
							<td class="font-mono text-xs">{r.code}</td>
							{#each permissions as p (p.code)}
								<td class="text-center">
									<form method="POST" action={hasPerm(r.id, p.code) ? '?/revokePermission' : '?/grantPermission'} use:enhance>
										<input type="hidden" name="role_id" value={r.id} />
										<input type="hidden" name="permission_code" value={p.code} />
										<button
											type="submit"
											class="size-5 rounded border border-ink/20 accent-merah"
											role="checkbox"
											aria-checked={hasPerm(r.id, p.code) ? 'true' : 'false'}
											aria-label={`${r.code} ${p.code}`}
										></button>
									</form>
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<!-- ==================== MENU ACCESS ==================== -->
	<div class="card p-5">
		<h2 class="mb-1 text-sm font-bold">{t('dewa.menu')}</h2>
		<p class="muted mb-3 text-xs">{t('dewa.menu.hint')}</p>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>Menu</th><th>Href</th>{#each roles as r (r.id)}<th class="text-center font-mono text-[11px] font-normal">{r.code}</th>{/each}</tr></thead>
				<tbody>
					{#each menu.data ?? [] as m (m.id)}
						<tr>
							<td class="text-xs">{m.label}</td>
							<td class="font-mono text-xs">{m.href}</td>
							{#each roles as r (r.id)}
								<td class="text-center">
									<form method="POST" action="?/setMenuAccess" use:enhance>
										<input type="hidden" name="menu_id" value={m.id} />
										<input type="hidden" name="role_id" value={r.id} />
										<input type="hidden" name="allowed" value={isAllowed(m.id, r.id) ? '0' : '1'} />
										<button
											type="submit"
											class="size-5 rounded border border-ink/20 accent-merah"
											role="checkbox"
											aria-checked={isAllowed(m.id, r.id) ? 'true' : 'false'}
											aria-label={`${m.label} ${r.code}`}
										></button>
									</form>
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>

	<!-- ==================== USER ==================== -->
	<div class="card p-5">
		<h2 class="mb-3 text-sm font-bold">{t('dewa.user')} ({users.length})</h2>
		<div class="overflow-x-auto">
			<table>
				<thead><tr><th>{t('dewa.user.email')}</th><th>{t('dewa.user.status')}</th><th>{t('dewa.user.platform')}</th><th>{t('dewa.user.merchant')}</th><th></th></tr></thead>
				<tbody>
					{#each users as u (u.id)}
						<tr>
							<td class="text-xs">{u.email}</td>
							<td class="text-xs">{u.status}</td>
							<td>
								{#if !u.merchant_id}
									<form method="POST" action="?/setUserRoles" class="flex flex-wrap items-center gap-2" use:enhance>
										<input type="hidden" name="user_id" value={u.id} />
										{#each platformRoles as r (r.id)}
											<label class="muted flex items-center gap-1 text-[11px]">
												<input type="checkbox" name="role_ids" value={r.id} checked={(u.platform_roles ?? []).includes(r.code)} class="accent-merah" />
												{r.code}
											</label>
										{/each}
										<button type="submit" class="btn btn-light">{t('dewa.user.save')}</button>
									</form>
								{:else}
									<span class="muted text-xs">—</span>
								{/if}
							</td>
							<td>
								{#if u.merchant_id}
									<form method="POST" action="?/setUserRoles" class="flex items-center gap-2" use:enhance>
										<input type="hidden" name="user_id" value={u.id} />
										<input type="hidden" name="merchant_id" value={u.merchant_id} />
										<select name="role_ids" class="inp max-w-40 px-2 py-1.5 text-xs">
											<option value="">—</option>
											{#each merchantRoles as r (r.id)}
												<option value={r.id} selected={u.merchant_role === r.code}>{r.code}</option>
											{/each}
										</select>
										<button type="submit" class="btn btn-light">{t('dewa.user.save')}</button>
									</form>
								{:else}
									<span class="muted text-xs">—</span>
								{/if}
							</td>
							<td></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
</section>
