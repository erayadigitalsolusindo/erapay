# EraPay Listener (Android)

Aplikasi kecil yang membaca notifikasi dari aplikasi bank/e-wallet yang **namanya Anda daftarkan**, lalu meneruskannya
ke sebuah **webhook** (mis. backend ARUS) dengan tanda tangan HMAC. Pola yang sama dengan "QRISGate Listener".

- Tanpa akun, tanpa server EraPay: cukup isi **URL webhook (https)**, **secret bersama**, dan **nama paket**.
- Antrean kirim memakai WorkManager: retry otomatis saat offline/5xx/408/429; 4xx lain dianggap permanen (URL/secret salah).
- Id kiriman stabil (`delivery_id`), jadi notifikasi yang dikirim ulang sistem tidak menggandakan event di penerima.

## Dipakai dengan QRISGate EraPay
Di menu Integrasi → tab QRISGate, EraPay menampilkan **URL webhook** (`…/webhooks/qrisgate/<merchant_id>`) dan membuat **rahasia listener** (`qls_…`, tampil sekali). Isikan keduanya di aplikasi ini, dengan nama paket yang sama seperti di langkah 2 tab itu. Penerima (EraPay) memverifikasi tanda tangan, mencocokkan nominal unik, dan melunasi pembayaran; lihat `docs/integrations/qrisgate.md`.

## Pakai
1. Pasang APK, buka aplikasi, isi URL webhook, secret (min. 16 karakter), dan nama paket satu per baris
   (mis. `com.gojek.app`, `id.dana`). Nama paket lihat di halaman Play Store aplikasinya (`?id=...`).
2. Tekan **Beri izin akses notifikasi** dan aktifkan EraPay Listener.
3. Tekan **Matikan penghemat baterai** (tanpa ini sistem bisa menghentikan listener).
4. **Kirim tes ke webhook** untuk memastikan penerima menerima dan memverifikasi.

## Format webhook
`POST <URL>` dengan header:

| Header | Isi |
|---|---|
| `Content-Type` | `application/json` |
| `X-Listener-Delivery-Id` | id unik event (kunci idempotensi, sama tiap retry) |
| `X-Listener-Timestamp` | unix detik |
| `X-Listener-Signature` | `sha256=` + hex(HMAC-SHA256(secret, `<timestamp>.` + body mentah)) |

Body:

```json
{
  "event": "notification.posted",
  "delivery_id": "9f2c…",
  "device_id": "c1b0…",
  "device_name": "HP kasir 1",
  "package": "com.gojek.app",
  "title": "Pembayaran diterima",
  "text": "Rp 25.000 dari BUDI",
  "amount": 25000,
  "posted_at": "2026-11-20T10:15:30Z"
}
```

`event` = `notification.posted` atau `listener.test` (tombol tes; abaikan di produksi). `amount` hanya **tebakan** dari teks
(rupiah pertama yang ditemukan, `null` bila tak ada); `text` mentah adalah sumber kebenaran.

## Penerima wajib
1. Hitung HMAC atas **body mentah** (bukan hasil parse ulang), banding **constant-time**.
2. Tolak bila `|now − timestamp| > 300` detik.
3. Simpan `delivery_id` dan abaikan duplikat (webhook bisa tiba lebih dari sekali).
4. Jangan menandai pesanan lunas hanya dari notifikasi: cocokkan nominal + waktu dengan pesanan yang menunggu, dan beri
   jalur konfirmasi manual untuk nominal yang tidak cocok (notifikasi bisa terlambat, hilang, atau dipalsukan oleh aplikasi lain).

Contoh verifikasi (Go):

```go
func verify(secret string, r *http.Request, body []byte) bool {
	ts, _ := strconv.ParseInt(r.Header.Get("X-Listener-Timestamp"), 10, 64)
	if d := time.Now().Unix() - ts; d > 300 || d < -300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(r.Header.Get("X-Listener-Signature")))
}
```

## Build
```bash
export JAVA_HOME=/path/ke/jdk17 ANDROID_HOME=/path/ke/android-sdk
./gradlew :app:testDebugUnitTest :app:assembleDebug   # APK: app/build/outputs/apk/debug/app-debug.apk
```
Untuk rilis, buat keystore sendiri dan tanda tangani APK (keystore tidak boleh masuk repo).

## Batasan yang perlu diketahui
- **Play Store** membatasi akses notifikasi; distribusi paling mudah lewat sideload (APK langsung).
- Deteksi bergantung pada HP: baterai/mode hemat, notifikasi yang dimatikan, atau HP mati berarti pembayaran tidak terbaca.
- Secret tersimpan di penyimpanan privat aplikasi (tanpa enkripsi tambahan); jangan pasang di perangkat yang tidak dipercaya.
- Notifikasi bisa dipalsukan oleh aplikasi lain yang punya izin tampil notifikasi; karena itu `text` harus dicocokkan dengan pesanan.
