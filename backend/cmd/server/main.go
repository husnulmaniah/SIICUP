package main

import (
	"log"
	"net/http"

	"cuti-app/config"
	"cuti-app/database"
	"cuti-app/routes"
)

func main() {
	config.Load()
	config.ConnectDB()

	database.Migrate(config.DB)
	database.Seed(config.DB)

	handler := routes.SetupRoutes(config.DB)

	addr := ":" + config.App.AppPort
	log.Println("server berjalan di http://localhost" + addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("gagal menjalankan server: %v", err)
	}
}
