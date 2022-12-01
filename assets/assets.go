package assets

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed swagger-ui
var swaggerUI embed.FS

//go:embed *.yaml
var OpenapiYML embed.FS

func SwaggerUI() http.FileSystem {
	fsys, err := fs.Sub(swaggerUI, "swagger-ui")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}
