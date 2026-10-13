import { fail, redirect } from '@sveltejs/kit';
import { guard, read, call, done, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'rbac:manage', 'platform');
	const menu = await read<any>(fetch, token, '/v1/admin/menu-permissions');
	const roles = await read<{ data: any[] }>(fetch, token, '/v1/admin/roles');
	const permissions = await read<{ data: any[] }>(fetch, token, '/v1/admin/permissions');
	const rolePermissions = await read<{ data: any[] }>(fetch, token, '/v1/admin/role-permissions');
	const users = await read<{ data: any[] }>(fetch, token, '/v1/admin/users');
	if (!menu || !roles || !permissions) redirect(303, '/dashboard');
	return {
		menu: menu ?? { data: [], roles: [], access: [] },
		roles: roles?.data ?? [],
		permissions: permissions?.data ?? [],
		rolePermissions: rolePermissions?.data ?? [],
		users: users?.data ?? []
	};
};

export const actions: Actions = {
	// --- Roles ---
	createRole: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		return done(
			await call<any>(fetch, token, 'POST', '/v1/admin/roles', {
				code: str(f, 'code'),
				scope: str(f, 'scope')
			})
		);
	},
	updateRole: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		return done(
			await call<any>(fetch, token, 'PUT', `/v1/admin/roles/${encodeURIComponent(str(f, 'id'))}`, {
				code: str(f, 'code'),
				scope: str(f, 'scope')
			})
		);
	},
	deleteRole: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		return done(
			await call<void>(fetch, token, 'DELETE', `/v1/admin/roles/${encodeURIComponent(str(f, 'id'))}`, {})
		);
	},

	// --- Matriks permission ---
	togglePerm: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const body = { role_id: str(f, 'role_id'), permission_code: str(f, 'permission_code') };
		return done(await call<void>(fetch, token, str(f, 'allowed') === 'true' ? 'POST' : 'DELETE', '/v1/admin/role-permissions', body));
	},
	// --- Matriks menu ---
	toggleMenu: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		return done(
			await call<void>(fetch, token, 'PUT', '/v1/admin/menu-permissions', {
				items: [{ menu_id: str(f, 'menu_id'), role_id: str(f, 'role_id'), allowed: str(f, 'allowed') === 'true' }]
			})
		);
	},

	// --- Assign user ---
	assignUser: async ({ request, cookies, locals, fetch }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const role = str(f, 'role_id');
		if (!str(f, 'user_id') || !role) return fail(400, { message: 'Pilih user dan role.' });
		return done(
			await call<void>(fetch, token, 'PUT', `/v1/admin/users/${encodeURIComponent(str(f, 'user_id'))}/roles`, {
				merchant_id: str(f, 'merchant_id'),
				role_ids: [role]
			})
		);
	}
};
