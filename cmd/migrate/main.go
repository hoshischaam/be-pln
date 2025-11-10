package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	var (
		dirFlag   = flag.String("dir", "migrations", "Folder penyimpanan file migrasi")
		cmdFlag   = flag.String("cmd", "up", "Perintah migrasi: up, down, steps, force, version")
		stepsFlag = flag.Int("steps", 0, "Jumlah langkah untuk perintah steps (positif untuk up, negatif untuk down)")
		forceFlag = flag.Int("forceVersion", -1, "Versi yang akan dipaksakan saat menggunakan cmd=force")
	)
	flag.Parse()

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		var err error
		dsn, err = buildDSNFromParts()
		if err != nil {
			log.Fatalf("DATABASE_URL tidak tersedia dan gagal merangkai DSN dari DB_*: %v", err)
		}
	}

	migrationsDir := fmt.Sprintf("file://%s", filepath.ToSlash(*dirFlag))
	m, err := migrate.New(migrationsDir, dsn)
	if err != nil {
		log.Fatalf("Gagal membuat migrator: %v", err)
	}
	defer m.Close()

	switch strings.ToLower(*cmdFlag) {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migrasi up gagal: %v", err)
		}
		log.Println("Migrasi up selesai.")
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migrasi down gagal: %v", err)
		}
		log.Println("Migrasi down selesai.")
	case "steps":
		if *stepsFlag == 0 {
			log.Fatalf("cmd=steps membutuhkan nilai --steps (positif atau negatif)")
		}
		if err := m.Steps(*stepsFlag); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("Migrasi steps gagal: %v", err)
		}
		log.Println("Migrasi steps selesai.")
	case "force":
		if *forceFlag < 0 {
			log.Fatalf("cmd=force membutuhkan --forceVersion (>=0)")
		}
		if err := m.Force(*forceFlag); err != nil {
			log.Fatalf("Migrasi force gagal: %v", err)
		}
		log.Println("Force version berhasil.")
	case "version":
		version, dirty, err := m.Version()
		if err != nil && err != migrate.ErrNilVersion {
			log.Fatalf("Gagal membaca versi migrasi: %v", err)
		}
		log.Printf("Migrasi versi=%d dirty=%v\n", version, dirty)
	default:
		log.Fatalf("cmd %q tidak dikenali. Gunakan up/down/steps/force/version", *cmdFlag)
	}
}

func buildDSNFromParts() (string, error) {
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	dbName := strings.TrimSpace(os.Getenv("DB_DATABASE"))
	if port == "" {
		port = "5432"
	}
	if user == "" || host == "" || dbName == "" {
		return "", fmt.Errorf("DB_USER/DB_HOST/DB_DATABASE wajib diisi")
	}
	u := &url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%s", host, port),
		Path:     dbName,
		RawQuery: "sslmode=disable",
	}
	password := os.Getenv("DB_PASSWORD")
	if password != "" {
		u.User = url.UserPassword(user, password)
	} else {
		u.User = url.User(user)
	}
	return u.String(), nil
}
