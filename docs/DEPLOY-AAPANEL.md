# Deploy EraPay dengan aaPanel

Pola sama seperti ARUS: aaPanel hanya mengurus domain, SSL Let's Encrypt, dan reverse proxy. API Go dijalankan systemd dan tidak mengurus TLS. Update dengan satu perintah (`~/update-erapay.sh`).

| Domain | Isi | Proxy ke |
|---|---|---|
| `erapay-api.erayadigital.co.id` | API Go + webhook DOKU | `127.0.0.1:8181` |
| `erapay.erayadigital.co.id` | Web SvelteKit (`apps/web`) | port Node SvelteKit (menyusul) |

Port 8080 dipakai arus-api, jadi EraPay memakai **8181**.

## 1. DNS
A record kedua domain → IP server aaPanel. Jika lewat Cloudflare, pakai "DNS only" saat menerbitkan sertifikat.

## 2. Prasyarat server
- PostgreSQL (database `erapay` + user sendiri) dan Redis (wajib: API gagal start tanpa `ERAPAY_REDIS_URL`).
- Go sesuai `services/api/go.mod`, git, dan akses sudo.
- Postgres/Redis hanya di `127.0.0.1`.

```sql
CREATE USER erapay WITH PASSWORD 'GANTI_PASSWORD_KUAT';
CREATE DATABASE erapay OWNER erapay;
```

## 3. Site aaPanel
1. Website → Add site `erapay-api.erayadigital.co.id` (pure static, tanpa DB). Document root: `/www/wwwroot/erapay-api.erayadigital.co.id`.
2. Reverse proxy → target `http://127.0.0.1:8181`, send domain `$host`.
3. SSL → Let's Encrypt → apply → Force HTTPS. Port 80/443 terbuka (firewall aaPanel + provider).
4. Pastikan WAF/rate limit tidak memblokir `/webhooks/`.

## 4. Env + kunci (sekali, jangan di-commit)
Di document root, `chmod 600` dan `chown www:www`:

`erapay.env` (format systemd `KEY=value`, tanpa `export`; PEM multi-baris tidak didukung, pakai `DOKU_PRIVATE_KEY_FILE`):
```
ERAPAY_ENV=sandbox
ERAPAY_HTTP_ADDR=127.0.0.1:8181
ERAPAY_DATABASE_URL=postgres://erapay:PASSWORD@127.0.0.1:5432/erapay?sslmode=disable
ERAPAY_REDIS_URL=redis://127.0.0.1:6379/0
ERAPAY_TRUSTED_PROXIES=127.0.0.1/32,::1/128
# sementara, untuk menangkap payload webhook DOKU asli; matikan setelah selesai
ERAPAY_WEBHOOK_CAPTURE=1

DOKU_CLIENT_ID=...
DOKU_SECRET_KEY=...
DOKU_PRIVATE_KEY_FILE=/www/wwwroot/erapay-api.erayadigital.co.id/doku-private.pem
DOKU_TERMINAL_ID=...
DOKU_POSTAL_CODE=...
DOKU_QRIS_STATUS_MAP_CONFIRMED=false
```
`ERAPAY_ENV=production` mewajibkan `DOKU_BASE_URL` diisi. Daftar env lengkap: `.env.example`.

## 5. Setup script (sekali)
```bash
git clone <repo-erapay> ~/erapay-src
cd ~/erapay-src && git checkout <branch>
cp deploy/update-erapay.sh ~/ && chmod +x ~/update-erapay.sh
sudo cp deploy/aapanel/erapay-api.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable erapay-api
```

## 6. Deploy / update
```bash
~/update-erapay.sh
```
Alur: `git pull --ff-only` → build (`deploy/build-release.sh`, binary `erapay-api`/`-migrate`/`-bootstrap`) → backup binary lama (5 terakhir) → `erapay-migrate up` → pasang → restart → cek `/readyz` (rollback binary otomatis jika gagal; migrasi tidak di-rollback).

Pada deploy pertama, jika service belum jalan: `sudo systemctl start erapay-api`.

Log: `journalctl -u erapay-api -f`. Jangan jalankan `migrate down`/`reset` di server.

Admin pertama (tidak ada signup publik):
```bash
cd /www/wwwroot/erapay-api.erayadigital.co.id
set -a; . ./erapay.env; set +a
ERAPAY_BOOTSTRAP_EMAIL=... ERAPAY_BOOTSTRAP_PASSWORD='...' ./bin/erapay-bootstrap platform-admin
```

## 7. Uji webhook
```bash
curl -i -X POST https://erapay-api.erayadigital.co.id/webhooks/doku -d '{"ping":1}'
psql "$ERAPAY_DATABASE_URL" -c "select id, created_at from webhook_captures order by id desc limit 1;"
```
Harus 200 dan menambah satu baris.

## 8. Daftarkan di DOKU
Notification URL (Back Office, sandbox dulu): `https://erapay-api.erayadigital.co.id/webhooks/doku`. Daftarkan dengan `https://` agar tidak ada redirect pada POST. Status verifikasi webhook: `docs/integrations/doku.md` §4 dan §8.

## Web (SvelteKit)
Menyusul. Env `ERAPAY_API_URL` di sisi web diarahkan ke `https://erapay-api.erayadigital.co.id` (atau `http://127.0.0.1:8181` bila SvelteKit satu server dan memanggil API dari sisi server).
