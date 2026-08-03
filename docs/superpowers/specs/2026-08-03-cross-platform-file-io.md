# Cross-Platform File I/O Design

`ly` accepts local media files up to 500 MiB and transfers them with streaming
Go I/O on macOS, Linux, and Windows. Uploads validate size before the request,
stream multipart data, check response status, and propagate all read/write/close
errors. Downloads stream into a target-directory temporary file and replace the
destination only after a complete response and successful close. Unix uses
atomic rename replacement; Windows temporarily moves an existing destination
aside and restores it if replacement fails. Failed transfers remove temporary
files and preserve any prior destination.
