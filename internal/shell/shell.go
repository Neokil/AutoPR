package shell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

type Result struct {
	Stdout string
	Stderr string
	Code   int
}

func Run(ctx context.Context, dir string, env map[string]string, stdin string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if stdin != "" {
		cmd.Stdin = bytes.NewBufferString(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
	if err != nil {
		return res, fmt.Errorf("run %s %v: %w", name, args, err)
	}
	return res, nil
}
