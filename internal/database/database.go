//  lib/database/database.go

package database

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// koneksi database global
var DB *sql.DB

//  menginisialisasi koneksi ke database
func ConnectDB() {
	
	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"), // Password bisa kosong
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_DATABASE"),
	)

	fmt.Println("Mencoba menghubungkan dengan DSN:", dsn) 

	var err error
	DB, err = sql.Open("postgres", dsn) 
	if err != nil {
		panic(err)
	}

	if err = DB.Ping(); err != nil {
		panic(fmt.Sprintf("Gagal ping database: %v", err))
	}

	fmt.Println("Koneksi ke Database berhasil!")

	DB.SetMaxOpenConns(25) 
	DB.SetMaxIdleConns(25)
	DB.SetConnMaxLifetime(5 * time.Minute) 
	fmt.Println("Koneksi ke Database berhasil dan pool dikonfigurasi!")

}
