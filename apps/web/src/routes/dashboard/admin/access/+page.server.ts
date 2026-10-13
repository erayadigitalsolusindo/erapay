import { redirect } from '@sveltejs/kit';
import { guard, read } from '../../../../lib/server/gw';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'rbac:manage', 'platform');
	const menu = await read<any>(fetch, token, '/v1/admin/menu-permissions');
	const roles = await read<{ data: any[] }>(fetch, token, '/v1/admin/roles');
	const permissions = await read<{ data: any[] }>(fetch, token, '/v1/admin/permissions');
	const users = await read<{ data: any[] }>(fetch, token, '/v1/admin/users');
	if (!menu || !roles || !permissions) redirect(303, '/dashboard');
	return {
		menu: menu ?? { data: [], roles: [], access: [] },
		roles: roles?.data ?? [],
		permissions: permissions?.data ?? [],
		users: users?.data ?? []
	};
};