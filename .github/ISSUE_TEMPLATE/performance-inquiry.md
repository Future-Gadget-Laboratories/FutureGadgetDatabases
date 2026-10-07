---
name: Slow query or node
about: The cluster is up, and something is slower than you expected
title: ''
labels: reliability
---

**What is slow**

One sentence. Example: `SELECT` on `lab.sensors` takes about 2 seconds on a 3-node lab.

**What you ran**

```sql
-- the statement
```

**Cluster**

Paste `cockroach-oss version`.

- Nodes:
- `--insecure` or certificates:
- Replication factor if you changed it (`SHOW ZONE CONFIGURATION FROM RANGE default`):

**What you already looked at**

- [ ] `cockroach node status --ranges` (`ranges_underreplicated`, `ranges_unavailable`)
- [ ] The DB Console on the node you connected to
- [ ] `<store>/logs/` for errors at the same time

**What you hoped would happen**
