// Menu & judul halaman panel admin. Hanya halaman yang benar-benar ada (jangan tambah tautan mati).
import type { Key } from '../../../lib/i18n/messages';

// platform: true = menu staf EraPay (Finance/Ops, tanpa merchant); selain itu menu merchant.
export type NavItem = { href: string; icon: string; label: Key; desc: Key; permission?: string; platform?: boolean };

export const nav: NavItem[] = [
	{
		href: '/dashboard',
		icon: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
		label: 'nav.overview',
		desc: 'nav.overview.desc'
	},
	{
		href: '/dashboard/transactions',
		icon: '<path d="M4 7h16M4 12h16M4 17h10"/><circle cx="19" cy="17" r="3"/>',
		label: 'nav.transactions',
		desc: 'nav.transactions.desc'
	},
	{
		href: '/dashboard/api-keys',
		icon: '<path d="m8 8-4 4 4 4m8-8 4 4-4 4m-3-11-2 14"/>',
		label: 'nav.apiKeys',
		desc: 'nav.apiKeys.desc',
		permission: 'api_key:manage'
	},
	{
		href: '/dashboard/payouts',
		icon: '<rect x="3" y="6" width="18" height="13" rx="2"/><path d="M3 10h18M7 15h3"/>',
		label: 'nav.payouts',
		desc: 'nav.payouts.desc',
		permission: 'payout:read'
	},
	{
		href: '/dashboard/refunds',
		icon: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
		label: 'nav.refunds',
		desc: 'nav.refunds.desc',
		permission: 'refund:read'
	},
	{
		href: '/dashboard/webhooks',
		icon: '<path d="M18 16.98h-5.99c-1.1 0-1.95.94-2.48 1.9A4 4 0 0 1 2 17c.01-.7.2-1.4.57-2"/><path d="m6 17 3.13-5.78c.53-.97.1-2.18-.5-3.1a4 4 0 1 1 6.89-4.06"/><path d="m12 6 3.13 5.73C15.66 12.7 16.9 13 18 13a4 4 0 0 1 0 8"/>',
		label: 'nav.webhooks',
		desc: 'nav.webhooks.desc',
		permission: 'webhook:manage'
	},
	{
		href: '/dashboard/finance/settlement',
		icon: '<path d="M3 6h18M3 12h18M3 18h12"/><path d="m17 16 2 2 4-4"/>',
		label: 'nav.fSettlement',
		desc: 'nav.fSettlement.desc',
		permission: 'settlement:read',
		platform: true
	},
	{
		href: '/dashboard/finance/payouts',
		icon: '<rect x="3" y="6" width="18" height="13" rx="2"/><path d="m9 13 2 2 4-4"/>',
		label: 'nav.fPayouts',
		desc: 'nav.fPayouts.desc',
		permission: 'payout:review',
		platform: true
	},
	{
		href: '/dashboard/finance/refunds',
		icon: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
		label: 'nav.fRefunds',
		desc: 'nav.fRefunds.desc',
		permission: 'refund:review',
		platform: true
	},
	{
		href: '/dashboard/finance/merchants',
		icon: '<path d="M4 9l1.5-5h13L20 9"/><path d="M4 9v11h16V9"/><path d="m9 15 2 2 4-4"/>',
		label: 'nav.fMerchants',
		desc: 'nav.fMerchants.desc',
		permission: 'merchant:review',
		platform: true
	}
];

type NavUser = { merchantId: string; permissions?: string[] };

/** Menu yang boleh dilihat: merchant melihat menu merchant, staf platform (tanpa merchant) melihat menu platform. */
export function visible(user: NavUser): NavItem[] {
	return nav.filter((n) => {
		if (!n.permission) return !n.platform || !user.merchantId;
		if (!user.permissions?.includes(n.permission)) return false;
		return n.platform ? !user.merchantId : !!user.merchantId;
	});
}

/** Item aktif = prefix terpanjang yang cocok dengan path. */
export function aktif(pathname: string): NavItem {
	return (
		[...nav].sort((a, b) => b.href.length - a.href.length).find((n) => pathname === n.href || pathname.startsWith(n.href + '/')) ??
		nav[0]
	);
}
