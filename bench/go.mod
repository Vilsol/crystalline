module github.com/Vilsol/crystalline/bench

go 1.26

require github.com/Vilsol/crystalline v0.0.0

require (
	golang.org/x/mod v0.39.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace github.com/Vilsol/crystalline => ../

tool github.com/Vilsol/crystalline/cmd/crystalline
