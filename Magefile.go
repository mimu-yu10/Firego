//go:build mage

// Package main defines the mage build targets for Firego: Lint, Build, Test,
// and Ci (which runs all three). See https://magefile.org for the mage tool
// itself.
package main

import (
	"fmt"
	"os"
	"os/exec"
)

// golangciLintVersion pins the golangci-lint version run by Lint, so local
// runs and CI always use the same linter regardless of what's cached on the
// machine invoking mage.
const golangciLintVersion = "v2.12.2"

// Lint runs golangci-lint against the whole module.
func Lint() error {
	return run("go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"+golangciLintVersion, "run", "./...")
}

// Build compiles every package in the module.
func Build() error {
	return run("go", "build", "./...")
}

// Test runs the test suite with the race detector enabled.
func Test() error {
	return run("go", "test", "-race", "./...")
}

// Ci runs Lint, Build, and Test, stopping at the first failure.
func Ci() error {
	for _, target := range []func() error{Lint, Build, Test} {
		if err := target(); err != nil {
			return err
		}
	}
	return nil
}

func run(cmd string, args ...string) error {
	fmt.Println("+", cmd, fmt.Sprint(args))
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()
	return c.Run()
}
