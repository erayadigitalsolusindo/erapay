<script lang="ts">
	import Combobox from '../../../../lib/components/Combobox.svelte';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	let { data, form } = $props();
	const { t, tx } = getI18n();
	const SCOPES = $derived([
		{ value: 'platform', label: t('dewa.role.scope.platform') },
		{ value: 'merchant', label: t('dewa.role.scope.merchant') }
	]);
	const scopeName = (s: string) => tx(`dewa.role.scope.${s}`, s);
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
	const menuAssignable = (menuId: string, roleId: string) =>
		(menu.access ?? []).some((a: any) => a.menu_id === menuId && a.role_id === roleId && a.assignable === true);
</script>

<section class="space-y-6">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<div>
			<h1 class="text-xl font-bold">{t('nav.access')}</h1>
			<p class="muted text-xs">{t('pf.ac.sub')}</p>
		</div>
		<nav class="flex flex-wrap gap-1.5" aria-label={t('pf.ac.tabs')}>
			{#each ([['roles', 'pf.ac.tab.roles'], ['perms', 'pf.ac.tab.perms'], ['menus', 'pf.ac.tab.menus'], ['users', 'pf.ac.tab.users']] as const) as [key, label] (key)}
				<button
					type="button"
					class="rounded-full border border-ink/10 px-3 py-1 text-[11px] font-bold hover:bg-ink/5 aria-[current=true]:bg-merah aria-[current=true]:text-white"
					aria-current={tab === key ? 'true' : undefined}
					onclick={() => (tab = key)}
				>
					{t(label)}
				</button>
			{/each}
		</nav>
	</div>

	{#if data.error}<p class="mb-2 text-xs font-semibold text-red-700" role="alert">{t('pf.ac.loadFail')}</p>{/if}
	{#if form?.message}<p class="mb-2 text-xs font-semibold text-red-700" role="alert">{form.message}</p>{/if}
	{#if form?.done}<p class="mb-2 text-xs font-semibold text-emerald-700" role="status">{t('pf.ac.saved')}</p>{/if}

	{#if tab === 'roles'}
		<div class="card p-5">
			<h2 class="mb-3 text-sm font-bold">{t('dewa.role.create')}</h2>
			<form method="POST" action="?/createRole" use:enhance class="flex flex-wrap items-center gap-2">
				<input class="inp w-48 py-2" name="code" placeholder={t('pf.ac.codePh')} required minlength="2" />
				<Combobox id="new-role-scope" name="scope" class="w-40" value="platform" options={SCOPES} />
				<button class="btn" type="submit">{t('dewa.role.save')}</button>
			</form>
		</div>

		<div class="card mt-5 overflow-hidden">
			<div class="p-5 pb-3"><h2 class="text-sm font-bold">{t('pf.ac.roleList', { n: roles.length })}</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>{t('pf.ac.col.code')}</th><th>{t('pf.ac.col.scope')}</th><th>{t('pf.ac.col.actions')}</th></tr></thead>
					<tbody>
						{#each roles as r (r.id)}
							<tr>
								<td><input class="inp w-48 py-2" name="code" value={r.code} form={`role-${r.id}`} required /></td>
								<td>
									<Combobox id={`role-scope-${r.id}`} name="scope" form={`role-${r.id}`} class="w-40" value={r.scope} options={SCOPES} />
								</td>
								<td class="flex gap-2">
									<form id={`role-${r.id}`} method="POST" action="?/updateRole" use:enhance>
										<input type="hidden" name="id" value={r.id} />
										<button class="btn" type="submit">{t('dewa.role.edit')}</button>
									</form>
									<form method="POST" action="?/deleteRole" use:enhance>
										<input type="hidden" name="id" value={r.id} />
										<button class="btn btn-danger" type="submit">{t('dewa.role.delete')}</button>
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
			<div class="p-5"><h2 class="text-sm font-bold">{t('pf.ac.permMatrix')}</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr><th>{t('pf.ac.rolePerm')}</th>{#each permissions as p (p.code)}<th class="text-[10px]">{p.code}</th>{/each}</tr>
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
			<div class="p-5"><h2 class="text-sm font-bold">{t('pf.ac.tab.menus')}</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr><th>{t('pf.ac.menuRole')}</th>{#each roles as r (r.id)}<th class="text-[10px]">{r.code}</th>{/each}</tr>
					</thead>
					<tbody>
						{#each menus as m (m.id)}
							<tr>
								<td class="text-xs">{tx(m.label, m.label)}<div class="muted font-mono text-[10px]">{m.href}</div></td>
								{#each roles as r (r.id)}
									<td class="text-center">
										{#if !menuAssignable(m.id, r.id)}
											<span class="muted text-xs" title={t('pf.ac.scopeMismatch')}>—</span>
										{:else}
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
										{/if}
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
			<h2 class="mb-3 text-sm font-bold">{t('pf.ac.assign')}</h2>
			<form method="POST" action="?/assignUser" use:enhance class="grid gap-3 sm:grid-cols-4">
				<Combobox id="assign-user" name="user_id" required placeholder={t('pf.ac.pickUser')} options={users.map((u) => ({ value: u.id, label: u.email }))} />
				<Combobox id="assign-role" name="role_id" required placeholder={t('pf.ac.pickRole')} options={roles.map((r) => ({ value: r.id, label: `${r.code} (${scopeName(r.scope)})` }))} />
				<input class="inp w-full py-2" name="merchant_id" placeholder={t('pf.ac.merchantPh')} />
				<button class="btn" type="submit">{t('pf.ac.assignBtn')}</button>
			</form>
		</div>

		<div class="card mt-5 overflow-hidden">
			<div class="p-5 pb-3"><h2 class="text-sm font-bold">{t('pf.ac.userList', { n: users.length })}</h2></div>
			<div class="overflow-x-auto">
				<table>
					<thead><tr><th>{t('dewa.user.email')}</th><th>{t('dewa.user.status')}</th><th>{t('dewa.user.platform')}</th><th>{t('dewa.user.merchant')}</th></tr></thead>
					<tbody>
						{#each users as u (u.id)}
							<tr>
								<td class="text-xs">{u.email}</td>
								<td>{tx(`user.st.${u.status}`, u.status)}</td>
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
