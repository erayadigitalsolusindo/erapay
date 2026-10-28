import { fail } from '@sveltejs/kit';
import { guardPlatform, platformApi } from '../../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export type CredField = {
	key: string;
	label: string;
	hint?: string;
	secret: boolean;
	multiline: boolean;
	required: boolean;
	options?: string[];
	set: boolean;
	preview?: string;
};
export type CredEnv = {
	environment: 'sandbox' | 'production';
	version: number;
	configured: boolean;
	complete: boolean;
	updated_at?: string;
	fields: CredField[];
};
export type CredGateway = { gateway: string; label: string; fields: CredField[]; environments: CredEnv[] };
export type CredOverview = { server_environment: string; gateways: CredGateway[] };
export type CredStep = { name: string; ok: boolean; code?: string; message?: string };
export type RouteOption = { code: string; available: boolean; platform_account: boolean };
export type RoutingOverview = {
	server_environment: string;
	gateway: string;
	version: number;
	updated_at?: string;
	options: RouteOption[];
};

const GW = /^[a-z0-9_-]{1,32}$/;
const ENV = /^(sandbox|production)$/;

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guardPlatform(locals, cookies);
	const [r, rt] = await Promise.all([
		platformApi<CredOverview>(fetch, token, 'GET', '/gateway-credentials'),
		platformApi<RoutingOverview>(fetch, token, 'GET', '/payment-routing')
	]);
	// null = API belum punya fitur jalur (belum di-update): kartu pilihan jalur disembunyikan.
	const routing = rt.ok ? rt.value : null;
	if (!r.ok) {
		// 404 = server tanpa ERAPAY_ENCRYPTION_KEY (fitur ini mati); selain itu galat sementara.
		return { overview: null as CredOverview | null, routing, unavailable: r.status === 404, error: r.status === 404 ? null : r.message };
	}
	return { overview: r.value, routing, unavailable: false, error: null as string | null };
};

function target(f: FormData) {
	const gateway = String(f.get('gateway') ?? '');
	const environment = String(f.get('environment') ?? '');
	return GW.test(gateway) && ENV.test(environment) ? { gateway, environment } : null;
}

export const actions: Actions = {
	// Pilih gateway untuk transaksi BARU. Transaksi yang sudah dibuat tetap di gateway-nya.
	route: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const gateway = String(f.get('gateway') ?? '');
		const environment = String(f.get('environment') ?? '');
		if (!GW.test(gateway) || !ENV.test(environment)) return fail(400, { key: 'route', error: 'Gateway atau environment tidak valid.' });
		const note = String(f.get('note') ?? '').trim();
		if (note.length < 3) return fail(400, { key: 'route', error: 'Alasan wajib diisi (minimal 3 karakter).' });
		const r = await platformApi(fetch, token, 'PUT', '/payment-routing', {
			environment,
			gateway,
			version: Number(f.get('version') ?? 0) || 0,
			note
		});
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { key: 'route', error: r.message });
		return { key: 'route', routed: gateway };
	},

	// Simpan (merge) kredensial. Field kosong = pertahankan nilai tersimpan; "clear:<key>" = hapus field opsional.
	save: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const t = target(f);
		if (!t) return fail(400, { error: 'Gateway atau environment tidak valid.' });
		const key = `${t.gateway}/${t.environment}`;
		const values: Record<string, string> = {};
		const clear: string[] = [];
		for (const [k, v] of f.entries()) {
			if (k.startsWith('v:')) values[k.slice(2)] = String(v);
			else if (k.startsWith('clear:') && v === '1') clear.push(k.slice(6));
		}
		const note = String(f.get('note') ?? '').trim();
		if (note.length < 3) return fail(400, { key, error: 'Alasan wajib diisi (minimal 3 karakter).' });
		const r = await platformApi(fetch, token, 'PUT', `/gateway-credentials/${t.gateway}/${t.environment}`, {
			values,
			clear,
			version: Number(f.get('version') ?? 0) || 0,
			note
		});
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { key, error: r.message });
		return { key, saved: true };
	},

	// Uji kredensial tersimpan ke gateway (tanpa mengubah adapter yang sedang dipakai).
	test: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const t = target(await request.formData());
		if (!t) return fail(400, { error: 'Gateway atau environment tidak valid.' });
		const key = `${t.gateway}/${t.environment}`;
		const r = await platformApi<{ ok: boolean; steps: CredStep[] }>(fetch, token, 'POST', `/gateway-credentials/${t.gateway}/${t.environment}/test`, {});
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { key, error: r.message });
		return { key, result: { ok: r.value.ok, applied: false, steps: r.value.steps } };
	},

	// Terapkan ke server tanpa restart; hanya berhasil bila tes koneksi lulus.
	apply: async ({ request, cookies, locals, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const t = target(await request.formData());
		if (!t) return fail(400, { error: 'Gateway atau environment tidak valid.' });
		const key = `${t.gateway}/${t.environment}`;
		const r = await platformApi<{ applied: boolean; ok: boolean; steps: CredStep[] }>(fetch, token, 'POST', `/gateway-credentials/${t.gateway}/${t.environment}/apply`, {});
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { key, error: r.message });
		return { key, result: { ok: r.value.ok, applied: r.value.applied, steps: r.value.steps } };
	}
};
