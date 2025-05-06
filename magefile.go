//go:build mage

package main

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	project = "protoc-gen-go-mcp"
	version = "0.1.0"

	binDir      = "bin"
	cmdDir      = "cmd"
	tempDir     = "tmp"
	testTempDir = tempDir + "/test"
	workDir, _  = os.Getwd()
	dirPerms    = fs.FileMode(0755)
	filePerms   = fs.FileMode(0644)

	pkg   = getEnv("PACKAGE", "./...")
	tests = getEnv("TESTS", "Test")

	env = map[string]string{
		"CGO_ENABLED": "0",
		"TEMP_DIR":    tempDir,
	}
)

func init() {
	fmt.Println("=> init")

	// outputs for GitHub Actions
	outputs := strings.Join([]string{"PROJECT_NAME=" + project, "VERSION=" + version, ""}, "\n")
	os.MkdirAll(tempDir, dirPerms)
	os.WriteFile(tempDir+"/outputs", []byte(outputs), filePerms)
	os.WriteFile(tempDir+"/summary.md", fmt.Appendf(nil, "```\n%s```", outputs), filePerms)
	fmt.Println(outputs)
}

//-----------------------------------------------------------------------------

// Clean removes all build artifacts.
func Clean() {
	fmt.Println("=> clean")
	bash("rm -rf %s %s", binDir, tempDir)
	bash("go clean -cache")
}

// Tidy updates dependencies.
func Tidy() {
	fmt.Println("=> tidy")
	bash("go mod tidy -v")
}

// Build builds the application for the local OS.
func Build() {
	mg.SerialDeps(Tidy)

	fmt.Println("=> build")
	sh.Rm(binDir)
	bash("go build -o ./%s/ ./%s/%s", binDir, cmdDir, project)
}

// Test runs unit tests.
func Test() {
	mg.SerialDeps(Tidy)

	fmt.Println("=> test")
	env["TEST_TEMP_DIR"] = fmt.Sprintf("%s/%s", workDir, testTempDir)
	env["LOGGING_FILE"] = env["TEST_TEMP_DIR"] + "/app.log"

	sh.Rm(testTempDir)
	os.MkdirAll(testTempDir, dirPerms)
	bash("go clean -testcache")
	bash("go test -v -run %s %s | tee %s/test.out", tests, pkg, testTempDir)
}

// Run runs the application.
func Run(args string) {
	fmt.Println("=> run")
	bash("./%s/%s %s", binDir, project, args)
}

//-----------------------------------------------------------------------------

type Util mg.Namespace

// Env prints the env vars defined in the env map.
func (Util) Env() {
	fmt.Println("=> util:env")
	keys := []string{}

	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Printf("%s=%s\n", k, env[k])
	}
}

//-----------------------------------------------------------------------------

func getEnv(key string, def string) string {
	return getEnvOr(key, func() string { return def })
}

func getEnvOr(key string, def func() string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def()
}

func bash(format string, args ...any) {
	cmd := []string{"-o", "pipefail", "-c", fmt.Sprintf(format, args...)}
	fmt.Println(cmd[len(cmd)-1])

	if err := sh.RunWithV(env, "bash", cmd...); err != nil {
		os.Exit(1)
	}
}
