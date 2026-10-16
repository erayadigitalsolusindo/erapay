import { error, fail } from '@sveltejs/kit';
import { guardPlatform, platformApi } from '../../../../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export type GwAccount = {
	id: string;
	gateway: string;
	environment: 'sandbox' | 'production';
	gateway_ref?: string;
	activation_status: 'pending_confirmation' | 'active' | 'suspended';
	created_at: string;
};
export type GwOverview = { server_environment: string; gateways: string[]; accounts: GwAccount[] };
export type GwStep = { name: string; ok: boolean; code?: string; message?: string };
export type GwTest = { ok: boolean; deep: boolean; steps: GwStep[] };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const load: PageServerLoad = async ({ locals, cookies, fetch, params, url }) => {
	const token = guardPlatform(locals, cookies);
	if (!UUID.test(params.id)) error(404, 'Tenant tidak ditemukan');
	const r = await platformApi<GwOverview>(fetch, token, 'GET', `/merchants/${params.id}/gateway-accounts`);
	if (!r.ok) {
		if (r.status === 404) error(404, 'Tenant tidak ditemukan');
		return { id: params.id, name: url.searchParams.get('n') ?? '', overview: null as GwOverview | null, error: r.message };
	}
	return { id: params.id, name: url.searchParams.get('n') ?? '', overview: r.value, error: null as string | null };
};

export const actions: Actions = {
	// Buat/ubah akun gateway. Mengaktifkan butuh gateway_ref yang sudah dikonfirmasi di dashboard gateway.
	save: async ({ request, cookies, locals, fetch, params }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const body = {
			gateway: String(f.get('gateway') ?? ''),
			environment: String(f.get('environment') ?? ''),
			gateway_ref: String(f.get('gateway_ref') ?? '').trim(),
			status: String(f.get('status') ?? ''),
			note: String(f.get('note') ?? '').trim()
		};
		if (body.note.length < 3) return fail(400, { saveError: 'Alasan wajib diisi (minimal 3 karakter).' });
		const r = await platformApi(fetch, token, 'PUT', `/merchants/${params.id}/gateway-accounts`, body);
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { saveError: r.message });
		return { saved: true };
	},

	// Tes koneksi. deep=true membuat QR uji Rp1.000 di gateway lalu langsung menutupnya (tidak ada uang bergerak).
	test: async ({ request, cookies, locals, fetch, params }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const gateway = String(f.get('gateway') ?? '');
		const environment = String(f.get('environment') ?? '');
		const deep = f.get('deep') === '1';
		const r = await platformApi<GwTest>(fetch, token, 'POST', `/merchants/${params.id}/gateway-accounts/test`, { gateway, environment, deep });
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { testError: r.message, tested: `${gateway}/${environment}` });
		return { test: r.value, tested: `${gateway}/${environment}` };
	}
};
