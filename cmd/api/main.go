package main

import (
	"image-pipeline/internal/app"

	_ "image-pipeline/docs" // swagger generated docs
)

func main() {
	application := app.NewApp()
	application.Run()
}
