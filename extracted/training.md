# AI-Driven Software Development Workflow & Training Manual

**Version 1.0\**
Company Internal Training and Operating Guide

## Purpose

This document defines the standard workflow for all software projects
within the company.\
The objective is to ensure that every requirement moves through a
consistent process from idea to deployment while maintaining visibility
for developers, project managers, stakeholders, and clients.

## Core Technology Stack

AI Tools: Claude, Kimi, Codex\
Project Management: Jira\
Source Control: GitHub\
Dashboard: Internal Project Dashboard\
Backend: Node.js/NestJS/Golang\
Frontend: Next.js/React\
Database: PostgreSQL/MongoDB\
Integrations: Jira API, GitHub API, GitHub Webhooks

## Project Lifecycle

1\. Client submits requirement.\
2. AI analyzes requirement.\
3. AI generates Epic, Story, Subtasks, Estimates, and Acceptance
Criteria.\
4. Jira issues are created automatically.\
5. Developer accepts assigned work.\
6. GitHub branch is created using Jira issue key.\
7. Development is completed.\
8. Pull Request is opened.\
9. Review and testing occur.\
10. Pull Request is merged.\
11. Jira status is updated automatically.\
12. Dashboard reflects progress automatically.

## Jira Standards

Structure:\
Client -\> Project -\> Epic -\> Story -\> Subtask\
\
Every story must include:\
- User Story\
- Business Value\
- Acceptance Criteria\
- Technical Notes\
- Definition of Done\
- Estimates\
- Dependencies

## GitHub Standards

Branch:\
feature/PROJECTKEY-123-feature-name\
\
Commit:\
PROJECTKEY-123 implement login validation\
\
Pull Request:\
PROJECTKEY-123 Add login validation\
\
Merged pull requests automatically update Jira and dashboard status.

## Dashboard Requirements

The dashboard must display:\
- Client\
- Project\
- Epic\
- Story\
- Jira Key\
- GitHub Branch\
- Pull Request URL\
- Status\
- Assigned Developer\
- Estimated Effort\
- Actual Effort\
- Progress Percentage\
- Last Updated

## AI Usage Policy

AI must:\
- Break requirements into stories and subtasks\
- Suggest estimates\
- Generate implementation plans\
- Avoid modifying unrelated code\
- Follow company coding standards\
- Update documentation when changes affect workflows

## Definition of Done

A feature is complete only when:\
- Code is implemented\
- Tests pass\
- Pull Request approved\
- Pull Request merged\
- Jira updated\
- Dashboard updated\
- Documentation updated if required

## Security Standards

Never expose:\
- API tokens\
- Client data\
- Database credentials\
- GitHub secrets\
- Jira credentials\
\
All secrets must be stored in environment variables.

## CLAUDE.md and AGENTS.md

Every repository must contain:\
- CLAUDE.md\
- AGENTS.md\
\
These files define project rules, coding standards, Jira workflow,
GitHub workflow, and AI operating procedures.

## New Employee Onboarding

Step 1: Read this document.\
Step 2: Review CLAUDE.md.\
Step 3: Review AGENTS.md.\
Step 4: Understand Jira workflow.\
Step 5: Understand GitHub workflow.\
Step 6: Understand dashboard architecture.\
Step 7: Complete onboarding project using the standard process.

## Implementation Roadmap

Phase 1:\
- Jira integration\
- GitHub integration\
\
Phase 2:\
- AI story generation\
- Automated Jira creation\
\
Phase 3:\
- GitHub merge synchronization\
- Dashboard synchronization\
\
Phase 4:\
- Metrics, reporting, forecasting, and AI project analytics
