import { fail } from '@sveltejs/kit';
import { call, done, guard, read, str } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type Endpoint = { id: string; url: string; status: string; secret_hint: string; created_at: string; secret?: string };
export type Delivery = {
	id: string;
	endpoint_id: string;
	event_type: string;
	status: string;
	attempts: number;
	next_attempt_at: string | null;
	last_http_status: number | null;
	last_error: string | null;
	redelivery_of: string | null;
	created_at: string;
};

const STATUSES = ['pending', 'succeeded', 'dead', 'cancelled'];

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guard(locals, cookies, 'webhook:manage', 'merchant');
	const status = url.searchParams.get('status') ?? '';
	const q = STATUSES.includes(status) ? `&status=${status}` : '';
	const [endpoints, deliveries] = await Promise.all([
		read<{ data: Endpoint[] }>(fetch, token, '/v1/webhook-endpoints'),
		read<{ data: Delivery[] }>(fetch, token, `/v1/webhook-deliveries?limit=50${q}`)
	]);
	return { endpoints: endpoints?.data ?? null, deliveries: deliveries?.data ?? null, status };
};

export const actions: Actions = {
	create: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'webhook:manage', 'merchant');
		const url = str(await request.formData(), 'url');
		if (!url.startsWith('https://')) return fail(400, { message: 'URL harus diawali https://' });
		const r = await call<Endpoint>(fetch, token, 'POST', '/v1/webhook-endpoints', { url });
		// Secret hanya ada di respons ini; tidak disimpan di server web.
		return r.ok ? { created: r.value } : fail(400, { message: r.message });
	},
	rotate: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'webhook:manage', 'merchant');
		const id = str(await request.formData(), 'id');
		const r = await call<Endpoint>(fetch, token, 'POST', `/v1/webhook-endpoints/${encodeURIComponent(id)}/rotate-secret`);
		return r.ok ? { created: r.value } : fail(400, { message: r.message });
	},
	disable: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'webhook:manage', 'merchant');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/webhook-endpoints/${encodeURIComponent(id)}/disable`));
	},
	redeliver: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'webhook:manage', 'merchant');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/webhook-deliveries/${encodeURIComponent(id)}/redeliver`));
	}
};
