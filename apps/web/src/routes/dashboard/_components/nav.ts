// Menu & judul halaman panel admin. Hanya halaman yang benar-benar ada (jangan tambah tautan mati).
import type { Key } from '../../../lib/i18n/messages';

export type NavItem = { href: string; icon: string; label: Key; desc: Key; permission?: string };

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
	}
];

/** Item aktif = prefix terpanjang yang cocok dengan path. */
export function aktif(pathname: string): NavItem {
	return (
		[...nav].sort((a, b) => b.href.length - a.href.length).find((n) => pathname === n.href || pathname.startsWith(n.href + '/')) ??
		nav[0]
	);
}
