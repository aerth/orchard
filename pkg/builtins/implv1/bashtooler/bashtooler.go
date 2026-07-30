package bashtooler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"m/pkg/global"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/aerth/mostly/stackerr"
)

var NewCleanEnviron = func() []string { return nil }

func Init(newcleanenv func() []string) {
	NewCleanEnviron = newcleanenv
}

func RunBash(ctx context.Context, wd string, readFile func(string) ([]byte, error), filename string, args ...string) (string, error) {
	b, err := readFile(filename)
	if err != nil {
		return "", err
	}
	stdin := bytes.NewReader(b)
	return RunBashScript(ctx, stdin, RunParams{WorkingDir: wd, Args: append([]string{"/dev/stdin"}, args...)})
}

func RunPy(ctx context.Context, wd string, readFile func(string) ([]byte, error), filename string, args ...string) (string, error) {
	b, err := readFile(filename)
	if err != nil {
		return "", err
	}
	stdin := bytes.NewReader(b)
	return RunPyStdin(ctx, stdin, RunParams{WorkingDir: wd, Args: args})
}

type RunParams struct {
	CombinedStderr bool
	Args           []string
	WorkingDir     string
	AddEnviron     []string
}

var EchoBash = false

// RunBashScript runs a script (stdin reader) using and returns stdout as a string
// caller must prefix args with /dev/stdin if script is stdin
func RunBashScript(ctx context.Context, stdinReal io.Reader, params RunParams) (string, error) {
	if stdinReal == nil {
		return "", fmt.Errorf("missing stdin script")
	}
	stdin := new(bytes.Buffer)
	if _, err := io.Copy(stdin, stdinReal); err != nil {
		return "", stackerr.Errorf("reading stdin failed: %w", err)
	}
	if err := ValidateCommand(stdin.String()); err != nil {
		return "", stackerr.Errorf("command validation failed: %w", err)
	}
	args := params.Args
	cmd := exec.CommandContext(ctx, bashExec, args...)
	var buf bytes.Buffer
	if params.WorkingDir == "" {
		params.WorkingDir = "."
	}

	var mw io.Writer = &buf
	if EchoBash {
		mw = io.MultiWriter(global.LogWriter, &buf)
	}
	cmd.Dir = params.WorkingDir
	cmd.Stderr = mw
	cmd.Stdout = mw
	cmd.Stdin = stdin
	cmd.Env = append(NewCleanEnviron(), params.AddEnviron...)
	if err := cmd.Run(); err != nil {
		//		wd, _ := os.Getwd()
		slog.Error(fmt.Sprintf("got err running cmd from %s: %s", params.WorkingDir, buf.String()))
		err = errors.Join(err, fmt.Errorf("%s", buf.String()))
		return strings.TrimSpace(buf.String()), err
	}

	if strings.TrimSpace(buf.String()) == "" {
		buf.WriteString("(no output)")
	}
	var x string
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		x = fmt.Sprintf("\nrun_bash exit code: %d", cmd.ProcessState.ExitCode())
	}
	//if EchoBash {
	//	println(buf.String())
	//}
	return strings.TrimSpace(buf.String() + x), nil
}

// DangerPattern holds a regex and its human-readable rejection reason.
type DangerPattern struct {
	regex  *regexp.Regexp
	reason string
}

const bashExec = "/usr/bin/bash"

var (
	pipeExecRe      = regexp.MustCompile(`\b(?:curl|wget)\b.*\|\s*(?:bash|sh|zsh|dash)\b`)
	operatorSplitRe = regexp.MustCompile(`\s*(?:&&|\|\||;)\s*`)

	dangerPatterns = []DangerPattern{
		{
			regex:  regexp.MustCompile(`\brm\s+.*-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*\s+[/~*]|\brm\s+.*-[a-zA-Z]*f[a-zA-Z]*r[a-zA-Z]*\s+[/~*]`),
			reason: "Recursive forced deletion of root, home, or wildcard paths",
		},
		{regex: regexp.MustCompile(`\bmkfs\b`), reason: "Filesystem formatting"},
		{regex: regexp.MustCompile(`\bdd\s+.*\bif=`), reason: "Raw disk image write"},
		{regex: regexp.MustCompile(`>\s*/dev/sd[a-z]`), reason: "Device overwrite"},
		{regex: regexp.MustCompile(`\bsudo\b`), reason: "Privilege escalation via sudo"},
		{regex: regexp.MustCompile(`\bsu\s+-`), reason: "User switching via su"},
		{regex: regexp.MustCompile(`\bchmod\s+.*-[a-zA-Z]*R.*\b777\b.*\s+/`), reason: "Recursive world-writable permission on root"},
		{regex: regexp.MustCompile(`\bchown\s+.*-[a-zA-Z]*R`), reason: "Recursive ownership change"},
		{regex: regexp.MustCompile(`\beval\b`), reason: "Arbitrary code evaluation"},
		{regex: regexp.MustCompile(`\bexec\b`), reason: "Process replacement via exec"},
		{regex: regexp.MustCompile(`:\(\)\s*\{\s*.*:\|:.*\s*\}`), reason: "Fork bomb"}, // Simplified slightly for standard Go RE2 matching
		{regex: regexp.MustCompile(`\bgit\s+push\s+.*--force\b`), reason: "Force push"},
		{regex: regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), reason: "Hard reset"},
		{regex: regexp.MustCompile(`\bgit\s+clean\s+.*-[a-zA-Z]*f`), reason: "Force clean"},
		{regex: regexp.MustCompile(`\bshutdown\b`), reason: "System shutdown"},
		{regex: regexp.MustCompile(`\breboot\b`), reason: "System reboot"},
		{regex: regexp.MustCompile(`\bhalt\b`), reason: "System halt"},
		{regex: regexp.MustCompile(`\bkill\s+.*-9\s+1\b`), reason: "Killing init process"},
	}
)

var ErrDangerous = errors.New("dangerous")

// ValidateCommand checks strings against dangerous patterns.
func ValidateCommand(command string) error {
	if pipeExecRe.MatchString(command) {
		return stackerr.Errorf("%w", ErrDangerous)
	}
	segments := operatorSplitRe.Split(command, -1)
	for _, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		for _, pattern := range dangerPatterns {
			if pattern.regex.MatchString(trimmed) {
				return ErrDangerous
			}
		}
	}
	return nil
}

var EchoPython = false

// RunPyStdin runs a script (stdin reader) using a specific venv python path and returns stdout as a string
func RunPyStdin(ctx context.Context, stdin io.Reader, params RunParams) (string, error) {
	args := params.Args
	args = append([]string{"-"}, args...)
	const python3Executable = ".venv/bin/python3"
	abs, err := os.Readlink(python3Executable)
	if err != nil {
		slog.Warn("uhoh ", "err", err)
		return "", fmt.Errorf("finding python failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, abs, args...)
	var buf bytes.Buffer
	if params.WorkingDir == "" {
		params.WorkingDir = "."
	}
	var mw io.Writer = &buf
	if EchoPython {
		mw = io.MultiWriter(global.LogWriter, &buf)
	}
	cmd.Dir = params.WorkingDir
	cmd.Stderr = mw
	cmd.Stdout = mw
	cmd.Stdin = stdin
	cmd.Env = append(NewCleanEnviron(), params.AddEnviron...)
	if err := cmd.Run(); err != nil {
		//		wd, _ := os.Getwd()
		if errors.Is(err, context.Canceled) {
			return strings.TrimSpace(buf.String()) + " (canceled)", err
		}
		slog.Error(fmt.Sprintf("got err running cmd from %s: %s", params.WorkingDir, buf.String()))
		err = errors.Join(err, fmt.Errorf("%s", buf.String()))
		return strings.TrimSpace(buf.String()), err
	}
	if EchoPython {
		println(buf.String())
	}
	return strings.TrimSpace(buf.String()), nil
}
