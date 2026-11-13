#!/usr/bin/env python3
"""Refuse a commit that is not already on a branch or tag of this repository.

workflow_dispatch passes COMMIT_SHA. A pull request from a fork is not a
branch of this repository, so its commit is refused before checkout.
"""

import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

API_BASE = os.environ.get("FGDB_GITHUB_API", "https://api.github.com").rstrip("/")
SHA_RE = re.compile(r"[0-9a-f]{40}")
REPO_RE = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)


def auth_headers():
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "fgdb-require-repo-commit",
        "X-GitHub-Api-Version": "2022-11-28",
    }
    auth = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN") or ""
    if auth:
        headers["Authorization"] = "Bearer " + auth
    return headers


def request(url):
    req = urllib.request.Request(url, headers=auth_headers())
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read()
            link = resp.headers.get("Link")
    except urllib.error.HTTPError as exc:
        raise LookupError(str(exc.code)) from exc
    return json.loads(body.decode()), link


def next_url(link):
    if not link:
        return None
    for part in link.split(","):
        section = part.strip()
        if 'rel="next"' not in section:
            continue
        start = section.find("<")
        end = section.find(">")
        if start < 0 or end < 0:
            return None
        url = section[start + 1 : end]
        if not url.startswith(API_BASE + "/"):
            fail("refusing API pagination outside the GitHub API")
        return url
    return None


def get_json(url):
    data, _link = request(url)
    return data


def get_pages(url):
    items = []
    while url:
        data, link = request(url)
        if not isinstance(data, list):
            fail("GitHub API list was not a list")
        items.extend(data)
        url = next_url(link)
    return items


def contained(repo, ref, sha):
    quoted = urllib.parse.quote(ref, safe="/")
    url = f"{API_BASE}/repos/{repo}/compare/{quoted}...{sha}"
    try:
        data = get_json(url)
    except LookupError:
        return False
    return data.get("ahead_by") == 0


def on_named_ref(repo, sha, kind, rows):
    key = "name"
    for row in rows:
        name = row.get(key)
        if isinstance(name, str) and contained(repo, name, sha):
            print(f"commit {sha} is on {kind} {name}")
            return True
    return False


def main():
    sha = os.environ.get("COMMIT_SHA", "").strip().lower()
    repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
    if SHA_RE.fullmatch(sha) is None:
        fail("commit_sha must be 40 hexadecimal characters")
    if REPO_RE.fullmatch(repo) is None:
        fail("GITHUB_REPOSITORY is not an owner/name")
    try:
        commit = get_json(f"{API_BASE}/repos/{repo}/commits/{sha}")
    except LookupError:
        fail(f"commit {sha} is not in {repo}")
    if commit.get("sha") != sha:
        fail(f"commit {sha} is not in {repo}")
    branches = get_pages(f"{API_BASE}/repos/{repo}/branches?per_page=100")
    if on_named_ref(repo, sha, "branch", branches):
        return
    tags = get_pages(f"{API_BASE}/repos/{repo}/tags?per_page=100")
    if on_named_ref(repo, sha, "tag", tags):
        return
    fail(f"commit {sha} is not on a branch or tag of {repo}")


if __name__ == "__main__":
    main()
