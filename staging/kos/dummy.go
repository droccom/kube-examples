package main

// This is here only so that `go mod vendor` will actually put
// k8s.io/code-generator in vendor/

import (
	_ "k8s.io/code-generator"
)

func main() {
	return
}
