package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Saman-dev12/lb/internal/server"
)

func main() {
	port := os.Getenv("LB_PORT")
	if port == "" {
		port = "8888"
	}

	srv := server.NewServer(":" + port)
	fmt.Printf("Server is listening on port %s\n", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
