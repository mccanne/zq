package ztest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func RunShell(dir *Dir, bindir, script string, stdin io.Reader, useenvs []string) (string, string, error) {
	// "-e -o pipefile" ensures a test will fail if any command
	// fails unexpectedly.
	cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", script)
	cmd.Dir = dir.Path()
	for _, env := range useenvs {
		if v, ok := os.LookupEnv(env); ok {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", env, v))
		}
	}
	cmd.Env = append(cmd.Env,
		"AppData="+cmd.Dir, // For os.User*Dir on Windows.
		"HOME="+cmd.Dir,    // For os.User*Dir on Unix.
		"PATH="+bindir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMP="+cmd.Dir,    // For os.TempDir on Windows.
		"TMPDIR="+cmd.Dir, // For os.TempDir on Unix.
	)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
