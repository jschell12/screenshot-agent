export interface PromptOptions {
  screenshotPath: string;
  repo: string;
  message?: string;
}

export function buildPrompt(opts: PromptOptions): string {
  const isUrl =
    opts.repo.startsWith("http") ||
    opts.repo.startsWith("git@") ||
    /^[\w-]+\/[\w.-]+$/.test(opts.repo);

  const timestamp = Date.now();
  const branch = `screenshot-fix/${timestamp}`;

  const repoSetup = isUrl
    ? `Clone the repo: git clone https://github.com/${opts.repo.replace(/^https:\/\/github\.com\//, "")} /tmp/screenshot-agent-${timestamp}
cd /tmp/screenshot-agent-${timestamp}`
    : `cd ${opts.repo}`;

  const userContext = opts.message
    ? `\n\nAdditional context from the user: "${opts.message}"`
    : "";

  return `You are a screenshot-driven code fix agent. Your job is to analyze a screenshot, understand the problem, and fix it in a target repo.

## Step 1: Analyze the screenshot

Read the file at ${opts.screenshotPath} using the Read tool. This is a screenshot showing a bug, UI issue, error, or desired change.${userContext}

Describe what you see and identify exactly what needs to change.

## Step 2: Set up the repo

${repoSetup}

Create a new branch:
git checkout -b ${branch}

## Step 3: Find and fix the issue

Based on your analysis of the screenshot, explore the codebase to find the relevant source files. Make the necessary code changes to fix the identified issue. Be surgical — only change what is needed.

## Step 4: Commit and push

Stage the changed files (by name, not git add -A), write a clear commit message describing what the screenshot showed and what you fixed, and push the branch:
git push -u origin ${branch}

## Step 5: Create and merge PR

Create a pull request:
gh pr create --title "<concise description of the fix>" --body "## Screenshot analysis
<what you saw in the screenshot>

## Changes made
<what you changed and why>

---
Automated fix by screenshot-agent"

Then merge it:
gh pr merge --squash --auto

If the merge fails (e.g. merge conflicts, required checks), report the PR URL so the user can handle it manually.

## Important

- Do NOT hallucinate file contents — always read files before editing them.
- If the screenshot is unclear or you cannot determine what to fix, explain what you see and stop.
- If the repo requires a build step, run it after your changes to verify they compile.
`;
}
