---
name: Bug
about: Wrong result, crash, stuck node, or lost availability
title: ''
labels: bug, reliability
---

**What broke**

Say what you expected and what the cluster did. If data was wrong, missing, or unavailable, say that first.

**Steps**

1. How the cluster was started (one node or more, `--insecure` or certificates).
2. The SQL or the command you ran.
3. What you saw.

Paste a small schema when the bug is in a query. Example:

```sql
CREATE TABLE lab.sensors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name STRING NOT NULL,
  reading FLOAT
);
```

**Version**

Paste `cockroach-oss version` (or `cockroach version` from the release tarball). The `Distribution` line should say `OSS`. If it does not, say so.

- Binary: local `./dev build oss`, or release `v23.2.15-oss`
- OS:
- Nodes:

**Logs**

Logs can contain SQL text and addresses. Paste only the lines that show the error. For a fatal error, say which node's `<store>/logs/` you looked at. Do not attach secrets or customer data. If the report is a vulnerability, close this issue and use a private advisory: https://github.com/Future-Gadget-Laboratories/FutureGadgetDatabases/security/advisories/new

**Reliability**

Which of these happened?

- [ ] Wrong query result
- [ ] Crash or restart loop
- [ ] Range or node unavailable
- [ ] Data lost after a restart
- [ ] Other (describe above)
