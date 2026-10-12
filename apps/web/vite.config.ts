import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';

// Origin publik web ini. Di belakang proxy (Apache/Nginx + Cloudflare) alamat diri sendiri
// diturunkan dari header Host/X-Forwarded-*, yang mudah salah; mendaftarkan origin di sini
// membuat pemeriksaan CSRF form tidak bergantung pada header proxy.
const trustedOrigins = ['https://erapay.erayadigital.co.id'];

export default defineConfig({
	plugins: [tailwindcss(), sveltekit({ adapter: adapter(), csrf: { trustedOrigins } })],
	test: { include: ['tests/**/*.test.ts', 'src/**/*.test.ts'] }
});
