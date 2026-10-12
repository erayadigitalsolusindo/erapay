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
git clone https://github.com/erayadigitalsolusindo/erapay.git /www/wwwroot/erapay-src
cd /www/wwwroot/erapay-src && git checkout main
cp deploy/update-erapay.sh ~/ && chmod +x ~/update-erapay.sh   # jalankan ulang bila skrip ini berubah
sudo cp deploy/aapanel/erapay-api.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable erapay-api
```

## 6. Deploy / update
```bash
~/update-erapay.sh
```
Alur: `git pull --ff-only` → build (`deploy/build-release.sh`, binary `erapay-api`/`-migrate`/`-bootstrap`/`-webhookverify`) → backup binary lama (5 terakhir) → `erapay-migrate up` → pasang → restart → cek `/readyz` (rollback binary otomatis jika gagal; migrasi tidak di-rollback).

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

## 9. Web (SvelteKit, `apps/web`)
Semua panggilan ke API terjadi di sisi **server** SvelteKit (`src/lib/server/*`), jadi tidak butuh CORS dan cookie sesi cukup untuk domain web. `ERAPAY_API_URL` diarahkan ke API lokal.

**Prasyarat:** Node >= 22.17 dan npm di server (`node -v`). Cek lokasi: `which node`.

1. **Site aaPanel** `erapay.erayadigital.co.id` (pure static) → Let's Encrypt + Force HTTPS → Reverse proxy ke `http://127.0.0.1:3001` (port 3000 sering dipakai app lain; cek `ss -ltnp | grep 3001`).
2. **Env web**, `/www/wwwroot/erapay.erayadigital.co.id/erapay-web.env` (`chmod 600`, `chown www:www`):
```
HOST=127.0.0.1
PORT=3001
ORIGIN=https://erapay.erayadigital.co.id
ERAPAY_API_URL=http://127.0.0.1:8181
ADDRESS_HEADER=X-Forwarded-For
XFF_DEPTH=2
```
   - `ORIGIN` wajib: tanpa itu SvelteKit menolak form POST (cek CSRF) di belakang proxy.
   - `ADDRESS_HEADER`+`XFF_DEPTH`: IP klien dipakai rate-limit login di API. `2` bila lewat Cloudflare + aaPanel; `1` bila langsung ke aaPanel.
3. **Service systemd** (sekali). Sesuaikan path `node` di `ExecStart` bila bukan `/usr/bin/node`:
```bash
sudo cp /www/wwwroot/erapay-src/deploy/web/erapay-web.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable erapay-web
```
4. **Deploy**: `~/update-erapay.sh` kini memperbarui API lalu web (`SKIP_WEB=1 ~/update-erapay.sh` hanya API). Web: `npm ci` + `npm run build` di server → tar → `deploy/web/update.sh` (backup, `npm ci --omit=dev`, restart, cek `/login`, rollback otomatis bila gagal). Pada deploy pertama, jika belum jalan: `sudo systemctl start erapay-web`.
5. Log: `journalctl -u erapay-web -f`. Login memakai akun yang dibuat dengan `erapay-bootstrap` (bagian 6).

## 10. Diagnosa signature webhook
Setelah ada capture notifikasi DOKU asli (`ERAPAY_WEBHOOK_CAPTURE=1`), cocokkan rumus signature dengan secret server:
```bash
cd /www/wwwroot/erapay-api.erayadigital.co.id
set -a; . ./erapay.env; set +a
./bin/erapay-webhookverify <id-di-tabel-webhook_captures>
```
`MATCH` berarti notifikasi berikutnya akan diterapkan. Matikan `ERAPAY_WEBHOOK_CAPTURE` setelah selesai (capture menyimpan data pembayar).
