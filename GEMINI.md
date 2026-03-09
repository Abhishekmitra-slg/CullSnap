# Cloud Agent Development Guidelines

When modifying this repository, strictly adhere to the following Go-specific development rules:

- **Idiomatic Go:** Always write idiomatic Go code. Ensure any changes pass `gofmt` and `go lint` without errors.
- **Performance & Memory:** Prioritize efficient memory management and file system operations. This application handles sorting large volumes of RAW files and extracting embedded previews—performance is critical.
- **Bounded Concurrency:** Use bounded concurrency (e.g., worker pools) when scanning directories or processing multiple files. Do not spawn unbounded goroutines to avoid resource exhaustion or memory spikes.
- **Strict Error Handling:** Enforce strict error handling. Wrap errors with context instead of using generic strings. Never fail or swallow errors silently.
- **Testing:** Write table-driven unit tests for any new parser or file-handling logic.
