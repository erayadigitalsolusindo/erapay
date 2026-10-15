import { guardPlatform, platformApi } from '../../../../lib/server/platform';
import type { PageServerLoad } from './$types';

type Row = {
	id: string;
	actor_email: string | null;
	action: string;
	entity_type: string;
	entity_id: string | null;
	metadata: Record<string, unknown>;
	request_id: string | null;
	created_at: string;
};

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guardPlatform(locals, cookies);
	const prefix = url.searchParams.get('prefix') ?? 'impersonation.';
	const merchant = url.searchParams.get('merchant_id')?.trim() ?? '';
	const before = url.searchParams.get('before') ?? '';
	const qs = new URLSearchParams({ prefix, limit: '100' });
	if (merchant) qs.set('merchant_id', merchant);
	if (before) qs.set('before', before);
	const r = await platformApi<{ data: Row[] }>(fetch, token, 'GET', `/audit-logs?${qs}`);
	const rows = r.ok ? r.value.data ?? [] : [];
	return {
		prefix,
		merchant,
		rows,
		next: rows.length === 100 ? rows[rows.length - 1].id : null,
		error: r.ok ? null : r.message
	};
};
