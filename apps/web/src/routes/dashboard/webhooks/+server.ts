import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

// Digabung ke halaman Integrasi.
export const GET: RequestHandler = () => redirect(308, '/dashboard/integrasi?tab=webhooks');
