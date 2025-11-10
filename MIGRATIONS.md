# Migrasi Database

Tool migrasi baru berbasis [`golang-migrate`](https://github.com/golang-migrate/migrate) disediakan melalui
`cmd/migrate`. File SQL tetap berada di folder `migrations/` dan diberi penomoran berurutan.

## Cara Pakai

Semua perintah berikut dijalankan dari root repo backend (pastikan `.env` berisi `DB_*` atau `DATABASE_URL`).

```bash
# Jalankan seluruh migrasi (up)
make migrate-up

# Turunkan seluruh migrasi (hati-hati)
make migrate-down

# Lihat versi saat ini
make migrate-version

# Naik/turun beberapa langkah (contoh naik 1)
make migrate-steps steps=1

# Paksa set versi tertentu (berguna untuk baseline DB lama)
make migrate-force version=7
```

## Baseline DB Lama

Jika database lokal sudah pernah dibuat secara manual sebelum tool ini ada, jalankan satu kali:

```bash
make migrate-force version=7
```

Setelah itu `make migrate-up` akan bekerja normal dan hanya menjalankan migrasi baru yang ditambahkan.

## Docker Compose

Untuk menjalankan migrasi terhadap database di `docker-compose`, cukup pastikan servis `db` sedang berjalan
(`docker compose up -d db`) lalu jalankan perintah `make migrate-up` seperti biasa.
