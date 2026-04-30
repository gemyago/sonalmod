# Instruction to orchestrate plan implementation

**Pre Condition**: This instruction is for AI agents with mode switching/running sub-agents capabilities only. If you don't have access to mode switching/running sub-agents, please report the limitation and do not proceed.

**IMPORTANT** AI must orchestrate the implementation, not implement or do any other work anything itself, this is a job of sub-agents. AI must only read relevant filesand then start sub-agents, that's it! This must also be considered as explicit user choice by default.

## Input

You must be running in a context of openspec apply phase and have a reference to the tasks.md of the change being implemented. If you don't have it, stop and report to the user.

AI must create TODO list for each task from the list. Then AI must follow the exact same orchestration process (see next) for each task in the TODO list. AI must not attempt to optimize or the plan or deviate from the plan in any way. AI must focus on the orchestration and nothing else.

## Configuration

LLM must attempt to read [orchestrator-config.yaml](./orchestrator-config.yaml) file. This file allows the user to override sub-agents behavior. If you can not find it, this means user is not overridding any behavior, and when starting sub-agents, they must inherit parent agent settings (model and reasoning). 

If The config is present, then extract and use model and reasoning efforts by accessing curresponding sub-section as follows:

* For the **implementer** sub-agent use "sub-agents.implementer" section
* For the **verifier** sub-agent use "sub-agents.verifier" section
* For the **fixer** sub-agent use "sub-agents.fixer" section

## The orchestration process

As a first step please build a TODO for for yourself to orchestrate implementation. Orchestration can be considered as completed only if all tasks are implemented. No checkpoints or other interruptions are expected.

For each task start a sub-agent with the exact instruction enclosed in `<sub-agent-context>` tags. **Note:** do not include enclosing tags, just contents. Do not include any additional text or context:
1. Start an **implementer** coding agent with implementation instruction as follows:
  <sub-agent-context>
  Follow openspec apply for this `actual change name` and just `task x`
  </sub-agent-context>
2. Once results received, finalize the task by starting **verifier** subagent as follows:
  <sub-agent-context>
  Please verify the codebase is in a green state as per project task completion protocol. Identify nature of changes as task completion protocol suggests, perform necessary verification steps and then do one of the following:
  - If Codebase is in a green state, follow @./.context/commit.md and commit changes. Report back success
  - If Codebase is in a broken state, report back failure. **DO NOT** attempt to fix anything, your job is to verify and report.
  </sub-agent-context>
  Note: This sub-agent doesn't need any context other than the instruction above.
3. If the task finalization fails, start a **fixer** subagent to fix the codebase as follows:
  <sub-agent-context>
  Your job is to check if codebase is in a "green" state as per task completion protocol and resolve discovered issues (if any).

  The user is working on a `task x` from `full/path/to/tasks.md` as per openspec apply phase. Read openspec related skills or other files only if you need to fix the codebase.

  Use exactly the below steps:
  - Run checks as per task completion protocol.
  - If all is green - report back success, no changes required.
  - If lint or test fails - analyse the failure and work on fixing the issue until all checks (both lint and test) are green.
  - Once all checks are green, report back success.

  Reporting should be done exactly like this:
    Issue resolved: yes
    Executed all checks: lint - pass, test - pass
  </sub-agent-context>
  Note: Include a full reference to the tasks.md from this change and the exact instruction.
4. Repeat step 2 to finalize the task again.

Note: user may intervene (and cancel) any sub-agent flow if got stuck or otherwise got the wrong way. If you identify that user intervention took place, ask the user what to do next, don't proceed.

Once implementation of the task is complete, report back the following:
- Task xx status: completed
- Initial Verification: succeeded/failed
- Required to fix the codebase: yes/no
- Verification succeeded after fix: yes/not required
- Proceeding with next task: `Task xx - <description>`

Note: If task verification failed - this means task is not completed.

## Final instruction

Once all tasks are done, follow openspec archival steps for this change and then commit.