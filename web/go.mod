module github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web

go 1.24.0

require (
	github.com/clerk/clerk-sdk-go/v2 v2.5.1
	github.com/go-jose/go-jose/v3 v3.0.4
	github.com/tursodatabase/go-libsql v0.0.0-20251219133454-43644db490ff
	github.com/victoroliveirab/vibe-pokemongo-appraisal-app/shared v0.0.0
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/libsql/sqlite-antlr4-parser v0.0.0-20240327125255-dbf53b6cbf06 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/slog-betterstack v1.4.3 // indirect
	github.com/samber/slog-common v0.20.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/exp v0.0.0-20230515195305-f3d0a9c9a5cc // indirect
	golang.org/x/text v0.30.0 // indirect
)

replace github.com/victoroliveirab/vibe-pokemongo-appraisal-app/shared => ../shared
