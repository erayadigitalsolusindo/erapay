<script lang="ts">
	import Combobox from '../../../../lib/components/Combobox.svelte';
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import { withConfirm } from '../../../../lib/ui/confirm';
	import { notify } from '../../../../lib/ui/dialog.svelte';

	let { data, form } = $props();
	const i18n = getI18n();
	const { t, tx } = i18n;

	type Tab = 'users' | 'roles' | 'perms' | 'menus';
	let tab = $state<Tab>('users');
	let q = $state('');
	let filter = $state<'all' | 'active' | 'disabled' | 'platform' | 'tenant'>('all');

	const roles = $derived(data.roles as any[]);
	const permissions = $derived(data.permissions as any[]);
	const rolePermissions = $derived(data.rolePermissions as any[]);
	const users = $derived(data.users as any[]);
	const menu = $derived(data.menu as any);
	const menus = $derived((menu.data ?? []) as any[]);
	const access = $derived((menu.access ?? []) as any[]);

	const SCOPES = $derived([
		{ value: 'platform', label: t('pf2.ac.scope.platform') },
		{ value: 'merchant', label: t('pf2.ac.scope.merchant') }
	]);

	// Indeks cepat: sel matriks dicek ratusan kali per render.
	const grantSet = $derived(new Set(rolePermissions.map((p) => `${p.role_id}|${p.permission_code}`)));
	const menuOn = $derived(new Set(access.filter((a) => a.allowed).map((a) => `${a.menu_id}|${a.role_id}`)));
	const menuOk = $derived(new Set(access.filter((a) => a.assignable).map((a) => `${a.menu_id}|${a.role_id}`)));

	const permCount = (rid: string) => rolePermissions.filter((p) => p.role_id === rid).length;
	const menuCount = (rid: string) => access.filter((a) => a.role_id === rid && a.allowed).length;
	const memberCount = (code: string) => users.filter((u) => (u.platform_roles ?? []).includes(code) || u.merchant_role === code).length;

	const scopePill = (s: string) => (s === 'platform' ? 'bg-merah/10 text-merah' : 'bg-emas/25 text-ink');
	const when = (s?: string) => (s ? new Date(s).toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' }) : '');
	const day = (s?: string) => (s ? new Date(s).toLocaleDateString(i18n.locale, { dateStyle: 'medium' }) : '');
	const initials = (e: string) => e.slice(0, 2).toUpperCase();

	const shownUsers = $derived(
		users.filter((u) => {
			const hay = `${u.email} ${(u.platform_roles ?? []).join(' ')} ${u.merchant_role ?? ''}`.toLowerCase();
			if (q && !hay.includes(q.toLowerCase())) return false;
			if (filter === 'active') return u.status === 'active';
			if (filter === 'disabled') return u.status !== 'active';
			if (filter === 'platform') return (u.platform_roles ?? []).length > 0;
			if (filter === 'tenant') return !!u.merchant_role;
			return true;
		})
	);

	// Izin dikelompokkan per sumber daya (bagian sebelum ":"), mis. payout:read → grup "payout".
	function groupBy<T>(items: T[], key: (x: T) => string) {
		const m = new Map<string, T[]>();
		for (const it of items) {
			const k = key(it) || t('pf2.ac.group.other');
			(m.get(k) ?? m.set(k, []).get(k)!).push(it);
		}
		return [...m.entries()];
	}
	const permGroups = $derived(
		groupBy(
			permissions.filter((p) => !q || p.code.toLowerCase().includes(q.toLowerCase())),
			(p) => (p.code.includes(':') ? p.code.split(':')[0] : '')
		)
	);
	const permShown = $derived(permGroups.reduce((n, [, a]) => n + a.length, 0));
	const menuShown = $derived(menus.filter((m) => !q || `${tx(m.label, m.label)} ${m.href}`.toLowerCase().includes(q.toLowerCase())));

	// Hasil action → toast. Form tidak di-reset agar isian lain tidak hilang.
	const act = (okMsg?: string) => () => async ({ result, update }: { result: any; update: (o?: { reset?: boolean }) => Promise<void> }) => {
		if (result.type === 'failure') notify('danger', result.data?.message ?? t('pf2.ac.loadFail'));
		else if (result.type === 'success' && okMsg) notify('success', okMsg);
		await update({ reset: result.type === 'success' && !!okMsg ? true : false });
	};

	const tabs: [Tab, string][] = [
		['users', 'pf2.ac.tab.users'],
		['roles', 'pf2.ac.tab.roles'],
		['perms', 'pf2.ac.tab.perms'],
		['menus', 'pf2.ac.tab.menus']
	];
</script>

<svelte:head><title>{t('pf2.ac.title')} · EraPay</title></svelte:head>

<section class="space-y-5">
	<header class="card relative overflow-hidden p-5">
		<div class="pointer-events-none absolute inset-y-0 right-0 w-1/3 bg-gradient-to-l from-merah/5 to-transparent"></div>
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div>
				<div class="font-mono text-[10px] tracking-[.18em] text-merah uppercase">rbac · iam</div>
				<h1 class="font-display mt-1 text-xl font-extrabold">{t('pf2.ac.title')}</h1>
				<p class="muted mt-1 max-w-2xl text-xs leading-relaxed">{t('pf2.ac.sub')}</p>
			</div>
			<div class="font-mono text-[11px] text-ink/70">
				{t('pf2.ac.stat', { u: users.length, r: roles.length, p: permissions.length, g: rolePermissions.length })}
			</div>
		</div>
		<div class="mt-5 flex flex-wrap gap-1 border-b border-ink/10" role="tablist">
			{#each tabs as [key, label] (key)}
				<button
					type="button"
					role="tab"
					aria-selected={tab === key}
					class="-mb-px border-b-2 border-transparent px-4 py-2 text-xs font-bold text-ink/60 transition hover:text-ink aria-selected:border-merah aria-selected:text-merah"
					onclick={() => {
						tab = key;
						q = '';
					}}
				>
					{t(label as any)}
				</button>
			{/each}
		</div>
	</header>

	{#if data.error}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{t('pf2.ac.loadFail')}</p>{/if}
	{#if form?.message}<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{form.message}</p>{/if}

	<!-- ============ IDENTITAS ============ -->
	{#if tab === 'users'}
		<div class="card p-5">
			<div class="flex flex-wrap items-baseline justify-between gap-2">
				<h2 class="text-sm font-bold">{t('pf2.ac.assign')}</h2>
				<p class="muted text-[11px]">{t('pf2.ac.assignHint')}</p>
			</div>
			<form method="POST" action="?/assignUser" use:enhance={act(t('pf2.ac.saved'))} class="mt-3 grid gap-3 md:grid-cols-[1fr_1fr_1fr_auto]">
				<Combobox id="assign-user" name="user_id" required placeholder={t('pf2.ac.pickUser')} options={users.map((u) => ({ value: u.id, label: u.email }))} />
				<Combobox id="assign-role" name="role_id" required placeholder={t('pf2.ac.pickRole')} options={roles.map((r) => ({ value: r.id, label: `${r.code} · ${t(`pf2.ac.scope.${r.scope}` as any)}` }))} />
				<input class="inp font-mono text-xs" name="merchant_id" placeholder={t('pf2.ac.merchantPh')} autocomplete="off" />
				<button class="btn btn-red" type="submit">{t('pf2.ac.assignBtn')}</button>
			</form>
		</div>

		<div class="card overflow-hidden">
			<div class="flex flex-wrap items-center gap-3 border-b border-ink/10 p-4">
				<input class="inp max-w-xs py-2 font-mono text-xs" placeholder={t('pf2.ac.search')} bind:value={q} aria-label={t('pf2.ac.search')} />
				<div class="flex flex-wrap gap-1">
					{#each ['all', 'active', 'disabled', 'platform', 'tenant'] as f (f)}
						<button
							type="button"
							class="rounded-md border border-ink/10 px-2.5 py-1 font-mono text-[10px] font-bold tracking-wide uppercase hover:bg-ink/5 aria-pressed:border-merah aria-pressed:bg-merah aria-pressed:text-white"
							aria-pressed={filter === f}
							onclick={() => (filter = f as typeof filter)}
						>
							{f === 'all' ? t('pf2.ac.all') : f}
						</button>
					{/each}
				</div>
				<span class="ml-auto font-mono text-[11px] text-ink/60">{shownUsers.length}/{users.length}</span>
			</div>
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr>
							<th>{t('pf2.ac.col.user')}</th><th>{t('pf2.ac.col.roles')}</th><th>{t('pf2.ac.col.mfa')}</th><th>{t('pf2.ac.col.sess')}</th><th>{t('pf2.ac.col.last')}</th><th>{t('pf2.ac.col.created')}</th><th class="text-right">{t('pf2.ac.col.actions')}</th>
						</tr>
					</thead>
					<tbody>
						{#each shownUsers as u (u.id)}
							{@const self = u.id === data.me}
							<tr class="hover:bg-ink/[.03]">
								<td>
									<div class="flex items-center gap-3">
										<span class="relative grid size-8 shrink-0 place-items-center rounded-lg bg-ink/10 font-mono text-[11px] font-bold">
											{initials(u.email)}
											<span class="absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full border-2 border-surface {u.status === 'active' ? 'bg-emerald-500' : 'bg-neutral-400'}" title={tx(`user.st.${u.status}`, u.status)}></span>
										</span>
										<div class="min-w-0">
											<div class="flex items-center gap-1.5 font-mono text-xs font-bold">
												{u.email}
												{#if self}<span class="rounded bg-merah px-1.5 py-0.5 text-[9px] text-white">{t('pf2.ac.you')}</span>{/if}
											</div>
											<div class="muted font-mono text-[10px]">{u.id}</div>
										</div>
									</div>
								</td>
								<td>
									<div class="flex flex-wrap gap-1">
										{#each u.platform_roles ?? [] as r (r)}<span class="pill bg-merah/10 font-mono text-merah">{r}</span>{/each}
										{#if u.merchant_role}<span class="pill bg-emas/25 font-mono text-ink" title={u.merchant_id}>{u.merchant_role}</span>{/if}
										{#if !(u.platform_roles ?? []).length && !u.merchant_role}<span class="muted">—</span>{/if}
									</div>
								</td>
								<td>
									<span class="pill font-mono {u.mfa_enabled ? 'bg-emerald-500/15 text-emerald-700' : 'bg-ink/10 text-ink/60'}">{u.mfa_enabled ? t('pf2.ac.mfaOn') : t('pf2.ac.mfaOff')}</span>
								</td>
								<td class="font-mono">{u.active_sessions ?? 0}</td>
								<td class="font-mono text-[11px]">{u.last_login_at ? when(u.last_login_at) : t('pf2.ac.never')}</td>
								<td class="font-mono text-[11px]">{day(u.created_at)}</td>
								<td class="text-right">
									<form
										method="POST"
										action="?/deleteUser"
										use:enhance={withConfirm((fd) => ({
											variant: 'danger',
											title: t('pf2.ac.delTitle'),
											message: t('pf2.ac.delMsg', { email: String(fd.get('email') ?? '') }),
											confirmText: t('pf2.ac.delConfirm'),
											successMessage: t('pf2.ac.delOk')
										}))}
									>
										<input type="hidden" name="id" value={u.id} />
										<input type="hidden" name="email" value={u.email} />
										<button class="btn btn-danger py-1.5" type="submit" disabled={self} title={self ? t('pf2.ac.delGuard') : undefined}>{t('pf2.ac.delete')}</button>
									</form>
								</td>
							</tr>
						{:else}
							<tr><td colspan="7" class="muted p-8 text-center">{t('pf2.sec.none')}</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
			<p class="muted border-t border-ink/10 p-3 text-[11px]">{t('pf2.ac.delGuard')}</p>
		</div>
	{/if}

	<!-- ============ PERAN ============ -->
	{#if tab === 'roles'}
		<div class="card p-5">
			<h2 class="text-sm font-bold">{t('pf2.ac.roleCreate')}</h2>
			<form method="POST" action="?/createRole" use:enhance={act(t('pf2.ac.saved'))} class="mt-3 flex flex-wrap items-center gap-2">
				<input class="inp w-72 font-mono text-xs" name="code" placeholder={t('pf2.ac.codePh')} required minlength="2" autocomplete="off" />
				<Combobox id="new-role-scope" name="scope" class="w-40" value="platform" options={SCOPES} />
				<button class="btn btn-red" type="submit">{t('pf2.ac.roleAdd')}</button>
			</form>
		</div>

		<div class="card overflow-hidden">
			<div class="overflow-x-auto">
				<table>
					<thead>
						<tr><th>{t('pf2.ac.col.code')}</th><th>{t('pf2.ac.col.scope')}</th><th>{t('pf2.ac.col.perms')}</th><th>{t('pf2.ac.col.menus')}</th><th>{t('pf2.ac.col.members')}</th><th class="text-right">{t('pf2.ac.col.actions')}</th></tr>
					</thead>
					<tbody>
						{#each roles as r (r.id)}
							<tr class="hover:bg-ink/[.03]">
								<td><input class="inp w-56 py-1.5 font-mono text-xs" name="code" value={r.code} form={`role-${r.id}`} required aria-label={t('pf2.ac.col.code')} /></td>
								<td><Combobox id={`role-scope-${r.id}`} name="scope" form={`role-${r.id}`} class="w-36" value={r.scope} options={SCOPES} /></td>
								<td>
									<div class="flex items-center gap-2">
										<div class="h-1.5 w-24 overflow-hidden rounded-full bg-ink/10"><div class="h-full rounded-full bg-merah" style:width={`${permissions.length ? (permCount(r.id) / permissions.length) * 100 : 0}%`}></div></div>
										<span class="font-mono text-[11px]">{t('pf2.ac.perm.count', { n: permCount(r.id), t: permissions.length })}</span>
									</div>
								</td>
								<td class="font-mono text-[11px]">{menuCount(r.id)}</td>
								<td class="font-mono text-[11px]">{memberCount(r.code)}</td>
								<td>
									<div class="flex justify-end gap-2">
										<form id={`role-${r.id}`} method="POST" action="?/updateRole" use:enhance={act(t('pf2.ac.saved'))}>
											<input type="hidden" name="id" value={r.id} />
											<button class="btn btn-light py-1.5" type="submit">{t('pf2.ac.roleSave')}</button>
										</form>
										<form
											method="POST"
											action="?/deleteRole"
											use:enhance={withConfirm({
												variant: 'danger',
												title: t('pf2.ac.roleDelTitle'),
												message: t('pf2.ac.roleDelMsg', { code: r.code }),
												confirmText: t('pf2.ac.delete'),
												successMessage: t('pf2.ac.saved')
											})}
										>
											<input type="hidden" name="id" value={r.id} />
											<button class="btn btn-danger py-1.5" type="submit">{t('pf2.ac.delete')}</button>
										</form>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}

	<!-- ============ MATRIKS IZIN ============ -->
	{#if tab === 'perms'}
		<div class="card overflow-hidden">
			<div class="flex flex-wrap items-center gap-3 border-b border-ink/10 p-4">
				<input class="inp max-w-sm py-2 font-mono text-xs" placeholder={t('pf2.ac.perm.search')} bind:value={q} aria-label={t('pf2.ac.perm.search')} />
				<span class="font-mono text-[11px] text-ink/60">{t('pf2.ac.perm.rows', { n: permShown })}</span>
				<span class="muted ml-auto flex items-center gap-3 text-[11px]">
					<span class="flex items-center gap-1.5"><span class="grid size-4 place-items-center rounded bg-emerald-500 text-[10px] font-bold text-white">✓</span>{t('pf2.ac.perm.granted')}</span>
					<span class="flex items-center gap-1.5"><span class="size-4 rounded border border-ink/20"></span>{t('pf2.ac.perm.denied')}</span>
				</span>
			</div>
			<form method="POST" action="?/togglePerm" use:enhance={act()}>
				<div class="max-h-[70vh] overflow-auto">
					<table class="min-w-full">
						<thead>
							<tr>
								<th class="sticky top-0 left-0 z-30 min-w-64">{t('pf2.ac.perm.col')}</th>
								{#each roles as r (r.id)}
									<th class="sticky top-0 z-20 text-center normal-case">
										<div class="font-mono text-[11px] font-bold text-ink">{r.code}</div>
										<span class="pill mt-1 text-[9px] uppercase {scopePill(r.scope)}">{t(`pf2.ac.scope.${r.scope}` as any)}</span>
										<div class="mt-1 font-mono text-[10px] font-normal text-ink/60">{t('pf2.ac.perm.count', { n: permCount(r.id), t: permissions.length })}</div>
									</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							{#each permGroups as [group, items] (group)}
								<tr>
									<td colspan={roles.length + 1} class="sticky left-0 border-t border-ink/10 bg-ink/[.04] py-2 font-mono text-[10px] font-bold tracking-[.16em] text-merah uppercase">
										{group} <span class="font-normal text-ink/50">· {items.length}</span>
									</td>
								</tr>
								{#each items as p (p.code)}
									<tr class="group hover:bg-ink/[.03]">
										<td class="sticky left-0 z-10 bg-surface font-mono text-[11px] group-hover:bg-page">{p.code}</td>
										{#each roles as r (r.id)}
											{@const on = grantSet.has(`${r.id}|${p.code}`)}
											<td class="text-center">
												<button
													name="cell"
													value={`${r.id}|${p.code}|${on ? 'false' : 'true'}`}
													type="submit"
													role="checkbox"
													aria-checked={on}
													aria-label={`${r.code} · ${p.code}`}
													class="mx-auto grid size-6 place-items-center rounded border text-[11px] font-bold transition {on ? 'border-emerald-600 bg-emerald-500 text-white hover:bg-emerald-600' : 'border-ink/20 text-transparent hover:border-merah hover:text-merah/60'}"
												>
													✓
												</button>
											</td>
										{/each}
									</tr>
								{/each}
							{:else}
								<tr><td colspan={roles.length + 1} class="muted p-8 text-center">{t('pf2.sec.none')}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			</form>
			<p class="muted border-t border-ink/10 p-3 text-[11px]">{t('pf2.ac.perm.legend')}</p>
		</div>
	{/if}

	<!-- ============ AKSES MENU ============ -->
	{#if tab === 'menus'}
		<div class="card overflow-hidden">
			<div class="flex flex-wrap items-center gap-3 border-b border-ink/10 p-4">
				<input class="inp max-w-sm py-2 font-mono text-xs" placeholder={t('pf2.ac.menu.search')} bind:value={q} aria-label={t('pf2.ac.menu.search')} />
				<span class="font-mono text-[11px] text-ink/60">{menuShown.length}/{menus.length}</span>
			</div>
			<form method="POST" action="?/toggleMenu" use:enhance={act()}>
				<div class="max-h-[70vh] overflow-auto">
					<table class="min-w-full">
						<thead>
							<tr>
								<th class="sticky top-0 left-0 z-30 min-w-64">{t('pf2.ac.menu.col')}</th>
								{#each roles as r (r.id)}
									<th class="sticky top-0 z-20 text-center normal-case">
										<div class="font-mono text-[11px] font-bold text-ink">{r.code}</div>
										<span class="pill mt-1 text-[9px] uppercase {scopePill(r.scope)}">{t(`pf2.ac.scope.${r.scope}` as any)}</span>
										<div class="mt-1 font-mono text-[10px] font-normal text-ink/60">{menuCount(r.id)}/{menus.length}</div>
									</th>
								{/each}
							</tr>
						</thead>
						<tbody>
							{#each menuShown as m (m.id)}
								<tr class="group hover:bg-ink/[.03]">
									<td class="sticky left-0 z-10 bg-surface group-hover:bg-page">
										<div class="text-xs font-bold">{tx(m.label, m.label)}</div>
										<div class="muted font-mono text-[10px]">{m.href}</div>
									</td>
									{#each roles as r (r.id)}
										{@const on = menuOn.has(`${m.id}|${r.id}`)}
										<td class="text-center">
											{#if !menuOk.has(`${m.id}|${r.id}`)}
												<span class="text-ink/25" title={t('pf2.ac.menu.legend')}>—</span>
											{:else}
												<button
													name="cell"
													value={`${m.id}|${r.id}|${on ? 'false' : 'true'}`}
													type="submit"
													role="checkbox"
													aria-checked={on}
													aria-label={`${m.href} · ${r.code}`}
													class="mx-auto grid size-6 place-items-center rounded border text-[11px] font-bold transition {on ? 'border-emerald-600 bg-emerald-500 text-white hover:bg-emerald-600' : 'border-ink/20 text-transparent hover:border-merah hover:text-merah/60'}"
												>
													✓
												</button>
											{/if}
										</td>
									{/each}
								</tr>
							{:else}
								<tr><td colspan={roles.length + 1} class="muted p-8 text-center">{t('pf2.sec.none')}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			</form>
			<p class="muted border-t border-ink/10 p-3 text-[11px]">{t('pf2.ac.menu.legend')}</p>
		</div>
	{/if}
</section>
