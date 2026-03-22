package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"

	"github.com/canhta/vibegram/internal/config"
)

type cliDeps struct {
	lookPath       func(string) (string, error)
	executablePath func() (string, error)
	lookupUser     func(string) (*user.User, error)
	runCommand     func(context.Context, string, ...string) error
}

func defaultCLIDeps() cliDeps {
	return cliDeps{
		lookPath:       exec.LookPath,
		executablePath: os.Executable,
		lookupUser:     user.Lookup,
		runCommand:     runCommand,
	}
}

func runArgs(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runArgsWithDeps(ctx, args, stdin, stdout, stderr, defaultCLIDeps())
}

func runArgsWithDeps(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, deps cliDeps) error {
	if len(args) == 0 {
		return runDaemon(ctx, configLoader(""))
	}

	switch args[0] {
	case "daemon":
		fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
		fs.SetOutput(stderr)
		envFile := fs.String("env-file", "", "path to vibegram env file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return runDaemon(ctx, configLoader(*envFile))
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		envFile := fs.String("env-file", "/etc/vibegram/env", "path to vibegram env file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return runInit(ctx, stdin, stdout, *envFile, deps)
	case "service":
		return runService(ctx, args[1:], stdout, stderr, deps)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func configLoader(envFile string) func() (config.Config, error) {
	if envFile == "" {
		return config.LoadFromEnv
	}
	return func() (config.Config, error) {
		return config.LoadFromEnvFile(envFile)
	}
}
