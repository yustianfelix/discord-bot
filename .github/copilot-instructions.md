# Copilot Instructions

## Go style
- Follow standard Go formatting and idiomatic naming.
- Prefer small, well-scoped functions and explicit error handling.
- Keep packages focused: AI logic in `ai/`, Discord interactions in `handlers/`, and audio playback in `music/`.

## Concurrency and safety
- Protect shared mutable state with `sync.Mutex` when used across goroutines.
- Avoid data races around voice session state, playback cancellation, and command handlers.
- Use `context.Context` for cancellation and cleanup of long-running operations like Gemini requests and the media pipeline.

## Process and resource cleanup
- Always clean up external processes (`yt-dlp`, `ffmpeg`) when playback ends, is skipped, or the app shuts down.
- Use `cmd.Process.Kill()` as a last resort when a background process must be stopped immediately.
- Ensure voice connections are disconnected cleanly when playback is stopped or the bot exits.

## Coding expectations
- Prefer `errors.Join` or wrapped errors for clearer diagnostics.
- Keep commands and interactions resilient: validate inputs before executing long-running operations.
- Ensure environment variables and configuration are explicitly checked before starting the bot.
