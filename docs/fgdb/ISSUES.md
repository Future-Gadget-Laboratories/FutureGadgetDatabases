> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

# Issue notifications

When someone opens an issue, or opens it again after it was closed, GitHub Actions posts a short note to Discord. The workflow is `.github/workflows/fgdb-issue-notify.yml`. It does not build the database.

The note includes the repository name, the issue number and title, who opened it, any labels, about the first 300 characters of the description, and a link to the issue. It does not ping `@everyone` or `@here`.

The job does nothing for issues opened by bots, and it does not run for a fork of this repository.

## The webhook secret

The job reads one repository secret: `DISCORD_ISSUES_WEBHOOK`. That value is the Discord webhook URL. It is not stored in the git tree. If the secret is missing or empty, the job exits successfully and does not send anything.

Do not paste the URL into an issue, a pull request, a log, or a commit.

## Rotate the webhook

Do this when the URL may have been copied, or when you want the old URL to stop working.

1. In Discord, create a new webhook. Copy its URL once.
2. On GitHub, open this repository's **Settings → Secrets and variables → Actions**.
3. Update `DISCORD_ISSUES_WEBHOOK` to the new URL.
4. Back in Discord, delete the old webhook. The old URL then stops working.

The next opened issue uses the new URL. You do not need to change the workflow file.
