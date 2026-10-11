declare global {
	namespace App {
		interface Locals {
			/** Diisi hooks.server.ts. null = belum login. */
			user: { id: string; name: string; role: string } | null;
			requestId: string;
		}
	}
}
export {};
