package main

import (
	"os"
	"os/exec"
	"strings"
)

func main() {
	out, _ := exec.Command("go", "list", "-f", "{{if .TestGoFiles}}{{.ImportPath}}{{end}}", "./...").Output()
	pkgs := strings.Fields(string(out))

	for _, pkg := range pkgs {
		cmd := exec.Command("go", "test", "-v", pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}
