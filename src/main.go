package main

import (
	"imgserver/internal/appimgserver"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	app := appimageserver.NewImgSrv(port)

	defer app.Stop()

	app.Start()
}
