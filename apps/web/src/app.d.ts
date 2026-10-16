declare global {
	namespace App {
		interface Locals {
			/** Diisi hooks.server.ts dari Go API (/v1/auth/me). null = belum login. */
			user: { id: string; email: string; fullName: string; merchantId: string; emailVerified: boolean; permissions: string[]; impersonating?: boolean; impersonationReason?: string } | null;
			/** Sesi panel platform DEWA (/v1/platform/auth/me). null = belum login panel. */
			platformUser: { id: string; email: string; emailVerified: boolean; permissions: string[] } | null;
			requestId: string;
		}
	}
}
export {};
