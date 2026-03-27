package main

import (
	"fmt"
	"os"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/kong"

	"github.com/anthropics/monocle/internal/adapters"
	"github.com/anthropics/monocle/internal/core"
	"github.com/anthropics/monocle/internal/db"
	"github.com/anthropics/monocle/internal/tui"
)

var version = "dev"

type CLI struct {
	Run             RunCmd             `cmd:"" default:"withargs" help:"Start a review session"`
	Register        RegisterCmd        `cmd:"" help:"Register Monocle for an agent"`
	Unregister      UnregisterCmd      `cmd:"" help:"Remove Monocle registration"`
	ServeMcpChannel ServeMCPChannelCmd `cmd:"serve-mcp-channel" help:"Run the MCP channel server" hidden:""`
	Install         InstallCmd         `cmd:"" help:"Install MCP channel (alias for register)" hidden:""`
	Uninstall       UninstallCmd       `cmd:"" help:"Remove MCP channel (alias for unregister)" hidden:""`
	Version         kong.VersionFlag   `help:"Print version" name:"version"`
}

type RunCmd struct {
	Socket         string   `help:"Override socket path for MCP channel connection" env:"MONOCLE_SOCKET" default:""`
	AdditionalPath []string `help:"Additional file or directory paths to include for review (repeatable)" name:"additional-path" short:"a" type:"path"`
	Continue       bool     `help:"Resume the most recent session for this repo" name:"continue" short:"c" xor:"session-mode"`
	Resume         bool     `help:"Show a picker to resume a previous session" name:"resume" short:"r" xor:"session-mode"`
	Session        string   `help:"Resume a specific session by ID" name:"session" short:"s" default:""`
}

type RegisterCmd struct {
	Agent  string `arg:"" optional:"" help:"Agent to register (claude, opencode, codex, gemini, all)"`
	Global bool   `help:"Register in user-level config instead of project" default:"false"`
}

type UnregisterCmd struct {
	Agent  string `arg:"" optional:"" help:"Agent to unregister (claude, opencode, codex, gemini, all)"`
	Global bool   `help:"Remove from user-level config instead of project" default:"false"`
}

type ServeMCPChannelCmd struct{}

type InstallCmd struct {
	Agent  string `arg:"" optional:"" help:"Agent to register (claude, opencode, codex, gemini, all)"`
	Global bool   `help:"Register in user-level config instead of project" default:"false"`
}

type UninstallCmd struct {
	Agent  string `arg:"" optional:"" help:"Agent to unregister (claude, opencode, codex, gemini, all)"`
	Global bool   `help:"Remove from user-level config instead of project" default:"false"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Name("monocle"),
		kong.Description("Terminal-based code review companion for Claude Code"),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

func (cmd *RunCmd) Run() error {
	return runTUI(cmd.Socket, cmd.AdditionalPath, cmd.Continue, cmd.Resume, cmd.Session)
}

func (cmd *RegisterCmd) Run() error {
	agents, err := resolveAgents(cmd.Agent)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return nil // user cancelled picker
	}

	for _, a := range agents {
		if a.HasConfig() {
			fmt.Printf("  ✓ %s: already registered\n", a.Name())
			continue
		}
		if err := a.Register(cmd.Global); err != nil {
			return fmt.Errorf("register %s: %w", a.Name(), err)
		}
		fmt.Printf("  ✓ %s: registered\n", a.Name())
		for _, p := range a.ConfigPaths(cmd.Global) {
			fmt.Printf("    → %s\n", p)
		}
	}
	return nil
}

func (cmd *UnregisterCmd) Run() error {
	agents, err := resolveAgents(cmd.Agent)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return nil
	}

	for _, a := range agents {
		if !a.HasConfig() {
			fmt.Printf("  ✓ %s: nothing to remove\n", a.Name())
			continue
		}
		if err := a.Unregister(cmd.Global); err != nil {
			return fmt.Errorf("unregister %s: %w", a.Name(), err)
		}
		fmt.Printf("  ✓ %s: removed\n", a.Name())
	}
	return nil
}

func resolveAgents(name string) ([]adapters.AgentAdapter, error) {
	switch name {
	case "":
		return adapters.PickAgents(adapters.AllAdapters())
	case "all":
		return adapters.AllAdapters(), nil
	default:
		a, err := adapters.GetAdapter(name)
		if err != nil {
			return nil, err
		}
		return []adapters.AgentAdapter{a}, nil
	}
}

func (cmd *ServeMCPChannelCmd) Run() error {
	// Write the embedded bundle to a temp file
	bundlePath, err := adapters.WriteChannelBundle()
	if err != nil {
		return err
	}

	// Detect JS runtime
	rt, err := adapters.DetectJSRuntime()
	if err != nil {
		return fmt.Errorf("monocle serve-mcp-channel requires a JavaScript runtime: %w", err)
	}

	// Exec into the JS runtime, replacing this process
	binPath, argv, err := rt.ExecArgs(bundlePath)
	if err != nil {
		return err
	}

	return syscall.Exec(binPath, argv, os.Environ())
}

// Deprecated: use 'monocle register' instead.
func (cmd *InstallCmd) Run() error {
	fmt.Fprintln(os.Stderr, "Note: 'monocle install' is deprecated, use 'monocle register' instead")
	return (&RegisterCmd{Agent: cmd.Agent, Global: cmd.Global}).Run()
}

// Deprecated: use 'monocle unregister' instead.
func (cmd *UninstallCmd) Run() error {
	fmt.Fprintln(os.Stderr, "Note: 'monocle uninstall' is deprecated, use 'monocle unregister' instead")
	return (&UnregisterCmd{Agent: cmd.Agent, Global: cmd.Global}).Run()
}

func startNewSession(engine core.EngineAPI, repoRoot string) error {
	opts := core.SessionOptions{
		Agent:    "claude",
		RepoRoot: repoRoot,
	}
	if _, err := engine.StartSession(opts); err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	return nil
}

func resolveSession(engine core.EngineAPI, repoRoot string, continueSession bool, resumePicker bool, sessionID string) error {
	switch {
	case sessionID != "":
		// Direct session ID provided via --session
		if _, err := engine.ResumeSession(sessionID); err != nil {
			return fmt.Errorf("resume session %s: %w", sessionID, err)
		}
		return nil

	case continueSession:
		sessions, err := engine.ListSessions(core.ListSessionsOptions{
			RepoRoot: repoRoot,
			Limit:    1,
		})
		if err != nil || len(sessions) == 0 {
			return startNewSession(engine, repoRoot)
		}
		if _, err := engine.ResumeSession(sessions[0].ID); err != nil {
			return fmt.Errorf("resume session: %w", err)
		}
		return nil

	case resumePicker:
		// --resume: defer session creation until user picks in the TUI modal
		return nil

	default:
		return startNewSession(engine, repoRoot)
	}
}

func runTUI(socketOverride string, additionalPaths []string, continueSession bool, resumePicker bool, sessionID string) error {
	// Load config
	cfg, err := core.LoadConfig()
	if err != nil {
		cfg = core.DefaultConfig()
	}

	// Open database
	database, err := db.Open(db.DBPath())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	// Get repo root
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	repoRoot = adapters.FindRepoRoot(repoRoot)
	nonGitMode := !adapters.IsGitRepo(repoRoot)

	// Create engine
	engine, err := core.NewEngine(cfg, database, repoRoot, nonGitMode)
	if err != nil {
		return fmt.Errorf("create engine: %w", err)
	}

	// Resolve session: continue, resume, or new
	if nonGitMode {
		// In non-git mode, always start a fresh session
		if err := startNewSession(engine, repoRoot); err != nil {
			return err
		}
	} else if err := resolveSession(engine, repoRoot, continueSession, resumePicker, sessionID); err != nil {
		return err
	}

	// Reload any pending (undelivered) feedback from a previous session
	if continueSession || sessionID != "" {
		engine.ReloadPendingFeedback()
	}

	// Add additional file paths if provided (only for new sessions)
	if len(additionalPaths) > 0 && !continueSession && !resumePicker && sessionID == "" {
		if _, err := engine.AddAdditionalPaths(additionalPaths); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not add additional paths: %v\n", err)
		}
	}

	// Start socket server (deferred when showing session picker)
	socketPath := socketOverride
	if socketPath == "" {
		socketPath = adapters.DefaultSocketPath(repoRoot)
	}
	if !resumePicker {
		if err := engine.StartServer(socketPath); err != nil {
			return fmt.Errorf("start server: %w", err)
		}
	}

	// Check if MCP channel needs registration
	var appOpts tui.AppOptions
	appOpts.NonGitMode = nonGitMode
	adapter := &adapters.ClaudeAdapter{}
	if adapter.Detect() && adapter.NeedsRegister() {
		appOpts.MCPRegisterFn = func(global bool) error {
			return adapter.Register(global)
		}
	}
	if resumePicker {
		appOpts.ShowSessionPicker = true
		appOpts.RepoRoot = repoRoot
		appOpts.DeferredSocket = socketPath
	}

	// Create TUI model
	app := tui.NewApp(engine, appOpts)

	// Create Bubble Tea program
	p := tea.NewProgram(app)

	// Bridge engine events to TUI
	tui.BridgeEngineEvents(engine, p)

	// Run program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}

	// Cleanup
	engine.Shutdown()
	return nil
}
