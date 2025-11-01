package server

/*
 * server_test.go
 * Tests for server.go
 * By J. Stuart McMurray
 * Created 20251030
 * Last Modified 20251101
 */

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magisterquis/curlrevshell/lib/chanlog"
)

// testPathPrefix is the prefix passed to Serve.
const testPathPrefix = "kittens"

// newTestServer starts a server serving.  The server's logs, URL prefix with
// a slash, and file root are returned.
func newTestServer(t *testing.T) (chanlog.ChanLog, string, *os.Root) {
	/* Logging. */
	cl, sl := chanlog.New()

	/* Listener. */
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if nil != err {
		t.Fatalf("Error starting listener: %s", err)
	}
	t.Cleanup(func() { l.Close() })

	/* File root. */
	root, err := os.OpenRoot(t.TempDir())
	if nil != err {
		t.Fatalf("Error opening root: %s", err)
	}

	/* Start serving. */
	ech := make(chan error, 1)
	go func() {
		ech <- Serve(t.Context(), sl, l, testPathPrefix, root)
	}()
	t.Cleanup(func() {
		if err := <-ech; nil != err {
			t.Errorf("Server returned error: %s", err)
		}
	})

	return cl, fmt.Sprintf("http://%s/%s/", l.Addr(), testPathPrefix), root
}

func TestServe_Smoketest(t *testing.T) { newTestServer(t) }

func TestCleanPrefix(t *testing.T) {
	for n, c := range map[string]struct {
		have string
		want string
	}{"empty": {
		have: "",
		want: "/",
	}, "one_slash": {
		have: "/",
		want: "/",
	}, "two_slashes": {
		have: "//",
		want: "/",
	}, "many_slashes": {
		have: "////////////////",
		want: "/",
	}, "leading_slashes": {
		have: "/////kittens",
		want: "/kittens/",
	}, "trailing_slashes": {
		have: "kittens/////////",
		want: "/kittens/",
	}, "surrounding_slashes": {
		have: "//////////kittens///////////",
		want: "/kittens/",
	}} {
		t.Run(n, func(t *testing.T) {
			/* Did it work? */
			got := CleanPrefix(c.have)
			if got != c.want {
				t.Errorf(
					"Clean failed\n"+
						"have: %s\n"+
						" got: %s\n"+
						"want: %s",
					c.have,
					got,
					c.want,
				)
			}
			/* Idempotent? */
			secondGot := CleanPrefix(got)
			if got != secondGot {
				t.Errorf(
					"CleanPrefix not idempotent\n"+
						"        have: %s\n"+
						" first clean: %s\n"+
						"second clean: %s",
					c.have,
					got,
					secondGot,
				)
			}
		})
	}
}

// Do we get a bland answer with no ID and no prefix?
func TestServe_InvalidID(t *testing.T) {
	cl, u, root := newTestServer(t)
	for n, c := range map[string]struct {
		have    string
		wantLog string
	}{"no_ID": {
		have: u,
		wantLog: `{"time":"","level":"WARN","msg":"Empty ID",` +
			`"request":{"remote_address":"test_remote_addr",` +
			`"method":"GET","host":"test_host",` +
			`"request_uri":"/kittens/",` +
			`"user_agent":"Go-http-client/1.1"}}`,
	}, "no_prefix": {
		have: strings.TrimSuffix(u, testPathPrefix+"/"),
	}, "trailing_slash": {
		have: u + "moose/",
		wantLog: `{"time":"","level":"WARN","msg":"Empty ID",` +
			`"request":{"remote_address":"test_remote_addr",` +
			`"method":"GET","host":"test_host",` +
			`"request_uri":"/kittens/moose/",` +
			`"user_agent":"Go-http-client/1.1"}}`,
	}, "wrong_prefix": {
		have: strings.TrimSuffix(u, testPathPrefix+"/") +
			"moose/zoomies",
	}, "bad_characters": {
		have: u + "ab_cd+e",
		wantLog: `{"time":"","level":"WARN","msg":"Invalid ID",` +
			`"request":{"remote_address":"test_remote_addr",` +
			`"method":"GET","host":"test_host",` +
			`"request_uri":"/kittens/ab_cd+e",` +
			`"user_agent":"Go-http-client/1.1"}}`,
	}} {
		t.Run(n, func(t *testing.T) {
			/* GET work? */
			res, err := http.Get(c.have)
			if nil != err {
				t.Fatalf("Get error: %s", err)
			}
			defer res.Body.Close()
			/* Should get a 404. */
			if got, want := res.StatusCode,
				http.StatusNotFound; got != want {
				t.Errorf(
					"Incorrect status code\n"+
						" url: %s\n"+
						" got: %d\n"+
						"want: %d",
					c.have,
					got,
					want,
				)
			}
			/* Shouldn't get a body. */
			b, err := io.ReadAll(res.Body)
			if nil != err {
				t.Errorf(
					"Error reading response body: %s",
					err,
				)
			}
			if 0 != len(b) {
				t.Errorf("Non-empty response body: %s", b)
			}
			/* Shouldn't make a file. */
			if des, err := os.ReadDir(root.Name()); nil != err {
				t.Errorf(
					"Error reading directory %s: %s",
					root.Name(),
					err,
				)
			} else if 0 != len(des) {
				var sb strings.Builder
				for _, de := range des {
					sb.WriteString(de.Name() + "\n")
				}
				t.Errorf("Directory not empty, found\n%s", &sb)
			}
			/* May or may not get logs. */
			logs := make([]string, 0, 1)
			if "" != c.wantLog {
				logs = append(logs, c.wantLog)
			}
			cl.ExpectEmpty(t, logs...)
		})
	}
}

// Does sending output work?
func TestServe_Output(t *testing.T) {
	cl, u, root := newTestServer(t)
	for _, v := range []string{http.MethodPut, http.MethodPost} {
		/* Empty output. */
		t.Run("empty/"+v, func(t *testing.T) {
			id := fmt.Sprintf("empty-%d", rand.Uint64())
			fn := id
			/* Send the not-output. */
			got := request(t, v, u+id, nil)
			if 0 != len(got) {
				t.Errorf("Unexpected response: %s", got)
			}
			/* Should make an empty file. */
			b, err := root.ReadFile(fn)
			if nil != err {
				t.Errorf(
					"Error reading output file %s: %s",
					u,
					err,
				)
			}
			if 0 != len(b) {
				t.Errorf("Output file not empty: %s", b)
			}

			/* Should get a debug log. */
			cl.ExpectEmpty(
				t,
				`{"time":"","level":"INFO",`+
					`"msg":"New ID","request":{`+
					`"remote_address":"test_remote_addr",`+
					`"method":"`+v+`",`+
					`"host":"test_host",`+
					`"request_uri":"/kittens/`+id+`",`+
					`"user_agent":"Go-http-client/1.1"},`+
					`"id":"`+id+`"}`,
				`{"time":"","level":"DEBUG",`+
					`"msg":"Output","request":{`+
					`"remote_address":"test_remote_addr",`+
					`"method":"`+v+`",`+
					`"host":"test_host",`+
					`"request_uri":"/kittens/`+id+`",`+
					`"user_agent":"Go-http-client/1.1"},`+
					`"id":"`+id+`","size":0}`,
			)
		})

		/* Not empty output. */
		t.Run("not_empty/"+v, func(t *testing.T) {
			id := fmt.Sprintf("not-empty-%d", rand.Uint64())
			fn := id
			outputs := []string{
				"Start of output:\n",
				fmt.Sprintf("output: %d\n", rand.Uint64()),
				fmt.Sprintf("output: %d\n", rand.Uint64()),
				"End of output.\n",
			}
			/* Send the not-outputs. */
			for _, o := range outputs {
				if got := request(
					t,
					v,
					u+id,
					strings.NewReader(o),
				); 0 != len(got) {
					t.Errorf(
						"Unexpected response to "+
							"output %q: %s",
						o,
						got,
					)
				}
			}
			/* Should make an output file. */
			b, err := root.ReadFile(fn)
			if nil != err {
				t.Errorf(
					"Error reading output file %s: %s",
					u,
					err,
				)
			}
			if got, want := string(b), strings.Join(
				outputs,
				"",
			); got != want {
				t.Errorf(
					"Output file incorrect\n"+
						" got: %q\n"+
						"want: %q",
					got,
					want,
				)
			}
			/* Should get logs. */
			wantLogs := make([]string, 1+len(outputs))
			wantLogs[0] = `{"time":"","level":"INFO",` +
				`"msg":"New ID","request":{` +
				`"remote_address":"test_remote_addr",` +
				`"method":"` + v + `",` +
				`"host":"test_host",` +
				`"request_uri":"/kittens/` + id + `",` +
				`"user_agent":"Go-http-client/1.1"},` +
				`"id":"` + id + `"}`
			for i, o := range outputs {
				wantLogs[i+1] = `{"time":"","level":"INFO",` +
					`"msg":"Output","request":{` +
					`"remote_address":"test_remote_addr",` +
					`"method":"` + v + `",` +
					`"host":"test_host",` +
					`"request_uri":"/kittens/` + id + `",` +
					`"user_agent":"Go-http-client/1.1"},` +
					`"id":"` + id + `",` +
					`"size":` + strconv.Itoa(len(o)) + `}`
			}
			cl.ExpectEmpty(t, wantLogs...)
		})
	}
}

// Does empty output give us an updated timestamp?
func TestServe_EmptyOutputFileTimestamp(t *testing.T) {
	_, u, root := newTestServer(t)
	id := fmt.Sprintf("ts-%d", rand.Uint64())
	u = u + id
	fn := id

	/* First output should make a file. */
	request(t, http.MethodPost, u, nil)
	fi, err := root.Stat(fn)
	if nil != err {
		t.Fatalf("Error stating %s after first request: %s", fn, err)
	}
	before := fi.ModTime()

	/* Second request should update its timestamp. */
	time.Sleep(time.Millisecond) /* :| */
	request(t, http.MethodPost, u, nil)
	fi, err = root.Stat(fn)
	if nil != err {
		t.Fatalf("Error stating %s after second request: %s", fn, err)
	}
	after := fi.ModTime()

	/* Did it work? */
	if !before.Before(after) {
		t.Errorf("File modification time not updated")
	}
}

// Does getting tasking work?
func TestServe_Tasking(t *testing.T) {
	lf, u, root := newTestServer(t)
	id := fmt.Sprintf("t-%d", rand.Uint64())
	u = u + id
	ofn := id
	tfn := id + TaskingSuffix

	/* No tasking should be easy. */
	if got := request(t, http.MethodGet, u, nil); 0 != len(got) {
		t.Errorf("Got unexpected tasking: %s", got)
	}

	/* Should have gotten an output file, for tracking. */
	fi, err := root.Stat(ofn)
	if nil != err {
		t.Fatalf("Did not get output file after empty tasking")
	}
	if got := fi.Size(); 0 != got {
		t.Errorf(
			"Output file not empty (%d bytes) after empty tasking",
			got,
		)
	}
	mtimeBefore := fi.ModTime()

	/* Make and send some tasking. */
	task := fmt.Sprintf("Task: %d", rand.Uint64())
	if err := root.WriteFile(tfn, []byte(task), 0660); nil != err {
		t.Fatalf("Error writing tasking file %s: %s", tfn, err)
	}
	if got, want := string(request(t, http.MethodGet, u, nil)),
		task; got != want {
		t.Errorf("Incorrect tasking\n got: %q\nwant: %q", got, want)
	}

	/* Make sure file's gone. */
	if _, err := root.Stat(tfn); nil != err &&
		!errors.Is(err, os.ErrNotExist) {
		t.Errorf(
			"Error checking if tasking file %s exists: %s",
			tfn,
			err,
		)
	} else if nil == err {
		t.Errorf("Tasking file %s still exists", tfn)
	}

	/* Output file should update. */
	fi, err = root.Stat(ofn)
	if nil != err {
		t.Fatalf("Did not get output file after non-empty tasking")
	}
	if got := fi.Size(); 0 != got {
		t.Errorf(
			"Output file not empty (%d bytes) "+
				"after non-empty tasking",
			got,
		)
	}
	mtimeAfter := fi.ModTime()
	if !mtimeBefore.Before(mtimeAfter) {
		t.Errorf(
			"Output file timestamp not updated " +
				"after non-empty tasking",
		)
	}

	/* Log-checking. */
	lf.ExpectEmpty(
		t,
		`{"time":"","level":"INFO","msg":"New ID","request":{`+
			`"remote_address":"test_remote_addr","method":"GET",`+
			`"host":"test_host",`+
			`"request_uri":"/kittens/`+id+`",`+
			`"user_agent":"Go-http-client/1.1"},`+
			`"id":"`+id+`"}`,
		`{"time":"","level":"DEBUG","msg":"No tasking","request":{`+
			`"remote_address":"test_remote_addr","method":"GET",`+
			`"host":"test_host",`+
			`"request_uri":"/kittens/`+id+`",`+
			`"user_agent":"Go-http-client/1.1"},`+
			`"id":"`+id+`"}`,
		`{"time":"","level":"INFO","msg":"Tasking","request":{`+
			`"remote_address":"test_remote_addr","method":"GET",`+
			`"host":"test_host",`+
			`"request_uri":"/kittens/`+id+`",`+
			`"user_agent":"Go-http-client/1.1"},`+
			`"id":"`+id+`","size":`+strconv.Itoa(len(task))+`}`,
	)
}

// request makes a request to the given URL with the given method and body and
// returns the response's body.  It terminates the test on error or if the
// respone's status code isn't 200
func request(t *testing.T, method, url string, body io.Reader) []byte {
	t.Helper()
	/* Make the request. */
	req, err := http.NewRequest(method, url, body)
	if nil != err {
		t.Fatalf("Error rolling request for %s: %s", url, err)
	}
	res, err := http.DefaultClient.Do(req)
	if nil != err {
		t.Fatalf("Error making request to %s: %s", url, err)
	}
	defer res.Body.Close()

	/* Make sure the response is ok. */
	if got, want := res.StatusCode, http.StatusOK; got != want {
		t.Fatalf("Incorrect status in response to %s: %d", url, got)
	}

	/* Return the body. */
	resBody, err := io.ReadAll(res.Body)
	if nil != err {
		t.Fatalf("Error reading response to %s: %s", url, err)
	}

	return resBody
}
