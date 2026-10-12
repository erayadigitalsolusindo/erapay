declare global {
	namespace App {
		interface Locals {
			/** Diisi hooks.server.ts dari Go API (/v1/auth/me). null = belum login. */
			user: { id: string; email: string; merchantId: string; emailVerified: boolean; permissions: string[] } | null;
			requestId: string;
		}
	}
}
export {};
