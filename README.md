# koyeb-github-runner-scheduler

koyeb-github-runner-scheduler is a golang HTTP server accepting requests from GitHub webhooks, and starting GitHub runners on demand on Koyeb.

## Usage

To start the scheduler on Koyeb, follow these steps:

#### Using the [control panel](https://app.koyeb.com/)

* On the Koyeb control panel, create a new service and select the "GitHub" deployment method
* Under "Public GitHub repository", enter the URL of this repository: https://github.com/koyeb/koyeb-github-runner-scheduler
* Select the "Dockerfile" builder
* Set the following environment variables:

| Variable name | Value |
|---------------|-------|
| **PORT** | 8000.*Make sure this value matches the port exposed under the section "Exposing gyour service".*
| **KOYEB_TOKEN** | A token created from [the console](https://app.koyeb.com/user/settings/api) which will be used to create Koyeb instances dynamically. *Prefer using a secret over a plain text environment variable.*
| **GITHUB_TOKEN** | Your GitHub token that will be used to create runner registration tokens. To generate it, go to [Developer Settings](https://github.com/settings/tokens?type=beta) > [Generate new token](https://github.com/settings/personal-access-tokens/new) and under "Permissions" select "Read & Write" for "Administration". *Prefer using a secret over a plain text value to store your token.*
| **API_SECRET** | A random secret used to authenticate requests from GitHub webhooks.*Prefer using a secret over a plain text environment variable.*
| *(optional)* **RUNNERS_TTL** | The number of minutes after which the runner will be deleted if no new jobs are received. Defaults to 2 hours.

#### Using the [Koyeb CLI](https://github.com/koyeb/koyeb-cli)

```bash
$> koyeb app create github-runner-scheduler
$> koyeb service create \
    --git github.com/koyeb/koyeb-github-runner-scheduler \
    --git-branch master \
    --git-builder docker \
    --routes /:8000 \
    --ports 8000:http \
    --env PORT=8000 \
    --env KOYEB_TOKEN=xxx \
    --env GITHUB_TOKEN=xxx \
    --env API_SECRET=xxx \
    --app github-runner-scheduler \
    scheduler
```

### Configuring your GitHub repository

Access the "Settings" page of your GitHub repository, and select the "Webhooks" section. Click on "Add webhook" and enter the following information:

* Payload URL: the public URL of your Koyeb service, which can be found on the [control panel](https://app.koyeb.com)
* Content type: select `application/json`
* Secret: enter the same value as the `API_SECRET` environment variable
* SSL verification: leave the default "Enable SSL verification" option
* Which events would you like to trigger this webhook? Select "Let me select individual events", and uncheck all events except "Workflow jobs"

### Create a workflow job

On your GitHub repository, create a new workflow file under the `.github/workflows` directory. For example, `.github/workflows/runner.yml`:

```yaml
name: my workflow

on:
  push:
    branches:
      - master

jobs:
  koyeb-paris:
    runs-on: koyeb-par-nano
    steps:
      - name: Test runner
        run: |
          echo Hello from Paris, on a Koyeb nano instance!

  koyeb-frankfurt:
    runs-on: koyeb-fra-nano
    steps:
      - name: Test runner
        run: |
          echo Hello from Frankfurt, on a Koyeb nano instance!
```