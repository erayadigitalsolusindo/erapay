import { fail } from '@sveltejs/kit';
import { guardPlatform, platformApi } from '../../../../lib/server/platform';
import { done, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guardPlatform(locals, cookies);
	const [menu, roles, permissions, rolePermissions, users] = await Promise.all([
		platformApi<any>(fetch, token, 'GET', '/admin/menu-permissions'),
		platformApi<{ data: any[] }>(fetch, token, 'GET', '/admin/roles'),
		platformApi<{ data: any[] }>(fetch, token, 'GET', '/admin/permissions'),
		platformApi<{ data: any[] }>(fetch, token, 'GET', '/admin/role-permissions'),
		platformApi<{ data: any[] }>(fetch, token, 'GET', '/admin/users')
	]);
	return {
		me: locals.platformUser?.id ?? '',
		error: menu.ok && roles.ok && permissions.ok && users.ok ? null : 'Gagal memuat data hak akses.',
		menu: (menu.ok ? menu.value : null) ?? { data: [], roles: [], access: [] },
		roles: roles.ok ? roles.value.data ?? [] : [],
		permissions: permissions.ok ? permissions.value.data ?? [] : [],
		rolePermissions: rolePermissions.ok ? rolePermissions.value.data ?? [] : [],
		users: users.ok ? users.value.data ?? [] : []
	};
};

export const actions: Actions = {
	// --- Roles ---
	createRole: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		return done(await platformApi<any>(fetch, token, 'POST', '/admin/roles', { code: str(f, 'code'), scope: str(f, 'scope') }));
	},
	updateRole: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		return done(
			await platformApi<any>(fetch, token, 'PUT', `/admin/roles/${encodeURIComponent(str(f, 'id'))}`, {
				code: str(f, 'code'),
				scope: str(f, 'scope')
			})
		);
	},
	deleteRole: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		return done(await platformApi<void>(fetch, token, 'DELETE', `/admin/roles/${encodeURIComponent(str(f, 'id'))}`, {}));
	},

	// --- Matriks permission ---
	togglePerm: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		// Satu form untuk seluruh matriks: tiap sel mengirim "role_id|permission_code|allowed".
		const [role_id, permission_code, allowed] = str(f, 'cell').split('|');
		if (!role_id || !permission_code) return fail(400, { message: 'Sel tidak valid.' });
		return done(await platformApi<void>(fetch, token, allowed === 'true' ? 'POST' : 'DELETE', '/admin/role-permissions', { role_id, permission_code }));
	},
	// --- Matriks menu ---
	toggleMenu: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		// Satu form untuk seluruh matriks: tiap sel mengirim "menu_id|role_id|allowed".
		const [menu_id, role_id, allowed] = str(f, 'cell').split('|');
		if (!menu_id || !role_id) return fail(400, { message: 'Sel tidak valid.' });
		return done(await platformApi<void>(fetch, token, 'PUT', '/admin/menu-permissions', { items: [{ menu_id, role_id, allowed: allowed === 'true' }] }));
	},

	// --- Assign user ---
	assignUser: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const role = str(f, 'role_id');
		if (!str(f, 'user_id') || !role) return fail(400, { message: 'Pilih user dan role.' });
		return done(
			await platformApi<void>(fetch, token, 'PUT', `/admin/users/${encodeURIComponent(str(f, 'user_id'))}/roles`, {
				merchant_id: str(f, 'merchant_id'),
				role_ids: [role]
			})
		);
	},

	// --- Hapus user + seluruh relasinya (transaksi di Go; ditolak bila punya jejak keuangan) ---
	deleteUser: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const id = str(f, 'id');
		if (!id) return fail(400, { message: 'User tidak valid.' });
		return done(await platformApi<any>(fetch, token, 'DELETE', `/admin/users/${encodeURIComponent(id)}`, {}), { deleted: true });
	}
};
