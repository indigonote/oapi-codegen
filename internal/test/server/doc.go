package server

//go:generate go run sios.tech/indigo/oapi-codegen/cmd/oapi-codegen --generate=types,chi-server --package=server -o server.gen.go ../test-schema.yaml
//go:generate go run github.com/matryer/moq -out server_moq.gen.go . ServerInterface
