import { redirect } from '@sveltejs/kit';
import { isLang } from '../../lib/i18n/messages';
import { read } from '../../lib/server/gw';
import { SESSION_COOKIE } from '../../lib/server/auth';
import type { LayoutServerLoad } from './$types';

// Penjaga semua rute di bawah /dashboard. Tema & bahasa dibaca dari cookie agar SSR tanpa kedip.
export const load: LayoutServerLoad = async ({ locals, cookies, fetch }) => {
	if (!locals.user) redirect(303, '/login');
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	// Merchant yang belum disetujui melihat banner status. Hanya peran dengan merchant:read yang bisa membaca
	// profil; bila gagal dimuat, banner tidak ditampilkan (Go tetap menolak transaksi untuk merchant non-aktif).
	let onboarding: { status: string; note: string | null } | null = null;
	const token = cookies.get(SESSION_COOKIE);
	// Daftar menu yang benar-benar terlihat dihitung dari menu_role_access (DEWA), bukan hardcode.
	let menus: string[] | null = null;
	if (token) {
		const m = await read<{ data?: string[] }>(fetch, token, '/v1/menus');
		menus = m?.data ?? null;
	}
	// Mode Sandbox/Live sesi ini. Di sandbox, menu payout & refund disembunyikan: persetujuannya oleh Finance
	// tidak mencakup merchant sandbox, jadi halamannya tidak akan berfungsi.
	type SandboxMode = { available: boolean; enabled: boolean; active: boolean };
	let sandbox: SandboxMode | null = null;
	if (token && locals.user.merchantId && locals.user.permissions.includes('merchant:read')) {
		sandbox = await read<SandboxMode>(fetch, token, '/v1/sandbox');
		if (sandbox?.active && menus) menus = menus.filter((h) => h !== '/dashboard/payouts' && h !== '/dashboard/refunds');
	}
	if (token && locals.user.merchantId && locals.user.permissions.includes('merchant:read')) {
		const p = await read<{ onboarding_status: string; review_note: string | null }>(fetch, token, '/v1/merchant/profile');
		if (p && p.onboarding_status !== 'approved') onboarding = { status: p.onboarding_status, note: p.review_note };
	}
	return {
		onboarding,
		menus,
		sandbox,
		user: locals.user,
		theme: (theme === 'dark' || theme === 'light' ? theme : 'system') as 'light' | 'dark' | 'system',
		lang: isLang(lang) ? lang : 'id'
	};
};
