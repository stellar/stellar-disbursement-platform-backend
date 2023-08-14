package publicfiles

import "embed"

//go:embed css/* js/* img/*
var PublicFiles embed.FS
