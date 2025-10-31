// Package server - Core server code
package server

/*
 * server.go
 * Core server code
 * By J. Stuart McMurray
 * Created 20251030
 * Last Modified 20251031
 */

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/magisterquis/curlrevshell/lib/ctxerrgroup"
)

// IOTimeout is more or less how long a request can last and how long an idle
// connection can stay idle.  It's also how long we wait for the server to
// terminate when the context comes done.
const IOTimeout = time.Minute

// IDAlphabet are the characters allowed in IDs.
const IDAlphabet = "abcdefghikjlmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPRQSTUVWXYZ" +
	"0123456789-."

// idParam is the URL parameter containing the ID.
const idParam = "id"

// OutputSuffix is appended to an ID to form an output filename.
const OutputSuffix = "_out"

// Serve serves bots on the given listener, using root as the root from which
// to retrieve tasking and to which to store output.
func Serve(
	ctx context.Context,
	sl *slog.Logger,
	l net.Listener,
	prefix string,
	root *os.Root,
) error {
	/* Set up routes. */
	prefix = CleanPrefix(prefix)
	h := handler{sl: sl, root: root, prefix: prefix}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("GET "+prefix+"{"+idParam+"...}", h.handleTasking)
	mux.HandleFunc("POST "+prefix+"{"+idParam+"...}", h.handleOutput)
	mux.HandleFunc("PUT "+prefix+"{"+idParam+"...}", h.handleOutput)

	/* Server, which requires the prefix. */
	svr := http.Server{
		Handler:      mux,
		ReadTimeout:  IOTimeout,
		WriteTimeout: IOTimeout,
		IdleTimeout:  IOTimeout,
		ErrorLog: slog.NewLogLogger(
			slog.Default().Handler(),
			slog.LevelDebug,
		),
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	/* Start the server plus something to watch for the context closing. */
	eg, ctx := ctxerrgroup.WithContext(ctx)
	eg.GoTag(ctx, "server", func(context.Context) error {
		if err := svr.Serve(l); nil != err &&
			!errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	eg.GoTag(ctx, "waiter", func(ctx context.Context) error {
		<-ctx.Done()
		toCtx, cancel := context.WithTimeout(
			context.Background(),
			IOTimeout,
		)
		defer cancel()
		return svr.Shutdown(toCtx)
	})

	/* Wait for something to go wrong. */
	return eg.Wait()
}

// CleanPrefix ensures prefix starts with a / and ends with no /.
func CleanPrefix(prefix string) string {
	prefix = strings.TrimLeft(prefix, "/")
	prefix = strings.TrimRight(prefix, "/")
	if 0 == len(prefix) {
		return "/"
	}
	return "/" + prefix + "/"
}

// handler holds the HTTP handler functions.  It's really just a handy way to
// pass them sl and root.
type handler struct {
	sl     *slog.Logger
	root   *os.Root
	prefix string
}

// handleTasking responds to a request for tasking with the contents of the
// tasking file with the same name as the last path element of the request, if
// it exists.
func (h handler) handleTasking(w http.ResponseWriter, r *http.Request) {
	id, sl := h.getIDAndLogger(r)
	if "" == id { /* Don't tell it's not ok. */
		w.WriteHeader(http.StatusNotFound)
		return
	}

	/* File with tasking, maybe. */
	f, err := h.root.Open(id)
	if errors.Is(err, os.ErrNotExist) {
		sl.Debug("No tasking")
		return
	} else if nil != err {
		sl.Error(
			"Could not open tasking file",
			"filename", id,
			"error", err,
		)
		return
	}
	defer f.Close()
	if err := h.root.Remove(id); nil != err {
		sl.Error(
			"Unable to remove tasking file",
			"filename", id,
			"error", err,
		)
		return
	}

	/* Send it. */
	n, err := io.Copy(w, f)
	sl = sl.With("size", n)
	lf := sl.Info
	if nil != err {
		sl = sl.With("error", err)
		lf = sl.Error
	} else if 0 == n {
		lf = sl.Debug
	}
	lf("Tasking")
}

// handleOutput appends the contents of the request body to the file with the
// same name as the last path element of the request, plus _out.
func (h handler) handleOutput(w http.ResponseWriter, r *http.Request) {
	id, sl := h.getIDAndLogger(r)
	if "" == id { /* Don't tell it's not ok. */
		w.WriteHeader(http.StatusNotFound)
		return
	}

	/* Open the output file and make sure it's updated. */
	fn := id + OutputSuffix
	f, err := h.root.OpenFile(
		fn,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0660,
	)
	if nil != err {
		sl.Error(
			"Could not open output file",
			"filename", fn,
			"error", err,
		)
		return
	}
	defer f.Close()

	/* Save the body. */
	n, err := io.Copy(f, r.Body)
	sl = sl.With("size", n)

	/* Log what we did. */
	lf := sl.Info
	if nil != err {
		sl = sl.With("error", err)
		lf = sl.Error
	} else if 0 == n {
		lf = sl.Debug
	}
	lf("Output")
}

// getIDAndLogger gets the last part of the path in r and makes sure it's only
// letters, digits, hyphens, and dots.
// The ID's output file is touched and a log generated if the file was created.
func (h handler) getIDAndLogger(r *http.Request) (string, *slog.Logger) {
	/* Logger with request info. */
	raddr := r.RemoteAddr
	host := r.Host
	if testing.Testing() {
		raddr = "test_remote_addr"
		host = "test_host"
	}
	sl := h.sl.With(
		slog.Group(
			"request",
			"remote_address", raddr,
			"method", r.Method,
			"host", host,
			"request_uri", r.RequestURI,
			"user_agent", r.UserAgent(),
		),
	)

	/* Get and check the ID. */
	id := r.PathValue(idParam)
	if rind := strings.LastIndex(id, "/"); -1 != rind {
		id = id[rind+1:]
	}
	if "" == id {
		sl.Warn("Empty ID")
		return "", nil
	}
	if "" != strings.Trim(id, IDAlphabet) {
		sl.Warn("Invalid ID")
		return "", nil
	}
	sl = sl.With("id", id)

	/* Touch the output file. */
	var (
		fn  = id + OutputSuffix
		now = time.Now()
	)
	if err := h.root.Chtimes(
		fn,
		time.Time{},
		now,
	); errors.Is(err, os.ErrNotExist) {
		/* New implant. */
		if f, err := h.root.OpenFile(
			fn,
			os.O_CREATE|os.O_RDONLY,
			0660,
		); nil != err {
			sl.Error(
				"Could not create output file",
				"filename", fn,
				"error", err,
			)
		} else {
			f.Close()
			sl.Info("New ID")
		}
	} else if nil != err {
		sl.Error(
			"Could not update output file mtime",
			"filename", fn,
			"time", now.Format(time.RFC3339),
			"error", err,
		)
	}

	return id, sl
}
