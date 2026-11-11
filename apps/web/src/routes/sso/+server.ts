import { dev } from '$app/env';
import { redirect } from '@sveltejs/kit';
import { ssoExchange, setSession } from '../../lib/server/auth';
import type { RequestHandler } from './$types';

// Tujuan masuk-sekali dari ARUS. Dibuka pengguna lewat klik (bukan tautan email), jadi GET aman;
// kode berlaku 60 detik dan hanya sekali. Gagal -> halaman login biasa.
export const GET: RequestHandler = async ({ url, fetch, cookies, getClientAddress }) => {
	const code = url.searchParams.get('code') ?? '';
	const s = code ? await ssoExchange(fetch, getClientAddress(), code) : null;
	if (!s) redirect(303, '/login');
	setSession(cookies, s.token, s.maxAge, !dev);
	redirect(303, '/dashboard/finance/settlement');
};
