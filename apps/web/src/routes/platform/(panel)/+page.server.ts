import { guardPlatform, platformApi } from '../../../lib/server/platform';
import type { PageServerLoad } from './$types';

export type PlatformStats = {
	generated_at: string;
	db_latency_ms: number;
	users: { total: number; active: number; disabled: number; staff: number; mfa_enabled: number };
	merchants_by_status: Record<string, number>;
	rbac: { roles: number; permissions: number; grants: number; menus: number };
	sessions: { active: number; platform: number; impersonation: number; failed_logins_1h: number; failed_logins_24h: number };
	payments: { total_24h: number; succeeded_24h: number; failed_24h: number; pending: number; volume_24h: number };
	webhooks: { pending: number; dead_24h: number };
	audit: { events_24h: number; series_24h: { hour: string; count: number }[] | null; recent: { at: string; action: string; entity_type: string }[] | null };
};

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guardPlatform(locals, cookies);
	const r = await platformApi<PlatformStats>(fetch, token, 'GET', '/stats');
	return { stats: r.ok ? r.value : null, apiUp: r.ok || r.status !== 0 };
};
