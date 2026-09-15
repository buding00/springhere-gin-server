package main

import (
	"github.com/buding00/springhere-gin-server/internal/model"
	"gorm.io/gen"
)

func main() {
	g := gen.NewGenerator(gen.Config{
		OutPath: "internal/model/query",
		Mode:    gen.WithQueryInterface,
	})
	// Derive query metadata from source models. This does not open or inspect a
	// database, so generation remains deterministic in CI and local development.
	g.ApplyBasic(model.User{})
	g.Execute()
}
