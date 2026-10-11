<script lang="ts">
	import { enhance } from '$app/forms';

	let { form } = $props();
	let showPw = $state(false);
	let busy = $state(false);
	let shake = $state(false);

	$effect(() => {
		if (form?.error) {
			shake = true;
			const t = setTimeout(() => (shake = false), 500);
			return () => clearTimeout(t);
		}
	});
</script>

<svelte:head>
	<title>Masuk · EraPay</title>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,700;9..144,800&family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

<div class="page">
	<section class="hero">
		<div class="brand">
			<svg viewBox="0 0 32 32" width="44" height="44" aria-hidden="true">
				<defs>
					<linearGradient id="emas" x1="0" y1="0" x2="1" y2="1">
						<stop offset="0" stop-color="#fff3c4" /><stop offset=".5" stop-color="#e8b84a" /><stop offset="1" stop-color="#b8862a" />
					</linearGradient>
				</defs>
				<circle cx="16" cy="16" r="13" fill="url(#emas)" stroke="#8a0a1f" stroke-width="1.5" />
				<text x="16" y="21.6" text-anchor="middle" font-family="Fraunces, serif" font-weight="800" font-size="16" fill="#8a0a1f">E</text>
			</svg>
			<span>ERAPAY</span>
		</div>
		<h1>Bayar dengan <em>bangga</em>,<br />dari Sabang<br />sampai Merauke.</h1>
		<div class="gold"></div>
		<p>Terima pembayaran QRIS untuk usaha Anda. Cepat, aman, dan bisa dilacak sampai ke rupiah terakhir.</p>
	</section>

	<main class="panel">
		<form
			method="POST"
			class:shake
			novalidate
			use:enhance={() => {
				busy = true;
				return async ({ update }) => {
					await update({ reset: false });
					busy = false;
				};
			}}
		>
			<p class="eyebrow">Selamat datang kembali</p>
			<h2>Masuk ke akun Anda</h2>

			{#if form?.error}
				<div class="alert" role="alert">{form.error}</div>
			{/if}

			<label for="email">Email</label>
			<input
				id="email"
				name="email"
				type="email"
				autocomplete="username"
				placeholder="nama@usaha.co.id"
				value={form?.email ?? ''}
				required
				aria-invalid={form?.error ? 'true' : undefined}
			/>

			<label for="password">Kata sandi</label>
			<div class="pw">
				<input
					id="password"
					name="password"
					type={showPw ? 'text' : 'password'}
					autocomplete="current-password"
					placeholder="••••••••••••"
					required
					aria-invalid={form?.error ? 'true' : undefined}
				/>
				<button type="button" class="eye" onclick={() => (showPw = !showPw)} aria-label={showPw ? 'Sembunyikan kata sandi' : 'Tampilkan kata sandi'}>
					{showPw ? 'Sembunyikan' : 'Tampilkan'}
				</button>
			</div>

			{#if form?.needMerchant}
				<label for="merchant_id">ID merchant</label>
				<input id="merchant_id" name="merchant_id" type="text" autocomplete="off" placeholder="Tempel ID merchant" required />
			{/if}

			<button class="submit" type="submit" disabled={busy}>
				{busy ? 'Memproses…' : 'Masuk dengan aman'}
			</button>

			<p class="hint">Akun dibuat oleh admin EraPay. Hubungi tim kami bila Anda belum punya akses.</p>
		</form>
	</main>
</div>

<style>
	:global(body) { font-family: 'Plus Jakarta Sans', system-ui, sans-serif; background: #4a0510; }
	.page {
		min-height: 100vh;
		display: grid;
		grid-template-columns: 1fr;
		background:
			radial-gradient(60rem 30rem at 50% 110%, rgb(232 184 74 / 0.35), transparent 70%),
			linear-gradient(to bottom, #4a0510 0%, #8a0a1f 30%, #c8102e 70%, #ff7a7a 100%);
		color: #fff8f0;
		position: relative;
		overflow: hidden;
	}
	/* motif kawung bergerak pelan */
	.page::before {
		content: '';
		position: absolute;
		inset: 0;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='80' height='80' viewBox='0 0 80 80'%3E%3Cg fill='none' stroke='white' stroke-opacity='.14' stroke-width='1.5'%3E%3Cellipse cx='40' cy='20' rx='10' ry='18'/%3E%3Cellipse cx='40' cy='60' rx='10' ry='18'/%3E%3Cellipse cx='20' cy='40' rx='18' ry='10'/%3E%3Cellipse cx='60' cy='40' rx='18' ry='10'/%3E%3C/g%3E%3Ccircle cx='40' cy='40' r='3' fill='white' fill-opacity='.2'/%3E%3C/svg%3E");
		animation: batik 60s linear infinite;
		pointer-events: none;
	}
	.hero, .panel { position: relative; z-index: 1; }
	.hero { display: none; padding: 4rem 3rem 4rem 5rem; align-self: center; text-shadow: 0 2px 14px rgb(74 5 16 / 0.55); }
	.brand { display: flex; align-items: center; gap: 0.75rem; font-family: Fraunces, Georgia, serif; font-weight: 800; font-size: 1.5rem; letter-spacing: 0.18em; }
	h1 { font-family: Fraunces, Georgia, serif; font-weight: 800; font-size: clamp(2.2rem, 4vw, 3.2rem); line-height: 1.1; margin: 1.5rem 0 0; }
	h1 em { font-style: normal; color: #ffd9de; }
	.gold { margin-top: 1.5rem; width: 12rem; height: 2px; border-radius: 9999px; background: linear-gradient(90deg, transparent, #e8b84a, #fff3c4, #e8b84a, transparent); }
	.hero p { margin-top: 1.5rem; max-width: 28rem; font-size: 1.1rem; line-height: 1.8; opacity: 0.85; }

	.panel { display: grid; place-items: center; padding: 1.25rem; min-height: 100vh; }
	form {
		width: 100%;
		max-width: 26rem;
		box-sizing: border-box;
		background: #fffaf4;
		color: #262626;
		border-radius: 1.5rem;
		padding: 2.25rem 2rem;
		box-shadow: 0 25px 60px rgb(74 5 16 / 0.45), 0 0 0 1px rgb(232 184 74 / 0.5);
		animation: naik 0.7s cubic-bezier(0.2, 0.7, 0.2, 1) both;
	}
	form.shake { animation: getar 0.45s ease-in-out; }
	.eyebrow { margin: 0; font-size: 0.75rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; color: #c8102e; }
	h2 { margin: 0.25rem 0 1.5rem; font-family: Fraunces, Georgia, serif; font-weight: 800; font-size: 1.9rem; color: #4a0510; }
	label { display: block; margin: 1rem 0 0.375rem; font-size: 0.85rem; font-weight: 600; color: #404040; }
	input {
		width: 100%;
		box-sizing: border-box;
		padding: 0.8rem 1rem;
		font: inherit;
		font-size: 0.95rem;
		border: 1px solid #d4d4d4;
		border-radius: 0.75rem;
		background: #fff;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	input:focus { border-color: #c8102e; box-shadow: 0 0 0 4px rgb(200 16 46 / 0.15); }
	input[aria-invalid='true'] { border-color: #c8102e; }
	.pw { position: relative; }
	.pw input { padding-right: 6.5rem; }
	.eye { position: absolute; top: 0; right: 0.5rem; bottom: 0; background: none; border: 0; color: #737373; font: inherit; font-size: 0.8rem; font-weight: 600; cursor: pointer; }
	.eye:hover { color: #c8102e; }
	.submit {
		width: 100%;
		margin-top: 1.5rem;
		padding: 0.9rem 1rem;
		border: 0;
		border-radius: 0.75rem;
		font: inherit;
		font-weight: 700;
		color: #fff;
		background: linear-gradient(to bottom, #c8102e, #8a0a1f);
		box-shadow: 0 10px 24px rgb(200 16 46 / 0.35);
		cursor: pointer;
		transition: transform 0.15s, box-shadow 0.15s;
	}
	.submit:hover:not(:disabled) { transform: translateY(-1px); box-shadow: 0 14px 28px rgb(200 16 46 / 0.45); }
	.submit:disabled { opacity: 0.7; cursor: progress; }
	.alert { padding: 0.75rem 1rem; border-radius: 0.75rem; background: #fef2f2; border: 1px solid #fecaca; color: #991b1b; font-size: 0.875rem; }
	.hint { margin: 1.25rem 0 0; font-size: 0.8rem; color: #737373; text-align: center; line-height: 1.5; }
	button:focus-visible { outline: 3px solid #e8b84a; outline-offset: 2px; }

	@media (min-width: 64rem) {
		.page { grid-template-columns: 1.1fr 1fr; }
		.hero { display: block; }
	}
	@keyframes batik { to { background-position: 320px 320px; } }
	@keyframes naik { from { opacity: 0; transform: translateY(24px); } to { opacity: 1; transform: none; } }
	@keyframes getar { 0%, 100% { transform: translateX(0); } 20%, 60% { transform: translateX(-9px); } 40%, 80% { transform: translateX(9px); } }
	@media (prefers-reduced-motion: reduce) {
		.page::before, form, form.shake { animation: none; }
	}
</style>
