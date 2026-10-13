import { redirect } from '@sveltejs/kit';
import { done, guard, read, call, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'rbac:manage', 'platform');
	const [menu, roles, permissions, rolePerms, users] = await Promise.all([
		read<any>(fetch, token, '/v1/admin/menu-permissions'),
		read<{ data: any[] }>(fetch, token, '/v1/admin/roles'),
		read<{ data: any[] }>(fetch, token, '/v1/admin/permissions'),
		read<{ data: any[] }>(fetch, token, '/v1/admin/role-permissions'),
		read<{ data: any[] }>(fetch, token, '/v1/admin/users')
	]);
	if (!menu || !roles || !permissions || !rolePerms) redirect(303, '/dashboard');
	return {
		menu: menu ?? { data: [], roles: [], access: [] },
		roles: roles?.data ?? [],
		permissions: permissions?.data ?? [],
		rolePerms: rolePerms?.data ?? [],
		users: users?.data ?? []
	};
};

export const actions: Actions = {
	createRole: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'POST', '/v1/admin/roles', {
			code: str(f, 'code'),
			scope: str(f, 'scope')
		});
		return done(r);
	},
	updateRole: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'PUT', `/v1/admin/roles/${encodeURIComponent(str(f, 'id'))}`, {
			code: str(f, 'code') || undefined,
			scope: str(f, 'scope') || undefined
		});
		return done(r);
	},
	deleteRole: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'DELETE', `/v1/admin/roles/${encodeURIComponent(id)}`));
	},
	grantPermission: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'POST', '/v1/admin/role-permissions', {
			role_id: str(f, 'role_id'),
			permission_code: str(f, 'permission_code')
		});
		return done(r);
	},
	revokePermission: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'DELETE', '/v1/admin/role-permissions', {
			role_id: str(f, 'role_id'),
			permission_code: str(f, 'permission_code')
		});
		return done(r);
	},
	setMenuAccess: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'PUT', '/v1/admin/menu-permissions', {
			items: [{ menu_id: str(f, 'menu_id'), role_id: str(f, 'role_id'), allowed: str(f, 'allowed') === '1' }]
		});
		return done(r);
	},
	setUserRoles: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'rbac:manage', 'platform');
		const f = await request.formData();
		const r = await call(fetch, token, 'PUT', `/v1/admin/users/${encodeURIComponent(str(f, 'user_id'))}/roles`, {
			merchant_id: str(f, 'merchant_id') || undefined,
			role_ids: f.getAll('role_ids').map(String)
		});
		return done(r);
	}
};
