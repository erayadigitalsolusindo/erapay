import { redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../lib/server/auth';
import { read } from '../../lib/server/gw';
import type { Payment } from '../../lib/server/payments';
import type { PageServerLoad } from './$types';

type Summary = {
	from: string;
	to: string;
	timezone: string;
	totals: {
		payments: number;
		succeeded: number;
		pending: number;
		failed: number;
		expired: number;
		cancelled: number;
		unknown: number;
		gross: number;
		mdr_fee: number;
		platform_fee: number;
		merchant_net: number;
		refunded: number;
	};
	daily: { date: string; count: number; gross: number }[];
};
type Balance = { available: number; pending: number; payable: number };

const PERIODS = [7, 30, 90];

/** Tanggal hari ini di WIB (YYYY-MM-DD), sama dengan zona yang dipakai laporan di Go. */
const wibToday = () => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());
const addDays = (iso: string, n: number) => {
	const d = new Date(`${iso}T00:00:00Z`);
	d.setUTCDate(d.getUTCDate() + n);
	return d.toISOString().slice(0, 10);
};

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const u = locals.user;
	const token = cookies.get(SESSION_COOKIE);
	if (!u || !token) redirect(303, '/login');
	if (!u.merchantId) return { platform: true as const };

	const asked = Number(url.searchParams.get('days'));
	const days = PERIODS.includes(asked) ? asked : 30;
	const to = wibToday();
	const from = addDays(to, -(days - 1));
	const canReport = u.permissions.includes('report:read');
	const canPayments = u.permissions.includes('payment:read');

	const [summary, balance, recent] = await Promise.all([
		canReport ? read<Summary>(fetch, token, `/v1/reports/summary?from=${from}&to=${to}`) : Promise.resolve(null),
		canReport ? read<Balance>(fetch, token, '/v1/balance') : Promise.resolve(null),
		canPayments ? read<{ data: Payment[] }>(fetch, token, '/v1/payments?limit=5') : Promise.resolve(null)
	]);
	return {
		platform: false as const,
		days,
		periods: PERIODS,
		canReport,
		canPayments,
		summary,
		balance,
		recent: recent?.data ?? null
	};
};
