//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/sh"
)

// Build builds the postgres-mcp-go binary
func Build() error {
	fmt.Println("Building postgres-mcp-go...")

	binary := "postgres-mcp-go"
	if os.Getenv("GOOS") == "windows" {
		binary += ".exe"
	}

	outputPath := filepath.Join("bin", binary)

	// Ensure bin directory exists
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Build the binary
	return sh.RunV("go", "build", "-o", outputPath, "./cmd/server")
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")
	return os.RemoveAll("bin")
}

// Install installs the binary to $GOPATH/bin
func Install() error {
	fmt.Println("Installing postgres-mcp-go...")
	return sh.RunV("go", "install", "./cmd/server")
}

// Test runs all tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.RunV("go", "test", "./...")
}
