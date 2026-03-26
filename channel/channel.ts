import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { connect } from "net";
import { createHash } from "crypto";
import { statSync, existsSync, readFileSync } from "fs";
import { resolve, join, dirname } from "path";

// -- Socket path computation (mirrors Go's FindRepoRoot + DefaultSocketPath) --

function findRepoRoot(startDir: string): string {
  let dir = resolve(startDir);
  while (true) {
    try {
      statSync(join(dir, ".git"));
      return dir;
    } catch {
      const parent = dirname(dir);
      if (parent === dir) return resolve(startDir);
      dir = parent;
    }
  }
}

function defaultSocketPath(dir: string): string {
  const abs = resolve(dir);
  const hash = createHash("sha256").update(abs).digest("hex").slice(0, 12);
  return "/tmp/monocle-" + hash + ".sock";
}

// -- Types --

type Message = {
  type: string;
  [key: string]: any;
};

// -- Engine Connection --

class EngineConnection {
  private socketPath: string;
  private conn: ReturnType<typeof connect> | null = null;
  private pendingRequests = new Map<
    string,
    { resolve: (msg: Message) => void; reject: (err: Error) => void }
  >();
  private onEvent: (event: string, payload: Record<string, any>) => void;
  private onConnectionChange: (connected: boolean) => void;
  private lineBuffer = "";
  private reconnecting = false;
  private closed = false;
  private _connected = false;
  private reconnectAttempts = 0;
  private static readonly MAX_RECONNECT_ATTEMPTS = 50;

  get isConnected(): boolean {
    return this._connected;
  }

  private setConnected(value: boolean) {
    if (this._connected !== value) {
      this._connected = value;
      this.onConnectionChange(value);
    }
  }

  constructor(
    socketPath: string,
    onEvent: (event: string, payload: Record<string, any>) => void,
    onConnectionChange: (connected: boolean) => void,
  ) {
    this.socketPath = socketPath;
    this.onEvent = onEvent;
    this.onConnectionChange = onConnectionChange;
  }

  async connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (!existsSync(this.socketPath)) {
        reject(new Error("Socket not found: " + this.socketPath));
        return;
      }

      this.conn = connect(this.socketPath, () => {
        // Send subscribe message
        const sub = JSON.stringify({
          type: "subscribe",
          events: [
            "feedback_submitted",
            "pause_changed",
            "content_item_added",
            "additional_file_added",
          ],
        });
        this.conn!.write(sub + "\n");
      });

      this.conn.setEncoding("utf8");
      this.lineBuffer = "";
      let gotAck = false;

      this.conn.on("data", (chunk: string) => {
        this.lineBuffer += chunk;
        const lines = this.lineBuffer.split("\n");
        this.lineBuffer = lines.pop() || "";

        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const msg: Message = JSON.parse(line);
            if (!gotAck && msg.type === "subscribe_response") {
              gotAck = true;
              this.setConnected(true);
              resolve();
              continue;
            }
            this.handleMessage(msg);
          } catch {
            // ignore malformed lines
          }
        }
      });

      this.conn.on("error", (err: Error) => {
        if (!gotAck) {
          reject(err);
        } else {
          this.setConnected(false);
          this.scheduleReconnect();
        }
      });

      this.conn.on("close", () => {
        if (gotAck && !this.closed) {
          this.setConnected(false);
          this.scheduleReconnect();
        }
      });
    });
  }

  private handleMessage(msg: Message) {
    if (msg.type === "event_notification") {
      this.onEvent(msg.event, msg.payload || {});
      return;
    }

    // Response to a request — match by type
    const key = msg.type;
    const pending = this.pendingRequests.get(key);
    if (pending) {
      this.pendingRequests.delete(key);
      pending.resolve(msg);
    }
  }

  async request(msg: Message): Promise<Message> {
    if (!this.conn || this.conn.destroyed) {
      throw new Error("Not connected to monocle engine");
    }

    return new Promise((resolve, reject) => {
      const responseType = msg.type + "_response";
      this.pendingRequests.set(responseType, { resolve, reject });
      this.conn!.write(JSON.stringify(msg) + "\n");

      // Timeout after 30s
      setTimeout(() => {
        if (this.pendingRequests.has(responseType)) {
          this.pendingRequests.delete(responseType);
          reject(new Error("Request timed out: " + msg.type));
        }
      }, 30000);
    });
  }

  private scheduleReconnect() {
    if (this.reconnecting || this.closed) return;
    this.reconnecting = true;

    const attempt = (delay: number) => {
      if (this.closed) return;
      if (this.reconnectAttempts >= EngineConnection.MAX_RECONNECT_ATTEMPTS) {
        this.close();
        return;
      }
      this.reconnectAttempts++;
      const timer = setTimeout(async () => {
        try {
          await this.connect();
          this.reconnecting = false;
          this.reconnectAttempts = 0;
        } catch {
          attempt(Math.min(delay * 2, 10000));
        }
      }, delay);
      timer.unref();
    };

    attempt(1000);
  }

  // Start trying to connect in the background. Never throws.
  connectInBackground() {
    this.scheduleReconnect();
  }

  // Send agent identification (fire-and-forget, no response expected).
  identify(agent: string) {
    if (this.conn && !this.conn.destroyed) {
      this.conn.write(JSON.stringify({ type: "identify", agent }) + "\n");
    }
  }

  close() {
    this.closed = true;
    this.setConnected(false);
    if (this.conn) {
      this.conn.destroy();
      this.conn = null;
    }
  }
}

// -- Blocking connection for get_feedback --wait --

async function blockingGetFeedback(socketPath: string): Promise<Message> {
  return new Promise((resolve, reject) => {
    const conn = connect(socketPath, () => {
      const msg = JSON.stringify({ type: "poll_feedback", wait: true });
      conn.write(msg + "\n");
    });

    conn.setEncoding("utf8");
    let buf = "";

    conn.on("data", (chunk: string) => {
      buf += chunk;
      const lines = buf.split("\n");
      buf = lines.pop() || "";

      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const msg = JSON.parse(line);
          resolve(msg);
          conn.destroy();
        } catch {
          // ignore
        }
      }
    });

    conn.on("error", (err: Error) => reject(err));
    conn.on("close", () => reject(new Error("Connection closed")));
  });
}

// -- Main --

const cwd = process.cwd();
const repoRoot = findRepoRoot(cwd);
const socketPath = process.env.MONOCLE_SOCKET || defaultSocketPath(repoRoot);

// When a tool (submit_plan_and_wait, get_feedback --wait) is blocking for
// feedback, suppress the event-based notification to avoid delivering the
// same feedback twice.
let waitingForFeedback = false;

// Create MCP server with channel capability
const mcp = new Server(
  { name: "monocle", version: "1.0.0" },
  {
    capabilities: {
      experimental: { "claude/channel": {} },
      tools: {},
    },
    instructions: [
      "Events from the monocle channel arrive as <channel source=\"monocle\" event=\"...\">.",
      "These are review events from your human reviewer who is watching your code changes in real-time using Monocle.",
      "",
      "When you receive a feedback_submitted event, the full review feedback is included in the notification content. Read and act on it directly — do not call get_feedback.",
      "When you receive a pause_requested event, your reviewer wants you to stop and wait. Use the get_feedback tool with wait=true to block until they submit their review.",
      "",
      "You can submit plans or architecture decisions for your reviewer to see using the submit_plan tool.",
      "When submitting plans, use the plan filename as the id parameter so updates replace the previous version.",
      "You can check the current review status at any time using the review_status tool.",
      "",
      "When a plan draft is ready, use submit_plan_and_wait to get feedback from your reviewer.",
      "Pass the plan file_path so the reviewer sees exactly what you wrote.",
      "If changes are requested, update the plan and call submit_plan_and_wait again. Continue iterating until approved.",
    ].join("\n"),
  },
);

// Engine connection with event handler that pushes channel notifications
const engine = new EngineConnection(
  socketPath,
  // onEvent
  (event, payload) => {
    switch (event) {
      case "feedback_submitted":
        if (waitingForFeedback) break; // Tool call will deliver this feedback
        mcp
          .notification({
            method: "notifications/claude/channel",
            params: {
              content:
                payload.message || "Your reviewer has submitted feedback. Use the get_feedback tool to retrieve it.",
              meta: { event: "feedback_submitted" },
            },
          })
          .catch((err: unknown) => { console.error("Failed to deliver feedback_submitted notification:", err); });
        break;
      case "pause_changed":
        if (payload.status === "pause_requested") {
          mcp
            .notification({
              method: "notifications/claude/channel",
              params: {
                content:
                  "Your reviewer has requested you pause and wait for feedback. " +
                  "Use the get_feedback tool with wait=true to block until feedback is ready.",
                meta: { event: "pause_requested" },
              },
            })
            .catch((err: unknown) => { console.error("Failed to deliver pause_requested notification:", err); });
        }
        break;
      case "content_item_added":
        // Informational — no push needed, the agent submitted this
        break;
      case "additional_file_added":
        // Informational — no push needed, the agent added this
        break;
    }
  },
  // onConnectionChange — notify Claude Code to re-fetch tools
  (_connected: boolean) => {
    mcp
      .notification({ method: "notifications/tools/list_changed" })
      .catch(() => {});
  },
);

// -- Tools --

const TOOLS = [
    {
      name: "review_status",
      description:
        "Check if your reviewer has pending feedback or has requested a pause",
      inputSchema: {
        type: "object" as const,
        properties: {},
      },
    },
    {
      name: "get_feedback",
      description:
        "Retrieve review feedback from your reviewer. Use wait=true to block until feedback is available (pause flow).",
      inputSchema: {
        type: "object" as const,
        properties: {
          wait: {
            type: "boolean",
            description: "Block until feedback is available",
          },
        },
      },
    },
    {
      name: "submit_plan",
      description:
        "Submit a plan, architecture decision, or other content for your reviewer to see and comment on",
      inputSchema: {
        type: "object" as const,
        properties: {
          title: {
            type: "string",
            description: "Title for the plan or content",
          },
          content: {
            type: "string",
            description: "The plan or content body (markdown supported). Ignored if file_path is provided.",
          },
          file_path: {
            type: "string",
            description: "Absolute path to a file whose content to submit. Takes precedence over content.",
          },
          id: {
            type: "string",
            description: "Optional ID for updating existing content",
          },
          content_type: {
            type: "string",
            description:
              "File extension for syntax highlighting (e.g. 'md', 'go', 'py', 'ts')",
          },
        },
        required: ["title"],
      },
    },
    {
      name: "add_files",
      description:
        "Add additional files for your reviewer to see in Monocle. Accepts absolute file or directory paths from anywhere on the filesystem.",
      inputSchema: {
        type: "object" as const,
        properties: {
          paths: {
            type: "array",
            items: { type: "string" },
            description:
              "Absolute file or directory paths to add for review",
          },
        },
        required: ["paths"],
      },
    },
    {
      name: "submit_plan_and_wait",
      description:
        "Submit a plan for review AND block until your reviewer responds with feedback. " +
        "Use this in plan mode instead of submit_plan. " +
        "If they request changes, update the plan and call this again. " +
        "Keep iterating until the reviewer approves, then continue with your normal workflow.",
      inputSchema: {
        type: "object" as const,
        properties: {
          title: {
            type: "string",
            description: "Title for the plan",
          },
          content: {
            type: "string",
            description: "The plan body (markdown supported). Ignored if file_path is provided.",
          },
          file_path: {
            type: "string",
            description: "Absolute path to a file whose content to submit. Takes precedence over content.",
          },
          id: {
            type: "string",
            description: "Optional ID for updating existing content",
          },
          content_type: {
            type: "string",
            description:
              "File extension for syntax highlighting (e.g. 'md', 'go', 'py', 'ts')",
          },
        },
        required: ["title"],
      },
    },
];

mcp.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: engine.isConnected ? TOOLS : [],
}));

mcp.setRequestHandler(CallToolRequestSchema, async (req) => {
  const args = (req.params.arguments || {}) as Record<string, any>;

  switch (req.params.name) {
    case "review_status": {
      try {
        const resp = await engine.request({ type: "get_review_status" });
        return {
          content: [
            { type: "text" as const, text: resp.summary || "No reviewer connected." },
          ],
        };
      } catch {
        return {
          content: [{ type: "text" as const, text: "No reviewer connected. Make sure Monocle is running in your terminal." }],
        };
      }
    }

    case "get_feedback": {
      try {
        let resp: Message;
        if (args.wait) {
          waitingForFeedback = true;
          try {
            resp = await blockingGetFeedback(socketPath);
          } finally {
            // Defer flag reset: the Go engine wakes the blocking poll before
            // emitting the event notification, so the event may still be
            // in-flight on the subscription socket when this resolves.
            setTimeout(() => { waitingForFeedback = false; }, 200);
          }
        } else {
          resp = await engine.request({ type: "poll_feedback", wait: false });
        }

        if (resp.has_feedback) {
          return {
            content: [{ type: "text" as const, text: resp.feedback }],
          };
        }
        return {
          content: [{ type: "text" as const, text: "No feedback pending." }],
        };
      } catch {
        return {
          content: [{ type: "text" as const, text: "No reviewer connected. Make sure Monocle is running in your terminal." }],
        };
      }
    }

    case "submit_plan": {
      try {
        let content = args.content || "";
        if (args.file_path) {
          content = readFileSync(args.file_path, "utf-8");
        }
        if (!content) {
          return {
            content: [{ type: "text" as const, text: "Either content or file_path must be provided." }],
          };
        }
        const resp = await engine.request({
          type: "submit_content",
          id: args.id || "",
          title: args.title,
          content,
          content_type: args.content_type || "",
          is_plan: true,
        });
        return {
          content: [
            {
              type: "text" as const,
              text: resp.message || "Content submitted for review.",
            },
          ],
        };
      } catch {
        return {
          content: [{ type: "text" as const, text: "No reviewer connected. Make sure Monocle is running in your terminal." }],
        };
      }
    }

    case "submit_plan_and_wait": {
      try {
        // Resolve content from file_path or inline content
        let content = args.content || "";
        if (args.file_path) {
          content = readFileSync(args.file_path, "utf-8");
        }
        if (!content) {
          return {
            content: [{ type: "text" as const, text: "Either content or file_path must be provided." }],
          };
        }

        // Step 1: Submit the plan
        await engine.request({
          type: "submit_content",
          id: args.id || "",
          title: args.title,
          content,
          content_type: args.content_type || "",
          is_plan: true,
        });

        // Step 2: Block until reviewer submits feedback
        waitingForFeedback = true;
        let feedback: Message;
        try {
          feedback = await blockingGetFeedback(socketPath);
        } finally {
          // Defer flag reset: the Go engine wakes the blocking poll before
          // emitting the event notification, so the event may still be
          // in-flight on the subscription socket when this resolves.
          setTimeout(() => { waitingForFeedback = false; }, 200);
        }

        if (feedback.has_feedback) {
          return {
            content: [{ type: "text" as const, text: feedback.feedback }],
          };
        }
        return {
          content: [{ type: "text" as const, text: "Review approved." }],
        };
      } catch {
        return {
          content: [{ type: "text" as const, text: "No reviewer connected. Make sure Monocle is running in your terminal." }],
        };
      }
    }

    case "add_files": {
      try {
        const resp = await engine.request({
          type: "add_additional_files",
          paths: args.paths || [],
        });
        return {
          content: [
            {
              type: "text" as const,
              text: resp.message || "Files added for review.",
            },
          ],
        };
      } catch {
        return {
          content: [{ type: "text" as const, text: "No reviewer connected. Make sure Monocle is running in your terminal." }],
        };
      }
    }

    default:
      throw new Error(`Unknown tool: ${req.params.name}`);
  }
});

// -- Start --

async function main() {
  // Try connecting to engine immediately — fails fast if socket doesn't exist.
  // This ensures tools are available on Claude's first fetch when Monocle is running.
  try {
    await engine.connect();
  } catch {
    // Monocle not running yet — retry in background; tools appear via list_changed.
    engine.connectInBackground();
  }

  // Connect to Claude Code via stdio
  const transport = new StdioServerTransport();
  await mcp.connect(transport);

  // Identify the connecting agent to Monocle
  const clientVersion = mcp.getClientVersion();
  if (clientVersion?.name) {
    engine.identify(clientVersion.name);
  }

  // Handle graceful shutdown
  process.on("SIGINT", () => {
    engine.close();
    process.exit(0);
  });
  process.on("SIGTERM", () => {
    engine.close();
    process.exit(0);
  });

  // Detect parent death: when Claude Code exits, stdin's write end closes.
  // The MCP SDK does not handle this, so we listen directly.
  const exitOnStdinClose = () => {
    engine.close();
    process.exit(0);
  };
  process.stdin.on("end", exitOnStdinClose);
  process.stdin.on("close", exitOnStdinClose);
}

main().catch((err) => {
  console.error("Fatal:", err);
  process.exit(1);
});
