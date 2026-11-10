// Command sextant-installer is the program the organisation's installation
// USB runs on tty1 (design 0016): it asks for a network if there is none and
// for the code from the console, says which disk it will erase, installs the
// laptop from the fleet repository and restarts it. The console address is
// built into the stick; nothing that grants access is.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	console := flag.String("console", "", "console address, e.g. https://sextant.example.org")
	flag.Parse()
	if *console == "" {
		fmt.Fprintln(os.Stderr, "usage: sextant-installer --console https://...")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	i := &installer{
		console:   strings.TrimSuffix(*console, "/"),
		in:        bufio.NewReader(os.Stdin),
		out:       os.Stdout,
		run:       runCommand,
		reachable: func(ctx context.Context) bool { return reachable(ctx, *console) },
		hasTPM2:   func() bool { _, err := os.Stat("/sys/class/tpm/tpm0"); return err == nil },
	}
	for {
		err := i.Run(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}
		// Whatever stopped it, the person at the laptop reads why and can
		// start over without knowing a command.
		fmt.Printf("\n%v\n\nPress Enter to start again.", err)
		_, _ = i.in.ReadString('\n')
	}
}

// runCommand runs one tool and reports its output and exit code.
func runCommand(ctx context.Context, env []string, name string, args ...string) runResult {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return runResult{Out: string(out)}
	case errors.As(err, &exit):
		return runResult{Out: string(out), Code: exit.ExitCode()}
	default:
		return runResult{Out: err.Error(), Code: -1}
	}
}

// reachable asks the console whether it is alive.
func reachable(ctx context.Context, console string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(console, "/")+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
