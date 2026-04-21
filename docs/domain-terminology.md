# Domain terminology

Canonical product vocabulary for planning, design, documentation, and UI/API copy—inside or outside the BMad framework. Prefer **one term per concept** consistently.

**Source:** Aligned with the innovation strategy narrative; this file is the **single place to edit** definitions. Other docs should link here rather than duplicating the table.

---

## Glossary

| Term | Definition |
|------|------------|
| **Workspace** | Boundary for people, repos, policies, and **Agents**. |
| **Project** | Optional human-facing initiative: goal + linked repos + milestones (grouping **above** a single repo). |
| **Connection** | Link to a Git forge, **agent runtime** (e.g. ACP endpoint, credential reference)—often **BYOA**—or—**when integrated**—a **notification channel** (**Slack**, **Discord**, …). |
| **Operator** | Human or role responsible for **approvals** and **policy-bound triggers**; distinct from **automated** work executed by specialists. |
| **Automated step** | Work performed **inside** a specialist tool/agent using existing prompts and tools—Sonalmod may **start** or **advance** it, not necessarily author every prompt. |
| **Agent** | Configured **specialist** the control plane runs: role, model/runtime, tools, prompts, limits—which **Skills** and **Commands** bind (per product/runtime rules). Conceptually, a **Session** is where agentic work happens; the control plane **binds** an **Agent** to that work—**Sessions** do not “invoke” **Agents**. One **Agent** may participate in many **Sessions** over time. |
| **Skill** | Standard **LLM** unit of reusable **capability**: bundled instructions (and often tool hooks)—e.g. **SKILL.md**-style modules an **Agent** or runtime loads and composes. |
| **Command** | Standard **LLM** (and agent-runtime) unit of **invocation**: a named **command** (arguments, behavior) the model or stack exposes—**pre-defined** QueueFlow entries align here when modeled as **Commands**. Distinct from **control-plane** actions (see disambiguation). |
| **Control-plane action** | User-invoked **Sonalmod** action in UI/CLI (e.g. start a **Session**, add a **Connection**). **Not** an LLM **Command** unless the product explicitly maps the two. |
| **Session** | The product’s unit of **agentic work**—aligned with how SEs and power users talk about **agentic sessions**: may be a **simple chat**, a **long orchestrated thread** of work, or both over time; has **status**, **owner**, **timeline**, and links to artifacts when tracked. At implementation level, a **Session** **composes** from **Session events** (see **Session event**). Distinguished from **runtime session** (see disambiguation). |
| **QueueFlow** | Internal codename for **agentic workflow queues**: **async**, **durable** agent workloads inside a **workspace**, with **throttle**, explicit lifecycle, and **queue workflow types**. Complements interactive orchestration; does **not** replace **Session** semantics for interactive UX. |
| **Queued job** | One **persisted** background agent workload (inputs, **queue workflow type**, lifecycle status, outputs). **Not** a human **Task**; only **linked** to a **Session** when the product explicitly ties them (e.g. a **Session** **spawns** jobs). |
| **Queue workflow type** | What a **Queued job** runs on QueueFlow: either a **one-off message** (ad hoc input) or a **pre-defined** **Command** (named, repeatable). Registry, UX, validation, and mapping to **Command** / **Skill** contracts are **TBD**. In QueueFlow UI copy, “**workflow type**” alone is acceptable when context is clear. |
| **Throttle** (QueueFlow) | Concurrency and/or cost **guardrail**: at most **N** concurrent workers (per **workspace** or globally) so LLM spend and provider rate limits stay **bounded**. |
| **Master plane** (post-MVP) | Central Sonalmod deployment that may **coordinate** other instances and **fleets** of specialists. |
| **Mesh** (post-MVP) | Topology linking instances (e.g. **local** ↔ **master plane**) with shared policy and visibility. |
| **Bring your own agent (BYOA)** | Customer-operated **agent runtime**: their **Connection** (endpoint, credentials), **compute**, and **models**—Sonalmod **orchestrates** **Sessions** and applies **policy**/**audit** without necessarily **hosting** specialist **execution**. Contrasts with **provider-hosted** runtimes or **mesh**/**cloud**-coordinated capacity where execution **environment** may be supplied or **metered** differently. **Commercial** packaging (e.g. whether orchestration is **metered** the same when execution is BYOA) is **not** fixed here. |
| **Playbook** / **Workflow** | Reusable sequence (e.g. plan → implement → PR → review); pick **one** primary term in UX. |
| **Session event** | One **recorded** item on a **Session**’s **timeline**—the same **grain** the implementation uses: e.g. **Agent** ↔ **Operator** **messages**, **tool** **invocations** (read file, create page, …), and other **typed** **events** in the **schema**. Prefer this over inventing a separate **pipeline** **segment** (e.g. “stage”): higher-level **Playbook** **intent** does not need to map 1:1 to a fixed sequence of **session events**. |
| **Task** | Human work-queue item: approve, answer question, resolve policy conflict. |
| **Policy** | Rules: branches, allowed operations, approvals, secret handling. |
| **Event trigger** | When a matching **event** arrives (often via forge **webhook**), Sonalmod runs the configured reaction: **start** or **continue** a **Session**, **enqueue** a **Queued job** (e.g. with a pre-defined **Command** on QueueFlow), or **route** between those outcomes per policy—same machinery; not every **event** opens a **Session**. In UI/API copy, prefer **event trigger**; **session trigger** is acceptable when the outcome is explicitly session-shaped. Avoid **subscription** (often read as billing). |

---

## Disambiguation

Use these qualifiers when context could confuse product language with implementation details:

| Phrase | Meaning |
|--------|---------|
| **Organization workspace** (or **product workspace**) | The glossary **Workspace**—tenant boundary for people, repos, policies, **Agents**. |
| **Tool workspace** / **filesystem workspace** | A scoped directory identifier used by filesystem tools (e.g. `workspacefs`); not the same as organization workspace unless you deliberately map them in product UX. |
| **Runtime session** | Persistent **conversation/thread** in the agent API (identifiers, replay, transport—e.g. SSE). One product **Session** may involve one or many **runtime sessions** over time (reconnects, queue handoff, retries). Prefer this qualifier in specs when “session” could mean product vs wire format. |
| **Agent execution** | One **invocation** of the agent in the runtime HTTP API (e.g. an SSE stream), usually tied to a **runtime session**. Many **agent executions** may occur inside one product **Session**. Implementation code may still say **run** (e.g. method names); user-facing copy should prefer **execution** or **invocation** over **run** where it reduces confusion with CI. |
| **Operator** (product) | Human approver / policy actor—not necessarily the same as “server operator” or deployment admin in ops language. |
| **Event trigger** vs informal **trigger** (policy / approval) | **Event trigger** = forge **event** (often via **webhook**) drives a configured outcome (**Session** and/or **Queued job**). “**Policy-bound triggers**” mean **gates** or **handoffs** for an **Operator**—not the same as an **event trigger** unless a spec ties them. |
| **Command** (LLM) vs **control-plane action** | **Command** = LLM/agent-runtime **invocation** (named entry). **Control-plane action** = **Sonalmod** UI/CLI gesture (start **Session**, connect forge, …). Same English word—use these qualifiers in specs. |
| **Skill** vs **Command** | **Skill** = reusable **capability** module (instructions, tool hooks). **Command** = **invocable** **entry point**; overlap per runtime **TBD**. |
| **Agent** vs **Agent execution** | **Agent** = configured **specialist** (recipe). **Agent execution** = one **invocation** in the runtime API—many **agent executions** may occur inside one product **Session**. |
| **BYOA** vs **hosted** / **mesh**-supplied runtime | **BYOA** = customer **owns** runtime **cost** and **infrastructure** for the specialist. **Hosted** or **mesh**-orchestrated capacity = execution **may** run on **provider**- or **org**-managed **environments** subject to different **ops** and **packaging**—see innovation strategy for **non-binding** commercial notes. |
| **Queue workflow type** vs LLM **Command** | **One-off** input is not necessarily a **Command**; **pre-defined** queue work maps to **Command** when it shares the same contract—details **TBD**. |
| **Queued job** vs **Task** | **Task** = human **inbox** item (approve, answer). **Queued job** = **async** agent unit in QueueFlow—different lifecycle and ownership. |
| **Queued job** vs **Session** (product) | **Session** = tracked **agentic work** (chat-shaped, playbook-shaped, PR-linked, handoffs). **Queued job** = **batch-style** async pipeline item; same **workspace** / **policy** / **audit** plane, different **default** UX and linking rules. |
| **Session event** vs **Event trigger** | **Session event** = something **on** the **Session** **timeline** (messages, tool calls, …). **Event trigger** = external **forge** **event** (often **webhook**) that **starts** or **routes** work—different namespace; do not conflate with per-session **event** **types** in storage/API. |
| **Playbook** vs **queue workflow type** | **Playbook** = reusable human-facing **pattern** (goals, **Event trigger** hooks); **Session** **behavior** is still **session** **events** under the hood. **Queue workflow type** = **what** runs on the queue (**one-off** vs **pre-defined** **Command**)—orthogonal unless a spec **connects** them. |
| **Queue job lifecycle** (user-facing) | **Pending** → **Processing** → **Ready** (success); terminal **Failed** (or **Cancelled**) when failures and cancellation are surfaced—keep labels consistent in UI and APIs. |

---