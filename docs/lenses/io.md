# Lens: IO

IO and filesystem operations — failure modes, resource management, blocking behaviour, atomicity.

**Defers to**: [general](general.md) (resource management broadly), [error-handling](error-handling.md) (how to handle IO errors once flagged), [phronithm:concurrency](../../skills/concurrency/SKILL.md) (file locking, concurrent access).

**Tool support**: [phronithm:static-analysis](../../skills/static-analysis/SKILL.md) can detect unclosed handles and unchecked return values.

## Concerns

### Failure modes

- Partial reads/writes: short reads are not errors in many APIs — callers must loop.
- Disk full, permission denied, file not found — all produce different recovery needs.
- Network IO: connection drops, timeouts, half-close (read EOF while write still open).
- POSIX: EINTR — must retry after signal interruption.
- EOF vs error distinction in read return values.

### Resource management

- Handles closed on all paths including error paths.
- Prefer language-level guarantees (`with`/`using`/`defer`/RAII). Flag manual cleanup.
- Handles leaked in loops (open inside loop body, not closed before next iteration).
- Multiple handles to same resource: clear which one is "live".

### Blocking behaviour

- Unbounded blocking: reads/writes with no timeout on sockets or slow devices.
- Blocking IO in async/event-loop contexts (async code calling sync IO).
- Synchronous IO on the main thread where UI or latency is a concern.

### Filesystem specifics

- Atomicity: write-then-rename is safe; truncate-then-write is not. Flag direct overwrites.
- TOCTOU: check-then-act races. `exists()`/`stat()` followed by `open()` is a race; open-and-handle-error is correct.
- Path construction: use path-joining APIs, not string concatenation. Platform separators.
- Metadata unreliability: file size from `stat()` may lag; timestamps have platform-dependent resolution.
- Symlink handling: is following symlinks intended? Flag `open()` calls where it may matter.
- Missing parent directories, non-empty directory on delete.

### Buffering and flushing

- Unflushed buffers: data written to a buffer but not flushed before close, crash, or fsync.
- Reading stale data from a buffered reader when the underlying file changed.
- Mixed buffered and unbuffered access to the same file descriptor.

## Red flags

- `write()` return value unchecked or short-count not handled.
- `read()` return value unchecked (assuming full buffer filled).
- No size limit on input read into memory.
- Blocking IO in async context.
- String concatenation for path construction.
- `exists()`/`stat()` then `open()` — TOCTOU.
- Truncating a file before writing the replacement (not atomic).
- EINTR not handled on POSIX.
- File handles not closed on error paths.
- Empty or log-and-continue handlers for IO errors. (Propagation analysis is [error-handling](error-handling.md).)
