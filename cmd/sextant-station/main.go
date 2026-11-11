// Command sextant-station is an imaging station's program (design 0018). It
// tells the console which machines are on the station's imaging network,
// claims the jobs an operator approved, and hands each one to the machine it
// is for when that machine, booted over the network into the installer, asks
// for it. The installer reports its steps back here and the station relays
// them, so the console's job page shows every step.
//
// It replaces the bash runner that installed targets over SSH: a target
// installs itself with the same sextant-installer as the installation USB.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	consoleURL := flag.String("console", "", "console address, e.g. https://sextant.example.org")
	tag := flag.String("tag", "", "this station's tag")
	credential := flag.String("credential", "/var/lib/sextant-agent/credential", "the station's credential file")
	leases := flag.String("leases", "/var/lib/dnsmasq/dnsmasq.leases", "dnsmasq's lease file")
	listen := flag.String("listen", ":8080", "address on the imaging network")
	boot := flag.String("boot", "", "netboot files to serve under /boot/ (optional)")
	state := flag.String("state", "/var/lib/sextant-station", "state directory")
	every := flag.Duration("every", 30*time.Second, "how often to report and claim")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if *consoleURL == "" || *tag == "" {
		log.Error("usage: sextant-station --console https://... --tag station-...")
		os.Exit(2)
	}

	s := &station{
		console: &console{url: strings.TrimSuffix(*consoleURL, "/"), tag: *tag, credential: *credential,
			http: &http.Client{Timeout: time.Minute}},
		leases: *leases, state: *state, log: log,
	}
	if err := s.load(); err != nil {
		log.Error("cannot read the jobs a previous run held", "err", err)
		os.Exit(1)
	}
	if err := s.run(*listen, *boot, *every); err != nil {
		log.Error("serve the imaging network", "err", err)
		os.Exit(1)
	}
}

// run serves the imaging network and reports and claims every interval,
// until a signal stops it.
func (s *station) run(listen, boot string, every time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: listen, Handler: s.routes(boot), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			s.tick(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	s.log.Info("station up", "tag", s.console.tag, "listen", listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
