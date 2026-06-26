package core

// Version is the engine's build version, injected from main at startup (matches
// the CLI's -ldflags version). Reported to clients via GetServerInfo so the TUI
// can show which server it is connected to and flag a version mismatch.
var Version = "dev"
