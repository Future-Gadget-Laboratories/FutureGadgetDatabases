> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# FGDb feature documentation

Add each feature page to this index when it is introduced. Use the template below for `docs/fgdb/features/<feature>.md`. See [FGDb coding rules](../CODING-RULES.md#feature-docs).

## Feature index

No feature pages listed yet.

## Template

```markdown
# <Feature name>

## Overview

Describe the user-visible behavior and who uses it.

## Spec

Link the spec and identify the requirements this feature implements.

## How it works

Describe the entry points, main execution path, and any persisted state.

## External references

Link the public documentation and standards used by the spec.

## Configuration and gating

List settings, defaults, enablement and rollback conditions, and behavior when off.

## Spec → code → test cross-reference

| Spec requirement | Code | Test |
| --- | --- | --- |
| Link to requirement | Link to implementation | Link to test or fixture |

## Test evidence

Record the command or CI run, tested revision or binary, result, and any skipped coverage. Include gate-off and gate-on evidence where applicable.

## Limitations and differences

Describe unsupported cases and known differences from the reference behavior.

## Change log

Record dated or versioned changes to behavior, defaults, or compatibility.
```
